package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yourusername/irondhcp/internal/logger"
)

// EventType represents the type of activity event
type EventType string

const (
	EventTypeDHCPDiscover EventType = "dhcp_discover"
	EventTypeDHCPOffer    EventType = "dhcp_offer"
	EventTypeDHCPRequest  EventType = "dhcp_request"
	EventTypeDHCPAck      EventType = "dhcp_ack"
	EventTypeDHCPNak      EventType = "dhcp_nak"
	EventTypeDHCPRelease  EventType = "dhcp_release"
	EventTypeDHCPDecline  EventType = "dhcp_decline"
	EventTypeLeaseExpired EventType = "lease_expired"
	EventTypeGitSync      EventType = "git_sync"
)

// ActivityEvent represents a single activity log event
type ActivityEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      EventType              `json:"type"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// Client represents an SSE client connection
type Client struct {
	ID      string
	Channel chan *ActivityEvent
}

// Broadcaster manages SSE clients and broadcasts events
type Broadcaster struct {
	clients      map[string]*Client
	register     chan *Client
	unregister   chan *Client
	broadcast    chan *ActivityEvent
	mu           sync.RWMutex
	eventCounter int64
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *ActivityEvent, 100),
	}
}

// Start starts the broadcaster
func (b *Broadcaster) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Close all client channels
				b.mu.Lock()
				for _, client := range b.clients {
					close(client.Channel)
				}
				b.clients = make(map[string]*Client)
				b.mu.Unlock()
				return

			case client := <-b.register:
				b.mu.Lock()
				b.clients[client.ID] = client
				b.mu.Unlock()
				logger.Debug().
					Str("client_id", client.ID).
					Int("total_clients", len(b.clients)).
					Msg("SSE client connected")

			case client := <-b.unregister:
				b.mu.Lock()
				if _, ok := b.clients[client.ID]; ok {
					close(client.Channel)
					delete(b.clients, client.ID)
				}
				b.mu.Unlock()
				logger.Debug().
					Str("client_id", client.ID).
					Int("total_clients", len(b.clients)).
					Msg("SSE client disconnected")

			case event := <-b.broadcast:
				b.mu.RLock()
				for _, client := range b.clients {
					select {
					case client.Channel <- event:
					default:
						// Client channel is full, skip this event for this client
						logger.Warn().
							Str("client_id", client.ID).
							Msg("Client channel full, skipping event")
					}
				}
				b.mu.RUnlock()
			}
		}
	}()
}

// Register registers a new SSE client
func (b *Broadcaster) Register(clientID string) *Client {
	client := &Client{
		ID:      clientID,
		Channel: make(chan *ActivityEvent, 10),
	}
	b.register <- client
	return client
}

// Unregister unregisters an SSE client
func (b *Broadcaster) Unregister(client *Client) {
	b.unregister <- client
}

// Broadcast sends an event to all connected clients
func (b *Broadcaster) Broadcast(event *ActivityEvent) {
	select {
	case b.broadcast <- event:
	default:
		// Broadcast channel is full, log warning
		logger.Warn().Msg("Broadcast channel full, dropping event")
	}
}

// BroadcastDHCPEvent broadcasts a DHCP-related activity event
func (b *Broadcaster) BroadcastDHCPEvent(eventType EventType, ip net.IP, mac net.HardwareAddr, hostname string, details map[string]interface{}) {
	b.mu.Lock()
	b.eventCounter++
	eventID := fmt.Sprintf("evt-%d", b.eventCounter)
	b.mu.Unlock()

	message := fmt.Sprintf("%s: %s (%s)", eventType, ip, mac)
	if hostname != "" {
		message = fmt.Sprintf("%s: %s (%s) - %s", eventType, ip, mac, hostname)
	}

	event := &ActivityEvent{
		ID:        eventID,
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
		Details: map[string]interface{}{
			"ip":       ip.String(),
			"mac":      mac.String(),
			"hostname": hostname,
		},
	}

	// Add additional details
	for k, v := range details {
		event.Details[k] = v
	}

	b.Broadcast(event)
}

// BroadcastGitSyncEvent broadcasts a Git sync activity event
func (b *Broadcaster) BroadcastGitSyncEvent(success bool, commitHash, commitMessage string, details map[string]interface{}) {
	b.mu.Lock()
	b.eventCounter++
	eventID := fmt.Sprintf("evt-%d", b.eventCounter)
	b.mu.Unlock()

	message := "Git sync completed"
	if !success {
		message = "Git sync failed"
	}
	if commitMessage != "" {
		message = fmt.Sprintf("%s: %s", message, commitMessage)
	}

	event := &ActivityEvent{
		ID:        eventID,
		Timestamp: time.Now(),
		Type:      EventTypeGitSync,
		Message:   message,
		Details: map[string]interface{}{
			"success":        success,
			"commit_hash":    commitHash,
			"commit_message": commitMessage,
		},
	}

	// Add additional details
	for k, v := range details {
		event.Details[k] = v
	}

	b.Broadcast(event)
}

// FormatSSE formats an event as SSE message
func FormatSSE(event *ActivityEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("id: %s\ndata: %s\n\n", event.ID, data)), nil
}
