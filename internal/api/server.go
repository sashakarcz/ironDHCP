package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yourusername/irondhcp/internal/config"
	"github.com/yourusername/irondhcp/internal/events"
	"github.com/yourusername/irondhcp/internal/gitops"
	"github.com/yourusername/irondhcp/internal/logger"
	"github.com/yourusername/irondhcp/internal/storage"
)

// Server provides HTTP API and health check endpoints
type Server struct {
	store       *storage.Store
	poller      *gitops.Poller
	broadcaster *events.Broadcaster
	authManager *AuthManager
	httpServer  *http.Server
	port        int
}

// Config holds API server configuration
type Config struct {
	Port    int
	Enabled bool
	WebAuth *config.WebAuth
}

// New creates a new API server
func New(cfg Config, store *storage.Store, poller *gitops.Poller, broadcaster *events.Broadcaster) *Server {
	return &Server{
		store:       store,
		poller:      poller,
		broadcaster: broadcaster,
		authManager: NewAuthManager(cfg.WebAuth),
		port:        cfg.Port,
	}
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Public endpoints (no auth required)
	mux.HandleFunc("/api/v1/login", s.handleLogin)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/health", s.handleHealth) // Alias for Docker healthcheck

	// Protected endpoints (require auth if enabled)
	mux.HandleFunc("/api/v1/dashboard/stats", s.AuthMiddleware(s.handleDashboardStats))
	mux.HandleFunc("/api/v1/leases", s.AuthMiddleware(s.handleLeases))
	mux.HandleFunc("/api/v1/subnets", s.AuthMiddleware(s.handleSubnets))
	mux.HandleFunc("/api/v1/reservations", s.AuthMiddleware(s.handleReservations))
	mux.HandleFunc("/api/v1/git/sync", s.AuthMiddleware(s.handleGitSync))
	mux.HandleFunc("/api/v1/git/status", s.AuthMiddleware(s.handleGitStatus))
	mux.HandleFunc("/api/v1/git/logs", s.AuthMiddleware(s.handleGitLogs))
	mux.HandleFunc("/api/v1/activity/stream", s.AuthMiddleware(s.handleActivityStream))

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Serve frontend (SPA with client-side routing)
	spaHandler := NewSPAHandler(WebFS)
	mux.Handle("/", spaHandler)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logger.Info().
		Int("port", s.port).
		Msg("Starting API server")

	// Start server in goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("API server error")
		}
	}()

	return nil
}

