package dhcp

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"time"

	"github.com/sashakarcz/irondhcp/internal/storage"
)

// Allocator handles IP address allocation using LRU algorithm
type Allocator struct {
	store    *storage.Store
	cache    *storage.LeaseCache
	serverID string // Server ID for tracking allocations in HA deployments
	useCache bool   // Whether to use read-only cache (optional optimization)
}

// NewAllocator creates a new IP allocator
func NewAllocator(store *storage.Store, cacheSize int, serverID string, useCache bool) *Allocator {
	return &Allocator{
		store:    store,
		cache:    storage.NewLeaseCache(cacheSize),
		serverID: serverID,
		useCache: useCache,
	}
}

// AllocateIP allocates an IP address for a client using LRU algorithm
// Priority:
// 1. Check for existing active lease for this MAC
// 2. Check for static reservation for this MAC
// 3. Allocate from pool (LRU: expired leases first, then never-used IPs)
func (a *Allocator) AllocateIP(ctx context.Context, req *AllocationRequest) (*storage.Lease, error) {
	// Step 1: Always check database first (source of truth for HA deployments)
	// Cache is NOT used for allocation decisions, only as read-only optimization
	lease, err := a.store.GetLeaseByMAC(ctx, req.MAC, req.Subnet)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing lease: %w", err)
	}
	if lease != nil && lease.IsActive() {
		// Optionally update read-only cache for performance
		if a.useCache {
			a.cache.Put(lease)
		}
		return lease, nil
	}

	// Step 2: Check for static reservation
	reservation, err := a.store.GetReservationByMAC(ctx, req.MAC)
	if err != nil {
		return nil, fmt.Errorf("failed to check reservation: %w", err)
	}
	if reservation != nil && reservation.Subnet.String() == req.Subnet.String() {
		// Create lease for reserved IP
		return a.createLeaseForReservation(ctx, req, reservation)
	}

	// Step 3: Allocate from pool using LRU
	for _, pool := range req.Pools {
		lease, err := a.allocateFromPool(ctx, req, pool)
		if err != nil {
			continue // Try next pool
		}
		if lease != nil {
			return lease, nil
		}
	}

	return nil, fmt.Errorf("no available IPs in any pool")
}

// allocateFromPool attempts to allocate an IP from a specific pool
func (a *Allocator) allocateFromPool(ctx context.Context, req *AllocationRequest, pool *PoolConfig) (*storage.Lease, error) {
	rangeStart := net.ParseIP(pool.RangeStart).To4()
	rangeEnd := net.ParseIP(pool.RangeEnd).To4()

	// First, try to find expired leases in this pool (LRU)
	expiredLeases, err := a.store.GetExpiredLeases(ctx, req.Subnet, rangeStart, rangeEnd, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get expired leases: %w", err)
	}

	// Try to claim an expired lease with advisory lock
	for _, expired := range expiredLeases {
		lockKey := getAdvisoryLockKey(expired.IP, req.Subnet)
		err := a.store.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
			// Check if IP is still available
			existing, err := a.store.GetLeaseByIP(ctx, expired.IP, req.Subnet)
			if err != nil {
				return err
			}
			if existing != nil && existing.State == storage.LeaseStateActive && existing.ExpiresAt.After(time.Now()) {
				// Someone else claimed it
				return fmt.Errorf("IP already claimed")
			}

			// Check if IP is reserved
			reservation, err := a.store.GetReservationByIP(ctx, expired.IP, req.Subnet)
			if err != nil {
				return err
			}
			if reservation != nil && reservation.MAC.String() != req.MAC.String() {
				// Reserved for someone else
				return fmt.Errorf("IP is reserved")
			}

			// Create new lease
			lease := &storage.Lease{
				IP:          expired.IP,
				MAC:         req.MAC,
				Hostname:    req.Hostname,
				Subnet:      req.Subnet,
				IssuedAt:    time.Now(),
				ExpiresAt:   time.Now().Add(req.LeaseDuration),
				LastSeen:    time.Now(),
				State:       storage.LeaseStateActive,
				ClientID:    req.ClientID,
				VendorClass: req.VendorClass,
				UserClass:   req.UserClass,
				AllocatedBy: a.serverID, // Track which server allocated this lease
			}

			if err := a.store.CreateLease(ctx, lease); err != nil {
				return fmt.Errorf("failed to create lease: %w", err)
			}

			// Optionally update read-only cache after database confirmation
			if a.useCache {
				a.cache.Put(lease)
			}

			return nil
		})

		if err == nil {
			// Successfully allocated
			expiredLeases[0].MAC = req.MAC
			expiredLeases[0].Hostname = req.Hostname
			expiredLeases[0].IssuedAt = time.Now()
			expiredLeases[0].ExpiresAt = time.Now().Add(req.LeaseDuration)
			expiredLeases[0].LastSeen = time.Now()
			expiredLeases[0].State = storage.LeaseStateActive
			expiredLeases[0].ClientID = req.ClientID
			expiredLeases[0].VendorClass = req.VendorClass
			expiredLeases[0].UserClass = req.UserClass

			return expiredLeases[0], nil
		}
	}

	// If no expired leases, try to find a never-used IP
	return a.findNeverUsedIP(ctx, req, pool)
}

