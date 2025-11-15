package dhcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/sashakarcz/irondhcp/internal/config"
	"github.com/sashakarcz/irondhcp/internal/events"
	"github.com/sashakarcz/irondhcp/internal/logger"
	"github.com/sashakarcz/irondhcp/internal/storage"
)

// Server represents a DHCP server
type Server struct {
	config      *config.Config
	store       *storage.Store
	allocator   *Allocator
	broadcaster Broadcaster
	server4     *server4.Server
	subnets     map[string]*SubnetConfig // subnet CIDR -> config
	interfaces  []string
	wg          sync.WaitGroup
	shutdown    chan struct{}
}

// Broadcaster interface for activity events
type Broadcaster interface {
	BroadcastDHCPEvent(eventType events.EventType, ip net.IP, mac net.HardwareAddr, hostname string, details map[string]interface{})
}

// SubnetConfig holds runtime subnet configuration
type SubnetConfig struct {
	Network          *net.IPNet
	Description      string
	Gateway          net.IP
	DNSServers       []net.IP
	LeaseDuration    time.Duration
	MaxLeaseDuration time.Duration
	Options          map[string]string
	TFTPServer       string // DHCP option 66
	BootFilename     string // DHCP option 67
	Pools            []*PoolConfig
}

// New creates a new DHCP server
func New(cfg *config.Config, store *storage.Store, broadcaster Broadcaster) (*Server, error) {
	// Create allocator with server ID and optional cache
	// Use server ID from config, or empty string if not set
	serverID := cfg.Server.ServerID
	useCache := cfg.Server.Cluster.UseReadCache
	allocator := NewAllocator(store, 10000, serverID, useCache) // 10k lease cache

	// Build subnet map
	subnets := make(map[string]*SubnetConfig)
	for _, subnetCfg := range cfg.Subnets {
		_, network, err := net.ParseCIDR(subnetCfg.Network)
		if err != nil {
			return nil, fmt.Errorf("invalid subnet %s: %w", subnetCfg.Network, err)
		}

		// Parse DNS servers
		var dnsServers []net.IP
		for _, dnsStr := range subnetCfg.DNSServers {
			dnsServers = append(dnsServers, net.ParseIP(dnsStr))
		}

		// Convert pools
		var pools []*PoolConfig
		for _, poolCfg := range subnetCfg.Pools {
			pools = append(pools, &PoolConfig{
				RangeStart: poolCfg.RangeStart,
				RangeEnd:   poolCfg.RangeEnd,
			})
		}

		// Extract boot options if present
		var tftpServer, bootFilename string
		if subnetCfg.Boot != nil {
			tftpServer = subnetCfg.Boot.TFTPServer
			bootFilename = subnetCfg.Boot.Filename
		}

		subnets[network.String()] = &SubnetConfig{
			Network:          network,
			Description:      subnetCfg.Description,
			Gateway:          net.ParseIP(subnetCfg.Gateway),
			DNSServers:       dnsServers,
			LeaseDuration:    subnetCfg.LeaseDuration,
			MaxLeaseDuration: subnetCfg.MaxLeaseDuration,
			Options:          subnetCfg.Options,
			TFTPServer:       tftpServer,
			BootFilename:     bootFilename,
			Pools:            pools,
		}
	}

	// Extract interface names
	var interfaces []string
	for _, iface := range cfg.Server.Interfaces {
		interfaces = append(interfaces, iface.Name)
	}

	return &Server{
		config:      cfg,
		store:       store,
		allocator:   allocator,
		broadcaster: broadcaster,
		subnets:     subnets,
		interfaces:  interfaces,
		shutdown:    make(chan struct{}),
	}, nil
}

// Start starts the DHCP server
func (s *Server) Start(ctx context.Context) error {
	logger.Info().
		Strs("interfaces", s.interfaces).
		Int("subnets", len(s.subnets)).
		Msg("Starting DHCP server")

	// Create DHCP handler
	handler := &Handler{
		server: s,
	}

	// Create DHCPv4 server
	laddr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: dhcpv4.ServerPort,
	}

	server, err := server4.NewServer("", laddr, handler.Handle)
	if err != nil {
		return fmt.Errorf("failed to create DHCP server: %w", err)
	}

	s.server4 = server

	// Start expiry worker
	s.wg.Add(1)
	go s.expiryWorker(ctx)

	// Start cache cleanup worker
	s.wg.Add(1)
	go s.cacheCleanupWorker(ctx)

	// Start server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := server.Serve(); err != nil {
			logger.Error().Err(err).Msg("DHCP server stopped with error")
		}
	}()

	logger.Info().Msg("DHCP server started successfully")
	return nil
}

