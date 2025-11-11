package dhcp

import (
	"context"
	"time"

	"github.com/yourusername/irondhcp/internal/logger"
	"github.com/yourusername/irondhcp/internal/storage"
)

// ExpiryWorker handles periodic lease expiration checks
type ExpiryWorker struct {
	store        *storage.Store
	checkInterval time.Duration
	stopChan     chan struct{}
	doneChan     chan struct{}
}

// NewExpiryWorker creates a new lease expiry worker
func NewExpiryWorker(store *storage.Store, checkInterval time.Duration) *ExpiryWorker {
	return &ExpiryWorker{
		store:        store,
		checkInterval: checkInterval,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start begins the expiry check loop
func (w *ExpiryWorker) Start(ctx context.Context) error {
	logger.Info().
		Dur("interval", w.checkInterval).
		Msg("Starting lease expiry worker")

	// Perform initial check
	if err := w.expireLeases(ctx); err != nil {
		logger.Error().Err(err).Msg("Initial lease expiry check failed")
	}

	// Start expiry loop in background
	go w.expiryLoop(ctx)

	return nil
}

// Stop stops the expiry check loop
func (w *ExpiryWorker) Stop(ctx context.Context) error {
	logger.Info().Msg("Stopping lease expiry worker")

	close(w.stopChan)

	// Wait for expiry loop to finish with timeout
	select {
	case <-w.doneChan:
		logger.Info().Msg("Lease expiry worker stopped")
		return nil
	case <-ctx.Done():
		logger.Warn().Msg("Lease expiry worker stop timed out")
		return ctx.Err()
	}
}

// expiryLoop is the main expiry check loop
func (w *ExpiryWorker) expiryLoop(ctx context.Context) {
	defer close(w.doneChan)

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			logger.Debug().Msg("Expiry worker received stop signal")
			return

		case <-ctx.Done():
			logger.Debug().Msg("Expiry worker context cancelled")
			return

		case <-ticker.C:
			logger.Debug().Msg("Expiry worker tick - checking for expired leases")
			if err := w.expireLeases(ctx); err != nil {
				logger.Error().Err(err).Msg("Failed to expire leases")
			}
		}
	}
}

// expireLeases marks expired leases as expired
func (w *ExpiryWorker) expireLeases(ctx context.Context) error {
	count, err := w.store.ExpireLeases(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		logger.Info().
			Int64("count", count).
			Msg("Expired leases")
	}

	return nil
}