// findNeverUsedIP searches for an IP that has never been allocated
// Uses randomized selection to reduce lock contention in active/active deployments
func (a *Allocator) findNeverUsedIP(ctx context.Context, req *AllocationRequest, pool *PoolConfig) (*storage.Lease, error) {
	rangeStart := net.ParseIP(pool.RangeStart).To4()
	rangeEnd := net.ParseIP(pool.RangeEnd).To4()

	// Calculate IP range
	startInt := binary.BigEndian.Uint32(rangeStart)
	endInt := binary.BigEndian.Uint32(rangeEnd)

	// Generate list of all IPs in range
	var ips []net.IP
	for ipInt := startInt; ipInt <= endInt; ipInt++ {
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, ipInt)
		ips = append(ips, ip)
	}

	// Shuffle to randomize order (reduces contention between servers)
	// Multiple servers will try different IPs first, reducing conflicts
	rand.Shuffle(len(ips), func(i, j int) {
		ips[i], ips[j] = ips[j], ips[i]
	})

	// Try random IPs until one succeeds
	for _, ip := range ips {
		lockKey := getAdvisoryLockKey(ip, req.Subnet)
		var lease *storage.Lease
		err := a.store.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
			// Check if IP is already in use
			existing, err := a.store.GetLeaseByIP(ctx, ip, req.Subnet)
			if err != nil {
				return err
			}
			if existing != nil {
				// IP is in use, skip
				return fmt.Errorf("IP in use")
			}

			// Check if IP is reserved
			reservation, err := a.store.GetReservationByIP(ctx, ip, req.Subnet)
			if err != nil {
				return err
			}
			if reservation != nil && reservation.MAC.String() != req.MAC.String() {
				// Reserved for someone else
				return fmt.Errorf("IP is reserved")
			}

			// Create new lease
			lease = &storage.Lease{
				IP:          ip,
				MAC:         req.MAC,
				Hostname:    req.Hostname,
				Subnet:      req.Subnet,
				IssuedAt:    time.Now(),
				ExpiresAt:   time.Now().Add(req.LeaseDuration),
				LastSeen:    time.Now(),
				State:       storage.LeaseStateActive,
				ClientID:    req.ClientID,
				VendorClass: req.VendorClass,
				UserClass:   req.UserClass,
				AllocatedBy: a.serverID, // Track which server allocated this lease
			}

			if err := a.store.CreateLease(ctx, lease); err != nil {
				return fmt.Errorf("failed to create lease: %w", err)
			}

			// Optionally update read-only cache after database confirmation
			if a.useCache {
				a.cache.Put(lease)
			}

			return nil
		})

		if err == nil && lease != nil {
			return lease, nil
		}
	}

	return nil, fmt.Errorf("pool exhausted: no available IPs")
}

