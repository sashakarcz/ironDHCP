package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sashakarcz/irondhcp/internal/api"
	"github.com/sashakarcz/irondhcp/internal/config"
	"github.com/sashakarcz/irondhcp/internal/dhcp"
	"github.com/sashakarcz/irondhcp/internal/events"
	"github.com/sashakarcz/irondhcp/internal/gitops"
	"github.com/sashakarcz/irondhcp/internal/logger"
	"github.com/sashakarcz/irondhcp/internal/metrics"
	"github.com/sashakarcz/irondhcp/internal/storage"
)

const banner = `
  _                 ___  _  _  ___ ___
 (_)_ _ ___ _ _    |   \| || |/ __| _ \
 | | '_/ _ \ ' \   | |) | __ | (__|  _/
 |_|_| \___/_||_|  |___/|_||_|\___|_|

  Simple, Production-Ready DHCP Server
  Version: 1.0.0
`

var (
	configFile = flag.String("config", "example-config.yaml", "Path to configuration file")
	version    = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Print("ironDHCP v1.0.0\n")
		os.Exit(0)
	}

	fmt.Print(banner)

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	if err := logger.Setup(logger.Config{
		Level:  cfg.Observability.LogLevel,
		Format: cfg.Observability.LogFormat,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info().
		Str("config", *configFile).
		Msg("Starting ironDHCP server")

	// Create main context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ensure database exists and run migrations
	logger.Info().Msg("Initializing database")
	if err := storage.EnsureDatabase(ctx, cfg.Database.Connection); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize database")
	}

	// Connect to database
	logger.Info().Msg("Connecting to database")
	store, err := storage.New(ctx, storage.Config{
		ConnectionString: cfg.Database.Connection,
		MaxConnections:   cfg.Database.MaxConnections,
		MinConnections:   cfg.Database.MinConnections,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer store.Close()

	logger.Info().Msg("Database connection established")

	// Initialize metrics
	_ = metrics.New()
	logger.Info().Msg("Initialized Prometheus metrics")

	// Initialize event broadcaster for activity log
	broadcaster := events.NewBroadcaster()
	broadcaster.Start(ctx)
	logger.Info().Msg("Initialized event broadcaster")

	// Initialize GitOps (if enabled)
	var gitPoller *gitops.Poller
	var syncService *gitops.SyncService
	var expiryWorker *dhcp.ExpiryWorker

	if cfg.Git.Enabled {
		logger.Info().
			Str("repository", cfg.Git.Repository).
			Str("branch", cfg.Git.Branch).
			Msg("Initializing GitOps")

		// Create Git repository manager
		repoConfig := &gitops.RepositoryConfig{
			URL:            cfg.Git.Repository,
			Branch:         cfg.Git.Branch,
			LocalPath:      "/tmp/irondhcp-git",
			ConfigFilePath: cfg.Git.ConfigPath,
			PollInterval:   cfg.Git.PollInterval,
		}

		// Add authentication if configured
		if cfg.Git.Auth.Type == "token" && cfg.Git.Auth.Token != "" {
			repoConfig.Username = "git"
			repoConfig.Password = cfg.Git.Auth.Token
		}

		repo := gitops.NewRepository(repoConfig)
		if err := repo.Initialize(ctx); err != nil {
			logger.Fatal().Err(err).Msg("Failed to initialize Git repository")
		}

		// Create sync service with base config and reload function (will be set later)
		syncService = gitops.NewSyncService(repo, store, cfg, nil)

		// Create poller
		gitPoller = gitops.NewPoller(syncService, cfg.Git.PollInterval)

		logger.Info().Msg("GitOps initialized")
	} else {
		logger.Info().Msg("GitOps disabled")

		// If GitOps is disabled, sync reservations from local config
		logger.Info().Msg("Syncing reservations from config to database")
		if err := syncReservations(ctx, store, cfg); err != nil {
			logger.Warn().Err(err).Msg("Failed to sync reservations")
		}
	}

	// Create DHCP server
	dhcpServer, err := dhcp.New(cfg, store, broadcaster)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create DHCP server - API server will continue")
	} else {
		// Start DHCP server
		if err := dhcpServer.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("Failed to start DHCP server - API server will continue")
			dhcpServer = nil
		}
	}

	// Create and start API server (if enabled)
	var apiServer *api.Server
	if cfg.Observability.WebEnabled {
		apiServer = api.New(api.Config{
			Port:    cfg.Observability.WebPort,
			Enabled: cfg.Observability.WebEnabled,
			WebAuth: &cfg.Observability.WebAuth,
		}, store, gitPoller, broadcaster, cfg)

		if err := apiServer.Start(ctx); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start API server")
		}

		// Set GitOps reload function to update API server config and DHCP server subnets
		// This must be set BEFORE starting the poller so the initial sync updates both servers
		if syncService != nil {
			syncService.SetReloadFunc(func(newCfg *config.Config) error {
				apiServer.UpdateConfig(newCfg)

				// Reload DHCP server subnets if server is running
				if dhcpServer != nil {
					if err := dhcpServer.ReloadSubnets(newCfg); err != nil {
						logger.Error().Err(err).Msg("Failed to reload DHCP server subnets")
						return fmt.Errorf("failed to reload DHCP server subnets: %w", err)
					}
				}

				return nil
			})
		}
	}

	// Start GitOps poller (if enabled)
	// This is started AFTER setting the reload callback so initial sync updates API server
	if gitPoller != nil {
		if err := gitPoller.Start(ctx); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start GitOps poller")
		}
	}

	// Start lease expiry worker
	expiryWorker = dhcp.NewExpiryWorker(store, 5*time.Minute)
	if err := expiryWorker.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start lease expiry worker")
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info().Msg("ironDHCP server is running. Press Ctrl+C to stop.")

	<-sigChan
	logger.Info().Msg("Shutdown signal received, stopping server...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop GitOps poller
	if gitPoller != nil {
		if err := gitPoller.Stop(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Error stopping GitOps poller")
		}
	}

	// Stop lease expiry worker
	if expiryWorker != nil {
		if err := expiryWorker.Stop(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Error stopping lease expiry worker")
		}
	}

	// Stop DHCP server
	if dhcpServer != nil {
		if err := dhcpServer.Stop(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Error stopping DHCP server")
		}
	}

	// Stop API server
	if apiServer != nil {
		if err := apiServer.Stop(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Error stopping API server")
		}
	}

	logger.Info().Msg("Server stopped. Goodbye!")
}

