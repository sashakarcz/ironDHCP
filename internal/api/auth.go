package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sashakarcz/irondhcp/internal/config"
	"github.com/sashakarcz/irondhcp/internal/logger"
)

// AuthManager handles authentication
type AuthManager struct {
	config *config.WebAuth
	tokens map[string]*TokenInfo
	mu     sync.RWMutex
}

// TokenInfo holds token metadata
type TokenInfo struct {
	Username  string
	ExpiresAt time.Time
}

// NewAuthManager creates a new auth manager
func NewAuthManager(cfg *config.WebAuth) *AuthManager {
	return &AuthManager{
		config: cfg,
		tokens: make(map[string]*TokenInfo),
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
}

// handleLogin handles login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If auth is disabled, return a dummy token
	if !s.authManager.config.Enabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Token:   "no-auth-required",
			Message: "Authentication disabled",
		})
		return
	}

	// Parse request
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid request",
		})
		return
	}

	// Validate credentials
	if !s.authManager.ValidateCredentials(req.Username, req.Password) {
		logger.Warn().
			Str("username", req.Username).
			Str("ip", r.RemoteAddr).
			Msg("Failed login attempt")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Generate token
	token, err := s.authManager.GenerateToken(req.Username)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to generate token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Internal server error",
		})
		return
	}

	logger.Info().
		Str("username", req.Username).
		Str("ip", r.RemoteAddr).
		Msg("Successful login")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{
		Success: true,
		Token:   token,
		Message: "Login successful",
	})
}

// ValidateCredentials validates username and password
func (am *AuthManager) ValidateCredentials(username, password string) bool {
	if username != am.config.Username {
		return false
	}

	// If no password hash is configured, allow any password for the correct username
	// This is for development/testing only
	if am.config.PasswordHash == "" {
		logger.Warn().Msg("No password hash configured - allowing any password (INSECURE)")
		return true
	}

	// Hash the provided password and compare
	hashedPassword := HashPassword(password)
	return hashedPassword == am.config.PasswordHash
}

// GenerateToken generates a new authentication token
func (am *AuthManager) GenerateToken(username string) (string, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Store token with expiration (24 hours)
	am.mu.Lock()
	am.tokens[token] = &TokenInfo{
		Username:  username,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	am.mu.Unlock()

	// Clean up expired tokens
	go am.cleanupExpiredTokens()

	return token, nil
}

// ValidateToken validates an authentication token
func (am *AuthManager) ValidateToken(token string) bool {
	// If auth is disabled, accept any token
	if !am.config.Enabled {
		return true
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	info, exists := am.tokens[token]
	if !exists {
		return false
	}

	// Check if token is expired
	if time.Now().After(info.ExpiresAt) {
		return false
	}

	return true
}

// cleanupExpiredTokens removes expired tokens
func (am *AuthManager) cleanupExpiredTokens() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	for token, info := range am.tokens {
		if now.After(info.ExpiresAt) {
			delete(am.tokens, token)
		}
	}
}

// AuthMiddleware is middleware that checks authentication
func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, allow all requests
		if !s.authManager.config.Enabled {
			next(w, r)
			return
		}

		var token string

		// Try to get token from Authorization header first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			// Extract token (format: "Bearer <token>")
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// If no token in header, try query parameter (for SSE compatibility)
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		// If still no token, reject
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate token
		if !s.authManager.ValidateToken(token) {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// HashPassword hashes a password using SHA-256
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", hash)
}
