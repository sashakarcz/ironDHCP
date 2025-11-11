package gitops

import (
	"context"
	"time"

	"github.com/yourusername/irondhcp/internal/logger"
	"github.com/yourusername/irondhcp/internal/storage"
)

// Poller handles periodic Git repository polling
type Poller struct {
	syncService  *SyncService
	pollInterval time.Duration
	stopChan     chan struct{}
	doneChan     chan struct{}
}

// NewPoller creates a new Git repository poller
func NewPoller(syncService *SyncService, pollInterval time.Duration) *Poller {
	return &Poller{
		syncService:  syncService,
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start begins the polling loop
func (p *Poller) Start(ctx context.Context) error {
	logger.Info().
		Dur("interval", p.pollInterval).
		Msg("Starting Git repository poller")

	// Perform initial sync
	logger.Info().Msg("Performing initial Git sync")
	if _, err := p.syncService.Sync(ctx, storage.GitSyncTriggerStartup, ""); err != nil {
		logger.Error().Err(err).Msg("Initial Git sync failed")
		// Don't fail startup if initial sync fails
	}

	// Start polling loop in background
	go p.pollLoop(ctx)

	return nil
}

// Stop stops the polling loop
func (p *Poller) Stop(ctx context.Context) error {
	logger.Info().Msg("Stopping Git repository poller")

	close(p.stopChan)

	// Wait for polling loop to finish with timeout
	select {
	case <-p.doneChan:
		logger.Info().Msg("Git repository poller stopped")
		return nil
	case <-ctx.Done():
		logger.Warn().Msg("Git repository poller stop timed out")
		return ctx.Err()
	}
}

// pollLoop is the main polling loop
func (p *Poller) pollLoop(ctx context.Context) {
	defer close(p.doneChan)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			logger.Debug().Msg("Git poller received stop signal")
			return

		case <-ctx.Done():
			logger.Debug().Msg("Git poller context cancelled")
			return

		case <-ticker.C:
			logger.Debug().Msg("Git poller tick - checking for updates")
			p.performSync(ctx)
		}
	}
}

// performSync performs a synchronization
func (p *Poller) performSync(ctx context.Context) {
	result, err := p.syncService.Sync(ctx, storage.GitSyncTriggerPoll, "")
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Git sync failed during polling")
		return
	}

	if result.HasChanges {
		logger.Info().
			Str("commit", result.CommitInfo.Hash).
			Interface("changes", result.ChangesApplied).
			Msg("Applied changes from Git repository")
	} else {
		logger.Debug().Msg("No changes detected in Git repository")
	}
}

// TriggerSync manually triggers a sync operation
func (p *Poller) TriggerSync(ctx context.Context, triggeredByUser string) (*SyncResult, error) {
	logger.Info().
		Str("user", triggeredByUser).
		Msg("Manual Git sync triggered")

	return p.syncService.Sync(ctx, storage.GitSyncTriggerManual, triggeredByUser)
}
