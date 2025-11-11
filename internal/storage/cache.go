package storage

import (
	"container/list"
	"net"
	"sync"
	"time"
)

// LeaseCache is an LRU cache for DHCP leases
type LeaseCache struct {
	mu          sync.RWMutex
	maxSize     int
	byMAC       map[string]*list.Element  // MAC -> lease element
	byIP        map[string]*list.Element  // IP -> lease element
	lruList     *list.List                // LRU eviction list
	hits        uint64
	misses      uint64
	evictions   uint64
}

// cacheEntry represents a cached lease with its LRU metadata
type cacheEntry struct {
	key   string // Either MAC or IP
	lease *Lease
}

// NewLeaseCache creates a new LRU cache with the specified maximum size
func NewLeaseCache(maxSize int) *LeaseCache {
	if maxSize <= 0 {
		maxSize = 10000 // Default to 10k leases
	}

	return &LeaseCache{
		maxSize: maxSize,
		byMAC:   make(map[string]*list.Element),
		byIP:    make(map[string]*list.Element),
		lruList: list.New(),
	}
}

// GetByMAC retrieves a lease from the cache by MAC address
func (c *LeaseCache) GetByMAC(mac net.HardwareAddr) (*Lease, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, found := c.byMAC[mac.String()]
	if !found {
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)
	c.hits++

	entry := elem.Value.(*cacheEntry)
	return entry.lease, true
}

// GetByIP retrieves a lease from the cache by IP address
func (c *LeaseCache) GetByIP(ip net.IP) (*Lease, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, found := c.byIP[ip.String()]
	if !found {
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)
	c.hits++

	entry := elem.Value.(*cacheEntry)
	return entry.lease, true
}

// Put adds or updates a lease in the cache
func (c *LeaseCache) Put(lease *Lease) {
	c.mu.Lock()
	defer c.mu.Unlock()

	macKey := lease.MAC.String()
	ipKey := lease.IP.String()

	// Check if lease already exists in cache
	if elem, found := c.byMAC[macKey]; found {
		// Update existing entry
		entry := elem.Value.(*cacheEntry)
		entry.lease = lease
		c.lruList.MoveToFront(elem)

		// Update IP index if IP changed
		if entry.key != ipKey {
			delete(c.byIP, entry.key)
			c.byIP[ipKey] = elem
			entry.key = ipKey
		}
		return
	}

	// Add new entry
	entry := &cacheEntry{
		key:   macKey,
		lease: lease,
	}
	elem := c.lruList.PushFront(entry)
	c.byMAC[macKey] = elem
	c.byIP[ipKey] = elem

	// Evict oldest if over capacity
	if c.lruList.Len() > c.maxSize {
		c.evictOldest()
	}
}

// Remove removes a lease from the cache
func (c *LeaseCache) Remove(mac net.HardwareAddr, ip net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()

	macKey := mac.String()
	ipKey := ip.String()

	if elem, found := c.byMAC[macKey]; found {
		c.removeElement(elem)
	} else if elem, found := c.byIP[ipKey]; found {
		c.removeElement(elem)
	}
}

// RemoveByIP removes a lease from the cache by IP address
func (c *LeaseCache) RemoveByIP(ip net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.byIP[ip.String()]; found {
		c.removeElement(elem)
	}
}

// RemoveByMAC removes a lease from the cache by MAC address
func (c *LeaseCache) RemoveByMAC(mac net.HardwareAddr) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.byMAC[mac.String()]; found {
		c.removeElement(elem)
	}
}

// Clear removes all entries from the cache
func (c *LeaseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byMAC = make(map[string]*list.Element)
	c.byIP = make(map[string]*list.Element)
	c.lruList = list.New()
}

// Size returns the current number of entries in the cache
func (c *LeaseCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Stats returns cache statistics
func (c *LeaseCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:      c.lruList.Len(),
		MaxSize:   c.maxSize,
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		HitRate:   hitRate,
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	Size      int
	MaxSize   int
	Hits      uint64
	Misses    uint64
	Evictions uint64
	HitRate   float64
}

// evictOldest removes the least recently used entry from the cache
// Caller must hold write lock
func (c *LeaseCache) evictOldest() {
	elem := c.lruList.Back()
	if elem != nil {
		c.removeElement(elem)
		c.evictions++
	}
}

// removeElement removes an element from the cache
// Caller must hold write lock
func (c *LeaseCache) removeElement(elem *list.Element) {
	c.lruList.Remove(elem)
	entry := elem.Value.(*cacheEntry)

	delete(c.byMAC, entry.lease.MAC.String())
	delete(c.byIP, entry.lease.IP.String())
}

// ExpireOld removes expired leases from the cache
func (c *LeaseCache) ExpireOld() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expired := 0

	// Walk the list and remove expired leases
	for elem := c.lruList.Back(); elem != nil; {
		entry := elem.Value.(*cacheEntry)
		prev := elem.Prev()

		if entry.lease.State == LeaseStateActive && entry.lease.ExpiresAt.Before(now) {
			c.removeElement(elem)
			expired++
		}

		elem = prev
	}

	return expired
}