// Stop stops the API server
func (s *Server) Stop(ctx context.Context) error {
	logger.Info().Msg("Stopping API server")

	if s.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown API server: %w", err)
	}

	logger.Info().Msg("API server stopped")
	return nil
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status   string                 `json:"status"`
	Database DatabaseHealth         `json:"database"`
	Cache    CacheHealth            `json:"cache,omitempty"`
	Time     string                 `json:"time"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// DatabaseHealth represents database health status
type DatabaseHealth struct {
	Status      string `json:"status"`
	Connections int    `json:"connections"`
	MaxConns    int32  `json:"max_conns"`
}

// CacheHealth represents cache health status
type CacheHealth struct {
	Size    int     `json:"size"`
	HitRate float64 `json:"hit_rate"`
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	health := HealthResponse{
		Status: "healthy",
		Time:   time.Now().UTC().Format(time.RFC3339),
	}

	// Check database
	if err := s.store.Health(ctx); err != nil {
		health.Status = "unhealthy"
		health.Database.Status = "unhealthy"
		health.Details = map[string]interface{}{
			"database_error": err.Error(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(health)
		return
	}

	// Get database stats
	stats := s.store.Stats()
	health.Database = DatabaseHealth{
		Status:      "healthy",
		Connections: int(stats.AcquiredConns()),
		MaxConns:    stats.MaxConns(),
	}

	// Return healthy response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

// GitSyncRequest represents a git sync trigger request
type GitSyncRequest struct {
	TriggeredBy string `json:"triggered_by"`
}

// GitSyncResponse represents a git sync response
type GitSyncResponse struct {
	Success        bool                   `json:"success"`
	Message        string                 `json:"message"`
	CommitHash     string                 `json:"commit_hash,omitempty"`
	CommitMessage  string                 `json:"commit_message,omitempty"`
	HasChanges     bool                   `json:"has_changes"`
	ChangesApplied map[string]interface{} `json:"changes_applied,omitempty"`
}

// GitStatusResponse represents git repository status
type GitStatusResponse struct {
	CurrentCommit  string    `json:"current_commit"`
	CommitMessage  string    `json:"commit_message,omitempty"`
	CommitAuthor   string    `json:"commit_author,omitempty"`
	CommitTime     time.Time `json:"commit_time,omitempty"`
	LastSyncTime   time.Time `json:"last_sync_time,omitempty"`
	LastSyncStatus string    `json:"last_sync_status,omitempty"`
}

// handleGitSync handles manual git sync trigger requests
func (s *Server) handleGitSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req GitSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.TriggeredBy == "" {
		req.TriggeredBy = "api"
	}

	// Check if poller is configured
	if s.poller == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(GitSyncResponse{
			Success: false,
			Message: "GitOps is not enabled",
		})
		return
	}

	// Trigger sync
	ctx := r.Context()
	result, err := s.poller.TriggerSync(ctx, req.TriggeredBy)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(GitSyncResponse{
			Success: false,
			Message: fmt.Sprintf("Sync failed: %v", err),
		})
		return
	}

	response := GitSyncResponse{
		Success:        result.Success,
		HasChanges:     result.HasChanges,
		ChangesApplied: result.ChangesApplied,
	}

	if result.CommitInfo != nil {
		response.CommitHash = result.CommitInfo.Hash
		response.CommitMessage = result.CommitInfo.Message
	}

	if result.Success {
		response.Message = "Sync completed successfully"
	} else {
		response.Message = result.ErrorMessage
	}

	w.Header().Set("Content-Type", "application/json")
	if result.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(response)
}

// handleGitStatus handles git repository status requests
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get last successful sync
	lastSync, err := s.store.GetLastSuccessfulSync(ctx)
	if err != nil {
		// No sync found yet
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(GitStatusResponse{})
		return
	}

	response := GitStatusResponse{
		CurrentCommit:  lastSync.CommitHash,
		CommitMessage:  lastSync.CommitMessage,
		CommitAuthor:   lastSync.CommitAuthor,
		LastSyncStatus: string(lastSync.Status),
	}

	if lastSync.CommitTimestamp != nil {
		response.CommitTime = *lastSync.CommitTimestamp
	}

	if lastSync.SyncCompletedAt != nil {
		response.LastSyncTime = *lastSync.SyncCompletedAt
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GitLogEntry represents a git sync log entry
type GitLogEntry struct {
	ID              int64                  `json:"id"`
	SyncStartedAt   time.Time              `json:"sync_started_at"`
	SyncCompletedAt *time.Time             `json:"sync_completed_at,omitempty"`
	Status          string                 `json:"status"`
	CommitHash      string                 `json:"commit_hash"`
	CommitMessage   string                 `json:"commit_message,omitempty"`
	CommitAuthor    string                 `json:"commit_author,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	ChangesApplied  map[string]interface{} `json:"changes_applied,omitempty"`
	TriggeredBy     string                 `json:"triggered_by"`
	TriggeredByUser string                 `json:"triggered_by_user,omitempty"`
}

// GitLogsResponse represents git sync logs response
type GitLogsResponse struct {
	Logs []GitLogEntry `json:"logs"`
}

// handleGitLogs handles git sync logs requests
func (s *Server) handleGitLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get recent sync logs (limit to 50)
	logs, err := s.store.GetRecentGitSyncLogs(ctx, 50)
	if err != nil {
		http.Error(w, "Failed to get sync logs", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	var entries []GitLogEntry
	for _, log := range logs {
		entry := GitLogEntry{
			ID:              log.ID,
			SyncStartedAt:   log.SyncStartedAt,
			SyncCompletedAt: log.SyncCompletedAt,
			Status:          string(log.Status),
			CommitHash:      log.CommitHash,
			CommitMessage:   log.CommitMessage,
			CommitAuthor:    log.CommitAuthor,
			ErrorMessage:    log.ErrorMessage,
			ChangesApplied:  log.ChangesApplied,
			TriggeredBy:     string(log.TriggeredBy),
			TriggeredByUser: log.TriggeredByUser,
		}
		entries = append(entries, entry)
	}

	response := GitLogsResponse{
		Logs: entries,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
