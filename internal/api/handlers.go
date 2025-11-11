package api

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sashakarcz/irondhcp/internal/events"
	"github.com/sashakarcz/irondhcp/internal/logger"
	"github.com/sashakarcz/irondhcp/internal/storage"
)

//go:embed all:dist
var webDist embed.FS

// WebFS is the embedded frontend filesystem
var WebFS fs.FS

func init() {
	var err error
	WebFS, err = fs.Sub(webDist, "dist")
	if err != nil {
		// If dist doesn't exist (dev mode), use a dummy FS
		WebFS = webDist
	}
}

// SPAHandler serves the SPA for all non-API routes
type SPAHandler struct {
	staticFS   http.FileSystem
	indexBytes []byte
}

// NewSPAHandler creates a new SPA handler
func NewSPAHandler(fsys fs.FS) *SPAHandler {
	// Read index.html once at startup
	indexBytes, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		// Fallback to simple message if index.html doesn't exist
		indexBytes = []byte("<html><body><h1>ironDHCP</h1><p>Frontend not built. Run 'make build-web'</p></body></html>")
	}

	return &SPAHandler{
		staticFS:   http.FS(fsys),
		indexBytes: indexBytes,
	}
}

// ServeHTTP serves the SPA
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Remove leading slash for fs.FS (it expects paths without leading /)
	cleanPath := path
	if len(cleanPath) > 0 && cleanPath[0] == '/' {
		cleanPath = cleanPath[1:]
	}

	// Default to index.html for root
	if cleanPath == "" {
		cleanPath = "index.html"
	}

	// Try to open and serve the file from embedded FS
	file, err := WebFS.Open(cleanPath)
	if err == nil {
		defer file.Close()

		// Get file info
		stat, err := file.Stat()
		if err == nil && !stat.IsDir() {
			// File exists and is not a directory - serve it
			http.ServeContent(w, r, cleanPath, stat.ModTime(), file.(interface {
				io.ReadSeeker
			}))
			return
		}
	}

	// File doesn't exist or is a directory, serve index.html (for SPA routing)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(h.indexBytes)
}

// DashboardStatsResponse represents dashboard statistics
type DashboardStatsResponse struct {
	TotalLeases       int    `json:"total_leases"`
	ActiveLeases      int    `json:"active_leases"`
	ExpiredLeases     int    `json:"expired_leases"`
	TotalSubnets      int    `json:"total_subnets"`
	TotalReservations int    `json:"total_reservations"`
	Uptime            string `json:"uptime"`
}

