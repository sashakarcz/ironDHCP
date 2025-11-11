package storage

import (
	"net"
	"time"
)

// LeaseState represents the state of a DHCP lease
type LeaseState string

const (
	LeaseStateActive   LeaseState = "active"
	LeaseStateExpired  LeaseState = "expired"
	LeaseStateReleased LeaseState = "released"
	LeaseStateDeclined LeaseState = "declined"
)

// Lease represents a DHCP lease record
type Lease struct {
	ID          int64
	IP          net.IP
	MAC         net.HardwareAddr
	Hostname    string
	Subnet      *net.IPNet
	IssuedAt    time.Time
	ExpiresAt   time.Time
	LastSeen    time.Time
	State       LeaseState
	ClientID    string
	VendorClass string
	UserClass   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsActive returns true if the lease is active and not expired
func (l *Lease) IsActive() bool {
	return l.State == LeaseStateActive && l.ExpiresAt.After(time.Now())
}

// Reservation represents a static IP reservation
type Reservation struct {
	ID           int64
	MAC          net.HardwareAddr
	IP           net.IP
	Hostname     string
	Subnet       *net.IPNet
	Description  string
	TFTPServer   string // DHCP option 66
	BootFilename string // DHCP option 67
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GitSyncStatus represents the status of a Git sync operation
type GitSyncStatus string

const (
	GitSyncStatusInProgress GitSyncStatus = "in_progress"
	GitSyncStatusSuccess    GitSyncStatus = "success"
	GitSyncStatusFailed     GitSyncStatus = "failed"
)

// GitSyncTrigger represents the source of a Git sync trigger
type GitSyncTrigger string

const (
	GitSyncTriggerPoll    GitSyncTrigger = "poll"
	GitSyncTriggerManual  GitSyncTrigger = "manual"
	GitSyncTriggerStartup GitSyncTrigger = "startup"
)

// GitSyncLog represents a Git synchronization event
type GitSyncLog struct {
	ID              int64
	SyncStartedAt   time.Time
	SyncCompletedAt *time.Time
	Status          GitSyncStatus
	CommitHash      string
	CommitMessage   string
	CommitAuthor    string
	CommitTimestamp *time.Time
	ErrorMessage    string
	ChangesApplied  map[string]interface{} // JSON data
	TriggeredBy     GitSyncTrigger
	TriggeredByUser string
	CreatedAt       time.Time
}

// ActiveConfig represents the currently active configuration
type ActiveConfig struct {
	ID         int
	CommitHash string
	AppliedAt  time.Time
	ConfigYAML string
}

// LeaseStatistics represents aggregated lease statistics per subnet
type LeaseStatistics struct {
	Subnet         *net.IPNet
	ActiveLeases   int64
	ExpiredLeases  int64
	ReleasedLeases int64
	DeclinedLeases int64
	NextExpiry     *time.Time
	LastActivity   *time.Time
}