// Stop stops the DHCP server
func (s *Server) Stop(ctx context.Context) error {
	logger.Info().Msg("Stopping DHCP server")

	close(s.shutdown)

	if s.server4 != nil {
		if err := s.server4.Close(); err != nil {
			logger.Error().Err(err).Msg("Error closing DHCP server")
		}
	}

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info().Msg("DHCP server stopped successfully")
	case <-time.After(10 * time.Second):
		logger.Warn().Msg("DHCP server shutdown timed out")
	}

	return nil
}

// expiryWorker periodically expires old leases
func (s *Server) expiryWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	logger.Info().Msg("Expiry worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Expiry worker stopped")
			return
		case <-s.shutdown:
			logger.Info().Msg("Expiry worker stopped")
			return
		case <-ticker.C:
			// Expire old leases
			count, err := s.store.ExpireOldLeases(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to expire old leases")
				continue
			}

			if count > 0 {
				logger.Info().Int64("count", count).Msg("Expired old leases")
			}

			// Delete very old leases (90 days)
			deleted, err := s.store.DeleteOldLeases(ctx, 90*24*time.Hour)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to delete old leases")
				continue
			}

			if deleted > 0 {
				logger.Info().Int64("count", deleted).Msg("Deleted old leases")
			}
		}
	}
}

// cacheCleanupWorker periodically cleans up the cache
func (s *Server) cacheCleanupWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	logger.Info().Msg("Cache cleanup worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Cache cleanup worker stopped")
			return
		case <-s.shutdown:
			logger.Info().Msg("Cache cleanup worker stopped")
			return
		case <-ticker.C:
			// Remove expired entries from cache
			count := s.allocator.cache.ExpireOld()
			if count > 0 {
				logger.Debug().Int("count", count).Msg("Removed expired leases from cache")
			}

			// Log cache stats
			stats := s.allocator.GetCacheStats()
			logger.Debug().
				Int("size", stats.Size).
				Int("max_size", stats.MaxSize).
				Uint64("hits", stats.Hits).
				Uint64("misses", stats.Misses).
				Float64("hit_rate", stats.HitRate).
				Msg("Cache statistics")
		}
	}
}

// findSubnetForRequest determines which subnet a request belongs to
func (s *Server) findSubnetForRequest(req *dhcpv4.DHCPv4) (*SubnetConfig, error) {
	logger.Debug().
		Int("subnet_count", len(s.subnets)).
		Str("giaddr", req.GatewayIPAddr.String()).
		Str("ciaddr", req.ClientIPAddr.String()).
		Msg("Finding subnet for request")

	// Check relay agent IP (GiAddr)
	if !req.GatewayIPAddr.IsUnspecified() {
		logger.Debug().
			Str("giaddr", req.GatewayIPAddr.String()).
			Msg("Checking relay agent IP against subnets")

		for subnetKey, subnet := range s.subnets {
			logger.Debug().
				Str("subnet_key", subnetKey).
				Str("subnet_network", subnet.Network.String()).
				Str("giaddr", req.GatewayIPAddr.String()).
				Bool("contains", subnet.Network.Contains(req.GatewayIPAddr)).
				Msg("Checking subnet match")

			if subnet.Network.Contains(req.GatewayIPAddr) {
				logger.Debug().
					Str("subnet", subnet.Network.String()).
					Msg("Found matching subnet via giaddr")
				return subnet, nil
			}
		}
	}

	// Check client IP (CiAddr) for renewals
	if !req.ClientIPAddr.IsUnspecified() {
		logger.Debug().
			Str("ciaddr", req.ClientIPAddr.String()).
			Msg("Checking client IP against subnets")

		for subnetKey, subnet := range s.subnets {
			logger.Debug().
				Str("subnet_key", subnetKey).
				Str("subnet_network", subnet.Network.String()).
				Str("ciaddr", req.ClientIPAddr.String()).
				Bool("contains", subnet.Network.Contains(req.ClientIPAddr)).
				Msg("Checking subnet match")

			if subnet.Network.Contains(req.ClientIPAddr) {
				logger.Debug().
					Str("subnet", subnet.Network.String()).
					Msg("Found matching subnet via ciaddr")
				return subnet, nil
			}
		}
	}

	// TODO: Check interface IP when we have interface binding
	// For now, return the first subnet if we only have one configured
	if len(s.subnets) == 1 {
		for _, subnet := range s.subnets {
			logger.Debug().
				Str("subnet", subnet.Network.String()).
				Msg("Using single configured subnet")
			return subnet, nil
		}
	}

	logger.Warn().
		Int("subnet_count", len(s.subnets)).
		Str("giaddr", req.GatewayIPAddr.String()).
		Str("ciaddr", req.ClientIPAddr.String()).
		Msg("No matching subnet found")

	return nil, fmt.Errorf("cannot determine subnet for request")
}