// syncReservations syncs reservations from config to database
// This is a simple implementation for Phase 1
// Phase 2 will use GitOps for configuration management
func syncReservations(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	// Get existing reservations
	existing, err := store.GetAllReservations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get existing reservations: %w", err)
	}

	// Build map of existing reservations by MAC
	existingMap := make(map[string]*storage.Reservation)
	for _, res := range existing {
		existingMap[res.MAC.String()] = res
	}

	// Sync reservations from config
	for _, subnetCfg := range cfg.Subnets {
		_, network, _ := config.ParseCIDR(subnetCfg.Network)

		for _, resCfg := range subnetCfg.Reservations {
			mac, _ := config.ParseMAC(resCfg.MAC)
			ip := config.ParseIP(resCfg.IP)

			// Extract boot options if present
			var tftpServer, bootFilename string
			if resCfg.Boot != nil {
				tftpServer = resCfg.Boot.TFTPServer
				bootFilename = resCfg.Boot.Filename
			}

			// Check if reservation exists
			if existingRes, found := existingMap[mac.String()]; found {
				// Update if changed
				if !existingRes.IP.Equal(ip) || existingRes.Hostname != resCfg.Hostname ||
					existingRes.Description != resCfg.Description ||
					existingRes.TFTPServer != tftpServer || existingRes.BootFilename != bootFilename {
					existingRes.IP = ip
					existingRes.Hostname = resCfg.Hostname
					existingRes.Subnet = network
					existingRes.Description = resCfg.Description
					existingRes.TFTPServer = tftpServer
					existingRes.BootFilename = bootFilename

					if err := store.UpdateReservation(ctx, existingRes); err != nil {
						logger.Error().
							Err(err).
							Str("mac", mac.String()).
							Msg("Failed to update reservation")
					} else {
						logger.Info().
							Str("mac", mac.String()).
							Str("ip", ip.String()).
							Msg("Updated reservation")
					}
				}
				delete(existingMap, mac.String())
			} else {
				// Create new reservation
				newRes := &storage.Reservation{
					MAC:          mac,
					IP:           ip,
					Hostname:     resCfg.Hostname,
					Subnet:       network,
					Description:  resCfg.Description,
					TFTPServer:   tftpServer,
					BootFilename: bootFilename,
				}

				if err := store.CreateReservation(ctx, newRes); err != nil {
					logger.Error().
						Err(err).
						Str("mac", mac.String()).
						Msg("Failed to create reservation")
				} else {
					logger.Info().
						Str("mac", mac.String()).
						Str("ip", ip.String()).
						Msg("Created reservation")
				}
			}
		}
	}

	// Delete reservations that are no longer in config
	for _, res := range existingMap {
		if err := store.DeleteReservation(ctx, res.ID); err != nil {
			logger.Error().
				Err(err).
				Str("mac", res.MAC.String()).
				Msg("Failed to delete reservation")
		} else {
			logger.Info().
				Str("mac", res.MAC.String()).
				Msg("Deleted reservation")
		}
	}

	return nil
}
