package gitops

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/irondhcp/internal/config"
	"github.com/yourusername/irondhcp/internal/logger"
	"github.com/yourusername/irondhcp/internal/storage"
)

// SyncService manages configuration synchronization from Git
type SyncService struct {
	repo        *Repository
	store       *storage.Store
	reloadFunc  func(*config.Config) error
	currentHash string
}

// SyncResult contains the result of a sync operation
type SyncResult struct {
	Success       bool
	HasChanges    bool
	CommitInfo    *CommitInfo
	ErrorMessage  string
	ChangesApplied map[string]interface{}
}

// NewSyncService creates a new sync service
func NewSyncService(repo *Repository, store *storage.Store, reloadFunc func(*config.Config) error) *SyncService {
	return &SyncService{
		repo:       repo,
		store:      store,
		reloadFunc: reloadFunc,
	}
}

// Sync performs a complete sync operation: pull, validate, and apply
func (s *SyncService) Sync(ctx context.Context, trigger storage.GitSyncTrigger, triggeredByUser string) (*SyncResult, error) {
	// Create sync log entry
	syncLog := &storage.GitSyncLog{
		SyncStartedAt:   time.Now(),
		Status:          storage.GitSyncStatusInProgress,
		TriggeredBy:     trigger,
		TriggeredByUser: triggeredByUser,
	}

	if err := s.store.CreateGitSyncLog(ctx, syncLog); err != nil {
		logger.Error().Err(err).Msg("Failed to create git sync log")
		return nil, fmt.Errorf("failed to create git sync log: %w", err)
	}

	result := &SyncResult{
		ChangesApplied: make(map[string]interface{}),
	}

	// Pull latest changes
	logger.Info().Msg("Pulling latest changes from Git repository")
	commitInfo, hasChanges, err := s.repo.Pull(ctx)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to pull from repository: %v", err)
		s.finalizeSyncLog(ctx, syncLog, result)
		return result, fmt.Errorf("failed to pull from repository: %w", err)
	}

	result.CommitInfo = commitInfo
	result.HasChanges = hasChanges

	// Update sync log with commit info
	syncLog.CommitHash = commitInfo.Hash
	syncLog.CommitMessage = commitInfo.Message
	syncLog.CommitAuthor = commitInfo.Author
	syncLog.CommitTimestamp = &commitInfo.Timestamp

	// If no changes, we're done
	if !hasChanges && s.currentHash == commitInfo.Hash {
		logger.Info().
			Str("commit", commitInfo.Hash).
			Msg("No changes in Git repository, skipping sync")

		result.Success = true
		syncLog.Status = storage.GitSyncStatusSuccess
		s.finalizeSyncLog(ctx, syncLog, result)
		return result, nil
	}

	// Validate configuration
	logger.Info().Msg("Validating configuration from Git")
	configPath := s.repo.GetConfigFilePath()
	newConfig, err := s.validateConfig(configPath)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Configuration validation failed: %v", err)
		s.finalizeSyncLog(ctx, syncLog, result)
		return result, fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Info().Msg("Configuration validated successfully")

	// Apply configuration atomically
	logger.Info().Msg("Applying new configuration")
	if err := s.applyConfig(ctx, newConfig, result); err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to apply configuration: %v", err)
		s.finalizeSyncLog(ctx, syncLog, result)
		return result, fmt.Errorf("failed to apply configuration: %w", err)
	}

	// Update current hash
	s.currentHash = commitInfo.Hash

	result.Success = true
	syncLog.Status = storage.GitSyncStatusSuccess

	logger.Info().
		Str("commit", commitInfo.Hash).
		Msg("Successfully synced configuration from Git")

	s.finalizeSyncLog(ctx, syncLog, result)
	return result, nil
}