// createLeaseForReservation creates a lease for a reserved IP
func (a *Allocator) createLeaseForReservation(ctx context.Context, req *AllocationRequest, reservation *storage.Reservation) (*storage.Lease, error) {
	lockKey := getAdvisoryLockKey(reservation.IP, req.Subnet)
	var lease *storage.Lease

	err := a.store.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
		// Check if there's an existing lease for this IP
		existing, err := a.store.GetLeaseByIP(ctx, reservation.IP, req.Subnet)
		if err != nil {
			return err
		}

		now := time.Now()
		expiresAt := now.Add(req.LeaseDuration)

		if existing != nil {
			// Update existing lease
			existing.MAC = req.MAC
			existing.Hostname = req.Hostname
			existing.IssuedAt = now
			existing.ExpiresAt = expiresAt
			existing.LastSeen = now
			existing.State = storage.LeaseStateActive
			existing.ClientID = req.ClientID
			existing.VendorClass = req.VendorClass
			existing.UserClass = req.UserClass
			existing.AllocatedBy = a.serverID // Track which server allocated this lease

			if err := a.store.UpdateLease(ctx, existing); err != nil {
				return fmt.Errorf("failed to update lease: %w", err)
			}

			lease = existing
		} else {
			// Create new lease
			lease = &storage.Lease{
				IP:          reservation.IP,
				MAC:         req.MAC,
				Hostname:    reservation.Hostname,
				Subnet:      req.Subnet,
				IssuedAt:    now,
				ExpiresAt:   expiresAt,
				LastSeen:    now,
				State:       storage.LeaseStateActive,
				ClientID:    req.ClientID,
				VendorClass: req.VendorClass,
				UserClass:   req.UserClass,
				AllocatedBy: a.serverID, // Track which server allocated this lease
			}

			if err := a.store.CreateLease(ctx, lease); err != nil {
				return fmt.Errorf("failed to create lease: %w", err)
			}
		}

		// Optionally update read-only cache after database confirmation
		if a.useCache {
			a.cache.Put(lease)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return lease, nil
}

// RenewLease renews an existing lease
func (a *Allocator) RenewLease(ctx context.Context, mac net.HardwareAddr, ip net.IP, subnet *net.IPNet, duration time.Duration) error {
	lockKey := getAdvisoryLockKey(ip, subnet)

	return a.store.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
		lease, err := a.store.GetLeaseByIP(ctx, ip, subnet)
		if err != nil {
			return fmt.Errorf("failed to get lease: %w", err)
		}
		if lease == nil {
			return fmt.Errorf("lease not found")
		}
		if lease.MAC.String() != mac.String() {
			return fmt.Errorf("MAC address mismatch")
		}

		expiresAt := time.Now().Add(duration)
		if err := a.store.RenewLease(ctx, lease.ID, expiresAt); err != nil {
			return err
		}

		// Optionally update read-only cache after database confirmation
		if a.useCache {
			lease.ExpiresAt = expiresAt
			lease.LastSeen = time.Now()
			a.cache.Put(lease)
		}

		return nil
	})
}

// ReleaseLease releases a lease
func (a *Allocator) ReleaseLease(ctx context.Context, ip net.IP, subnet *net.IPNet) error {
	if err := a.store.ReleaseLease(ctx, ip, subnet); err != nil {
		return err
	}

	// Remove from cache
	a.cache.RemoveByIP(ip)

	return nil
}

// DeclineLease marks a lease as declined (IP conflict)
func (a *Allocator) DeclineLease(ctx context.Context, ip net.IP, subnet *net.IPNet) error {
	if err := a.store.DeclineLease(ctx, ip, subnet); err != nil {
		return err
	}

	// Remove from cache
	a.cache.RemoveByIP(ip)

	return nil
}

// GetCacheStats returns cache statistics
func (a *Allocator) GetCacheStats() storage.CacheStats {
	return a.cache.Stats()
}

// AllocationRequest contains parameters for IP allocation
type AllocationRequest struct {
	MAC           net.HardwareAddr
	Hostname      string
	Subnet        *net.IPNet
	Pools         []*PoolConfig
	LeaseDuration time.Duration
	ClientID      string
	VendorClass   string
	UserClass     string
}

// PoolConfig represents a DHCP pool configuration
type PoolConfig struct {
	RangeStart string
	RangeEnd   string
}

// getAdvisoryLockKey generates a lock key for an IP and subnet
func getAdvisoryLockKey(ip net.IP, subnet *net.IPNet) int64 {
	h := fnv.New64a()
	h.Write([]byte(subnet.String()))
	h.Write(ip)
	return int64(h.Sum64())
}