// handleDashboardStats handles dashboard statistics requests
func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get lease stats
	stats, err := s.store.GetLeaseStatistics(ctx)
	if err != nil {
		http.Error(w, "Failed to get lease statistics", http.StatusInternalServerError)
		return
	}

	totalActive := int64(0)
	totalExpired := int64(0)
	for _, stat := range stats {
		totalActive += stat.ActiveLeases
		totalExpired += stat.ExpiredLeases
	}

	// Get reservation count
	reservations, err := s.store.GetAllReservations(ctx)
	if err != nil {
		http.Error(w, "Failed to get reservations", http.StatusInternalServerError)
		return
	}

	response := DashboardStatsResponse{
		TotalLeases:       int(totalActive + totalExpired),
		ActiveLeases:      int(totalActive),
		ExpiredLeases:     int(totalExpired),
		TotalSubnets:      len(stats),
		TotalReservations: len(reservations),
		Uptime:            "N/A", // TODO: Track uptime
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// LeaseResponse represents a lease for API responses
type LeaseResponse struct {
	ID          int64  `json:"id"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	Hostname    string `json:"hostname"`
	Subnet      string `json:"subnet"`
	IssuedAt    string `json:"issued_at"`
	ExpiresAt   string `json:"expires_at"`
	LastSeen    string `json:"last_seen"`
	State       string `json:"state"`
	ClientID    string `json:"client_id"`
	VendorClass string `json:"vendor_class"`
}

// handleLeases handles lease listing requests
func (s *Server) handleLeases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get all dynamic leases
	leases, err := s.store.GetAllLeases(ctx)
	if err != nil {
		http.Error(w, "Failed to get leases", http.StatusInternalServerError)
		return
	}

	// Get all reservations (static leases)
	reservations, err := s.store.GetAllReservations(ctx)
	if err != nil {
		http.Error(w, "Failed to get reservations", http.StatusInternalServerError)
		return
	}

	// Convert to response format (initialize as empty array, not nil)
	response := make([]LeaseResponse, 0)

	// Add dynamic leases
	for _, lease := range leases {
		response = append(response, LeaseResponse{
			ID:          lease.ID,
			IP:          lease.IP.String(),
			MAC:         lease.MAC.String(),
			Hostname:    lease.Hostname,
			Subnet:      lease.Subnet.String(),
			IssuedAt:    lease.IssuedAt.Format(time.RFC3339),
			ExpiresAt:   lease.ExpiresAt.Format(time.RFC3339),
			LastSeen:    lease.LastSeen.Format(time.RFC3339),
			State:       string(lease.State),
			ClientID:    lease.ClientID,
			VendorClass: lease.VendorClass,
		})
	}

	// Add static leases (reservations) that don't have active dynamic leases
	// Create a map of active lease MACs for quick lookup
	activeMacs := make(map[string]bool)
	for _, lease := range leases {
		activeMacs[lease.MAC.String()] = true
	}

	// Add reservations that don't have active leases
	zeroTime := time.Time{}.Format(time.RFC3339)
	for _, res := range reservations {
		// Skip if this MAC already has an active lease
		if activeMacs[res.MAC.String()] {
			continue
		}

		response = append(response, LeaseResponse{
			ID:          res.ID,
			IP:          res.IP.String(),
			MAC:         res.MAC.String(),
			Hostname:    res.Hostname,
			Subnet:      res.Subnet.String(),
			IssuedAt:    zeroTime,
			ExpiresAt:   zeroTime,
			LastSeen:    zeroTime,
			State:       "static",
			ClientID:    "",
			VendorClass: "",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// SubnetResponse represents a subnet for API responses
type SubnetResponse struct {
	Network       string   `json:"network"`
	Description   string   `json:"description"`
	Gateway       string   `json:"gateway"`
	DNSServers    []string `json:"dns_servers"`
	LeaseDuration string   `json:"lease_duration"`
	ActiveLeases  int64    `json:"active_leases"`
	TotalIPs      int      `json:"total_ips"`
	Utilization   float64  `json:"utilization"`
}

// handleSubnets handles subnet listing requests
func (s *Server) handleSubnets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get lease statistics per subnet
	stats, err := s.store.GetLeaseStatistics(ctx)
	if err != nil {
		http.Error(w, "Failed to get subnet statistics", http.StatusInternalServerError)
		return
	}

	// Create map of statistics by subnet
	statsMap := make(map[string]*storage.LeaseStatistics)
	for i := range stats {
		statsMap[stats[i].Subnet.String()] = stats[i]
	}

	// Convert to response format (initialize as empty array, not nil)
	response := make([]SubnetResponse, 0)

	// Get subnets from current config (may be nil if not yet loaded)
	if s.config != nil {
		for _, subnet := range s.config.Subnets {
			// Parse network to calculate total IPs
			_, network, err := net.ParseCIDR(subnet.Network)
			if err != nil {
				continue
			}

			ones, bits := network.Mask.Size()
			totalIPs := 1 << uint(bits-ones) - 2 // Subtract network and broadcast addresses

			// Get statistics for this subnet if available
			activeLeases := int64(0)
			if stat, ok := statsMap[subnet.Network]; ok {
				activeLeases = stat.ActiveLeases
			}

			utilization := float64(0)
			if totalIPs > 0 {
				utilization = (float64(activeLeases) / float64(totalIPs)) * 100
			}

			response = append(response, SubnetResponse{
				Network:       subnet.Network,
				Description:   subnet.Description,
				Gateway:       subnet.Gateway,
				DNSServers:    subnet.DNSServers,
				LeaseDuration: subnet.LeaseDuration.String(),
				ActiveLeases:  activeLeases,
				TotalIPs:      totalIPs,
				Utilization:   utilization,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ReservationResponse represents a reservation for API responses
type ReservationResponse struct {
	ID           int64  `json:"id"`
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	Hostname     string `json:"hostname"`
	Subnet       string `json:"subnet"`
	Description  string `json:"description"`
	TFTPServer   string `json:"tftp_server,omitempty"`
	BootFilename string `json:"boot_filename,omitempty"`
}

// handleReservations handles reservation listing requests
func (s *Server) handleReservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get all reservations
	reservations, err := s.store.GetAllReservations(ctx)
	if err != nil {
		http.Error(w, "Failed to get reservations", http.StatusInternalServerError)
		return
	}

	// Convert to response format (initialize as empty array, not nil)
	response := make([]ReservationResponse, 0)
	for _, res := range reservations {
		response = append(response, ReservationResponse{
			ID:           res.ID,
			MAC:          res.MAC.String(),
			IP:           res.IP.String(),
			Hostname:     res.Hostname,
			Subnet:       res.Subnet.String(),
			Description:  res.Description,
			TFTPServer:   res.TFTPServer,
			BootFilename: res.BootFilename,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleActivityStream handles Server-Sent Events (SSE) for activity log
func (s *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if broadcaster is available
	if s.broadcaster == nil {
		http.Error(w, "Activity stream not available", http.StatusServiceUnavailable)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Generate client ID
	clientID := uuid.New().String()

	// Register client with broadcaster
	client := s.broadcaster.Register(clientID)
	defer s.broadcaster.Unregister(client)

	// Create a context that gets cancelled when client disconnects
	ctx := r.Context()

	// Send initial connection event
	initialEvent := &events.ActivityEvent{
		ID:        "init",
		Timestamp: time.Now(),
		Type:      "connection",
		Message:   "Connected to activity stream",
	}

	data, _ := events.FormatSSE(initialEvent)
	w.Write(data)

	// Flush to ensure client receives the initial event
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Stream events to client
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return

		case event, ok := <-client.Channel:
			if !ok {
				// Channel closed
				return
			}

			// Format and send event
			data, err := events.FormatSSE(event)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to format SSE event")
				continue
			}

			if _, err := w.Write(data); err != nil {
				// Error writing to client, they probably disconnected
				return
			}

			// Flush immediately to send event to client
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