// validateConfig validates a configuration file
func (s *SyncService) validateConfig(configPath string) (*config.Config, error) {
	// Check if file exists
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("config file not found: %w", err)
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate config
	if err := s.validateConfigStructure(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validateConfigStructure performs structural validation on the config
func (s *SyncService) validateConfigStructure(cfg *config.Config) error {
	// Validate subnets
	if len(cfg.Subnets) == 0 {
		return fmt.Errorf("no subnets defined")
	}

	// Validate each subnet
	for i, subnet := range cfg.Subnets {
		if subnet.Network == "" {
			return fmt.Errorf("subnet %d: network is required", i)
		}

		if len(subnet.Pools) == 0 && len(subnet.Reservations) == 0 {
			logger.Warn().
				Str("subnet", subnet.Network).
				Msg("Subnet has no pools or reservations")
		}

		// Validate pools
		for j, pool := range subnet.Pools {
			if pool.RangeStart == "" || pool.RangeEnd == "" {
				return fmt.Errorf("subnet %d, pool %d: range_start and range_end are required", i, j)
			}
		}

		// Validate reservations
		for j, res := range subnet.Reservations {
			if res.MAC == "" {
				return fmt.Errorf("subnet %d, reservation %d: MAC address is required", i, j)
			}
			if res.IP == "" {
				return fmt.Errorf("subnet %d, reservation %d: IP address is required", i, j)
			}
		}
	}

	// Validate database config
	if cfg.Database.Connection == "" {
		return fmt.Errorf("database connection string is required")
	}

	return nil
}

// applyConfig applies the new configuration atomically
func (s *SyncService) applyConfig(ctx context.Context, newConfig *config.Config, result *SyncResult) error {
	// Track changes
	changes := make(map[string]interface{})

	// Sync reservations to database
	logger.Info().Msg("Syncing reservations to database")

	// Get existing reservations
	existing, err := s.store.GetAllReservations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get existing reservations: %w", err)
	}

	existingMap := make(map[string]*storage.Reservation)
	for _, res := range existing {
		existingMap[res.MAC.String()] = res
	}

	reservationsAdded := 0
	reservationsUpdated := 0
	reservationsDeleted := 0

	// Add/update reservations from config
	for _, subnetCfg := range newConfig.Subnets {
		_, network, _ := config.ParseCIDR(subnetCfg.Network)

		for _, resCfg := range subnetCfg.Reservations {
			mac, _ := config.ParseMAC(resCfg.MAC)
			ip := config.ParseIP(resCfg.IP)

			var tftpServer, bootFilename string
			if resCfg.Boot != nil {
				tftpServer = resCfg.Boot.TFTPServer
				bootFilename = resCfg.Boot.Filename
			}

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

					if err := s.store.UpdateReservation(ctx, existingRes); err != nil {
						logger.Error().Err(err).Str("mac", mac.String()).Msg("Failed to update reservation")
					} else {
						reservationsUpdated++
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

				if err := s.store.CreateReservation(ctx, newRes); err != nil {
					logger.Error().Err(err).Str("mac", mac.String()).Msg("Failed to create reservation")
				} else {
					reservationsAdded++
				}
			}
		}
	}

	// Delete reservations not in config
	for _, res := range existingMap {
		if err := s.store.DeleteReservation(ctx, res.ID); err != nil {
			logger.Error().Err(err).Str("mac", res.MAC.String()).Msg("Failed to delete reservation")
		} else {
			reservationsDeleted++
		}
	}

	changes["reservations_added"] = reservationsAdded
	changes["reservations_updated"] = reservationsUpdated
	changes["reservations_deleted"] = reservationsDeleted
	changes["total_subnets"] = len(newConfig.Subnets)

	logger.Info().
		Int("added", reservationsAdded).
		Int("updated", reservationsUpdated).
		Int("deleted", reservationsDeleted).
		Msg("Synced reservations")

	// Call reload function to reload DHCP server configuration
	if s.reloadFunc != nil {
		logger.Info().Msg("Reloading DHCP server configuration")
		if err := s.reloadFunc(newConfig); err != nil {
			return fmt.Errorf("failed to reload configuration: %w", err)
		}
		changes["config_reloaded"] = true
	}

	result.ChangesApplied = changes
	return nil
}

// finalizeSyncLog updates the sync log with final status
func (s *SyncService) finalizeSyncLog(ctx context.Context, syncLog *storage.GitSyncLog, result *SyncResult) {
	now := time.Now()
	syncLog.SyncCompletedAt = &now

	if result.Success {
		syncLog.Status = storage.GitSyncStatusSuccess
	} else {
		syncLog.Status = storage.GitSyncStatusFailed
		syncLog.ErrorMessage = result.ErrorMessage
	}

	syncLog.ChangesApplied = result.ChangesApplied

	if err := s.store.UpdateGitSyncLog(ctx, syncLog); err != nil {
		logger.Error().Err(err).Msg("Failed to update git sync log")
	}
}

// GetCurrentCommitHash returns the current commit hash
func (s *SyncService) GetCurrentCommitHash() string {
	return s.currentHash
}
