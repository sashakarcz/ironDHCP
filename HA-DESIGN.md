# ironDHCP Active/Active High Availability Design

Implementation plan for true active/active HA deployment with multiple concurrent DHCP servers.

## Current State Analysis

### What Works Now
- ✅ **PostgreSQL Advisory Locks**: Prevents double-allocation at database level
- ✅ **Shared Database**: All servers can use the same PostgreSQL instance
- ✅ **UNIQUE Constraints**: Database enforces IP uniqueness per subnet
- ✅ **Idempotent Operations**: Expiry workers and cleanup are safe to run on multiple servers

### What Prevents Active/Active
- ❌ **In-Memory Cache Inconsistency**: Each server has isolated 10,000-entry LRU cache
- ❌ **Sequential IP Scanning**: Both servers compete for same IPs in order
- ❌ **Cache Update Timing**: Race condition between cache update and database check
- ❌ **No Server Coordination**: No load distribution or pool partitioning
- ❌ **No Server Identity**: Can't track which server allocated a lease

## Architecture: Three Approaches

### Approach 1: Minimal Changes (Cache Removal)
**Best for**: Quick deployment, low complexity

**Changes Required:**
1. Remove in-memory cache entirely
2. Rely on PostgreSQL advisory locks + database queries
3. Add database connection pooling optimization
4. Implement randomized IP selection instead of sequential scan

**Pros:**
- Simple implementation (~2 days)
- No new dependencies
- Guaranteed consistency
- Works immediately with existing infrastructure

**Cons:**
- Higher database load (every lookup hits DB)
- Slightly higher latency (no cache hits)
- More PostgreSQL connections required

**Performance Impact:**
- Estimated: 100-200 leases/second per server (down from 1000+)
- Acceptable for most deployments (10,000 clients = ~50 requests/sec average)

---

### Approach 2: Distributed Cache (Redis)
**Best for**: High performance, scalability

**Changes Required:**
1. Add Redis as shared cache layer
2. Implement cache invalidation on lease changes
3. Add cache-aside pattern with TTL-based expiry
4. Implement randomized IP selection
5. Add Redis health monitoring

**Pros:**
- High performance (cache hit latency ~1ms)
- True active/active with shared state
- Scales to many servers
- Cache invalidation propagates between servers

**Cons:**
- Additional infrastructure (Redis cluster)
- More complex deployment
- New failure mode (Redis unavailable)
- Requires Redis Sentinel or Cluster for Redis HA

**Performance Impact:**
- Estimated: 800-1000 leases/second per server
- Near-identical to current single-server performance

---

### Approach 3: Pool Partitioning (Hash-Based)
**Best for**: Maximum simplicity, deterministic behavior

**Changes Required:**
1. Add server ID to configuration
2. Implement hash-based pool partitioning
3. Each server handles subset of IP range based on hash(MAC) % server_count
4. Fallback to other pools if primary server is down
5. Add server health checks and failover logic

**Pros:**
- No cache synchronization needed
- Deterministic allocation (same MAC always goes to same server)
- Minimal database contention
- Easy to reason about

**Cons:**
- Uneven distribution if MACs aren't uniformly distributed
- Server addition/removal requires rebalancing
- Complex failover logic
- Wasted capacity if one server is overloaded

**Performance Impact:**
- Estimated: 1000+ leases/second per server
- Each server handles 1/N of total load

---

## Recommended Implementation: Hybrid Approach

Combine **Approach 1** (cache removal) with **Approach 3** (pool partitioning) for best balance.

### Phase 1: Foundation (Week 1)

#### 1.1 Add Server Identity
```yaml
# config.yaml
server:
  id: "dhcp-01"  # Unique server identifier
  cluster:
    enabled: true
    total_servers: 2  # Total number of servers in cluster
```

**Files to modify:**
- `internal/config/config.go` - Add server config struct
- `internal/dhcp/server.go` - Store server ID
- `migrations/002_add_server_id.sql` - Add `allocated_by` column to leases table

**Database migration:**
```sql
ALTER TABLE leases ADD COLUMN allocated_by TEXT;
CREATE INDEX idx_leases_allocated_by ON leases(allocated_by);
```

---

#### 1.2 Remove/Simplify In-Memory Cache

**Option A: Complete Removal (Simplest)**
```go
// internal/dhcp/allocator.go
func (a *Allocator) AllocateIP(ctx context.Context, clientMAC net.HardwareAddr,
    subnet *net.IPNet, requestedIP net.IP) (net.IP, error) {

    // Remove cache lookup - go straight to database
    // Check existing active lease
    existingLease, err := a.store.GetActiveLeaseByMAC(ctx, clientMAC, subnet)
    if err != nil {
        return nil, err
    }
    if existingLease != nil {
        return existingLease.IP, nil
    }

    // Rest of allocation logic...
}
```

**Option B: Read-Only Cache (Better Performance)**
```go
// Cache is used ONLY for reads, never for allocation decisions
// Always verify with database before allocation
func (a *Allocator) AllocateIP(ctx context.Context, clientMAC net.HardwareAddr,
    subnet *net.IPNet, requestedIP net.IP) (net.IP, error) {

    // Check cache for hints, but ALWAYS verify with DB
    cachedLease := a.cache.GetByMAC(clientMAC)
    if cachedLease != nil {
        // Verify cache entry is still valid in database
        dbLease, err := a.store.GetLeaseByID(ctx, cachedLease.ID)
        if err == nil && dbLease != nil && dbLease.State == storage.LeaseStateActive {
            return dbLease.IP, nil
        }
        // Cache was stale, remove it
        a.cache.Remove(cachedLease.IP)
    }

    // Always check database as source of truth
    existingLease, err := a.store.GetActiveLeaseByMAC(ctx, clientMAC, subnet)
    // ...
}
```

**Files to modify:**
- `internal/dhcp/allocator.go` - Modify AllocateIP logic
- `internal/storage/cache.go` - Add read-only mode or remove entirely

---

#### 1.3 Implement Randomized IP Selection

Replace sequential scanning with randomized selection to reduce contention:

```go
// internal/dhcp/allocator.go
func (a *Allocator) findNeverUsedIP(ctx context.Context, pool *PoolConfig,
    subnet *net.IPNet) (net.IP, error) {

    // Convert IP range to list
    ips := generateIPList(pool.RangeStart, pool.RangeEnd)

    // Randomize order to reduce contention between servers
    rand.Shuffle(len(ips), func(i, j int) {
        ips[i], ips[j] = ips[j], ips[i]
    })

    // Try random IPs until we find one that's available
    for _, ip := range ips {
        // Acquire advisory lock
        lockKey := generateLockKey(subnet, ip)

        err := a.store.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
            // Check if IP is in use
            exists, err := a.store.LeaseExists(ctx, ip, subnet)
            if err != nil {
                return err
            }
            if exists {
                return ErrIPInUse  // Try next IP
            }

            // Create lease
            return a.store.CreateLease(ctx, lease)
        })

        if err == nil {
            return ip, nil  // Success
        }
        if err != ErrIPInUse {
            return nil, err  // Real error
        }
        // IP was in use, try next random IP
    }

    return nil, ErrNoIPsAvailable
}
```

**Files to modify:**
- `internal/dhcp/allocator.go` - Replace sequential scan with randomized selection

---

### Phase 2: Pool Partitioning (Week 2)

#### 2.1 Hash-Based Pool Assignment

Implement consistent hashing to distribute load:

```go
// internal/dhcp/allocator.go
func (a *Allocator) getPreferredPool(clientMAC net.HardwareAddr, pools []*PoolConfig) *PoolConfig {
    if !a.config.Cluster.Enabled {
        return pools[0]  // No clustering, use first pool
    }

    // Hash MAC address to determine which server should handle this client
    h := fnv.New64a()
    h.Write([]byte(clientMAC.String()))
    hash := h.Sum64()

    // Determine which server this MAC maps to
    preferredServerIndex := hash % uint64(a.config.Cluster.TotalServers)

    // This server's index (0-based)
    myServerIndex := a.getMyServerIndex()

    if preferredServerIndex == myServerIndex {
        // This MAC is assigned to us, use primary strategy
        return pools[0]
    } else {
        // This MAC is assigned to another server
        // We can still handle it, but with lower priority (failover mode)
        // Try our pools last to avoid conflicts
        return nil  // Signal to use fallback allocation
    }
}

func (a *Allocator) AllocateIP(ctx context.Context, clientMAC net.HardwareAddr,
    subnet *net.IPNet, requestedIP net.IP) (net.IP, error) {

    // Check existing lease first
    existingLease, err := a.store.GetActiveLeaseByMAC(ctx, clientMAC, subnet)
    if err != nil {
        return nil, err
    }
    if existingLease != nil {
        return existingLease.IP, nil
    }

    // Check static reservation
    reservation, err := a.store.GetReservationByMAC(ctx, clientMAC)
    if err != nil {
        return nil, err
    }
    if reservation != nil && reservation.Subnet.String() == subnet.String() {
        return a.allocateReservedIP(ctx, clientMAC, reservation.IP, subnet)
    }

    // Determine preferred pool based on MAC hash
    pools := a.getPoolsForSubnet(subnet)
    preferredPool := a.getPreferredPool(clientMAC, pools)

    if preferredPool != nil {
        // Try preferred pool first (this server handles this MAC range)
        ip, err := a.allocateFromPool(ctx, preferredPool, subnet)
        if err == nil {
            return ip, nil
        }
    }

    // Fallback: try all pools (failover mode)
    for _, pool := range pools {
        ip, err := a.allocateFromPool(ctx, pool, subnet)
        if err == nil {
            return ip, nil
        }
    }

    return nil, ErrNoIPsAvailable
}
```

---

#### 2.2 Server Configuration

```yaml
# config.yaml for server 1
server:
  id: "dhcp-01"
  cluster:
    enabled: true
    total_servers: 2
    server_index: 0  # This server is index 0

# config.yaml for server 2
server:
  id: "dhcp-02"
  cluster:
    enabled: true
    total_servers: 2
    server_index: 1  # This server is index 1
```

**Files to modify:**
- `internal/config/config.go` - Add cluster configuration
- `internal/dhcp/allocator.go` - Implement hash-based assignment

---

### Phase 3: Monitoring & Observability (Week 3)

#### 3.1 Add Metrics

```go
// internal/metrics/metrics.go
var (
    dhcpAllocationsPerServer = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "irondhcp_allocations_per_server_total",
            Help: "Total DHCP allocations per server",
        },
        []string{"server_id", "subnet"},
    )

    dhcpAllocationConflicts = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "irondhcp_allocation_conflicts_total",
            Help: "Number of IP allocation conflicts between servers",
        },
        []string{"server_id", "subnet"},
    )

    dhcpCacheHitRate = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "irondhcp_cache_hit_rate",
            Help: "Cache hit rate percentage",
        },
        []string{"server_id"},
    )
)
```

---

#### 3.2 Health Checks

Add server-to-server health checks:

```go
// internal/cluster/health.go
type ClusterHealth struct {
    store       *storage.Store
    serverID    string
    totalServers int
}

func (c *ClusterHealth) CheckPeerHealth(ctx context.Context) ([]ServerStatus, error) {
    // Query recent allocations by server to detect if peers are alive
    query := `
        SELECT allocated_by, COUNT(*), MAX(created_at)
        FROM leases
        WHERE created_at > NOW() - INTERVAL '5 minutes'
        GROUP BY allocated_by
    `

    // If a server hasn't allocated anything in 5 minutes but should be handling
    // requests (based on hash distribution), it may be down

    // Return status of all servers
    return statuses, nil
}

func (c *ClusterHealth) UpdateHeartbeat(ctx context.Context) error {
    // Insert or update server heartbeat in database
    query := `
        INSERT INTO server_heartbeats (server_id, last_seen)
        VALUES ($1, NOW())
        ON CONFLICT (server_id) DO UPDATE SET last_seen = NOW()
    `
    _, err := c.store.Exec(ctx, query, c.serverID)
    return err
}
```

**New migration:**
```sql
-- migrations/003_add_server_heartbeats.sql
CREATE TABLE server_heartbeats (
    server_id TEXT PRIMARY KEY,
    last_seen TIMESTAMPTZ NOT NULL,
    version TEXT,
    metadata JSONB
);

CREATE INDEX idx_server_heartbeats_last_seen ON server_heartbeats(last_seen);
```

---

### Phase 4: Advanced Features (Week 4+)

#### 4.1 Automatic Failover

Implement automatic pool rebalancing when a server goes down:

```go
// internal/cluster/failover.go
func (c *ClusterHealth) DetectFailedServers(ctx context.Context) ([]string, error) {
    query := `
        SELECT server_id
        FROM server_heartbeats
        WHERE last_seen < NOW() - INTERVAL '1 minute'
    `

    // These servers are considered down
    // Active servers should take over their share of the pool
}

func (a *Allocator) adjustPoolAssignment(ctx context.Context) error {
    // If server count has changed (due to failures), adjust hash distribution
    // This is handled automatically by getPreferredPool() - just needs updated total_servers
}
```

---

#### 4.2 Lease Migration

When servers are added/removed, migrate leases for better distribution:

```go
// internal/cluster/migration.go
func MigrateLeases(ctx context.Context, store *storage.Store,
    oldServerCount, newServerCount int) error {

    // Get all active leases
    leases, err := store.GetAllLeases(ctx)
    if err != nil {
        return err
    }

    // For each lease, calculate if it needs to be "transferred"
    // (This is logical only - lease stays in DB, but allocated_by may change)
    for _, lease := range leases {
        oldServer := hashMAC(lease.MAC) % oldServerCount
        newServer := hashMAC(lease.MAC) % newServerCount

        if oldServer != newServer {
            // Update allocated_by to reflect new assignment
            // This is informational only - doesn't affect functionality
            err := store.UpdateLeaseServer(ctx, lease.ID,
                fmt.Sprintf("dhcp-%02d", newServer))
            if err != nil {
                return err
            }
        }
    }

    return nil
}
```

---

## Deployment Strategy

### Initial Deployment (Active/Passive)

```yaml
# Start with active/passive to test
server:
  id: "dhcp-01"
  cluster:
    enabled: false  # Single server mode

# Second server remains off
```

### Enable Active/Active

```yaml
# Server 1
server:
  id: "dhcp-01"
  cluster:
    enabled: true
    total_servers: 2
    server_index: 0

# Server 2
server:
  id: "dhcp-02"
  cluster:
    enabled: true
    total_servers: 2
    server_index: 1
```

### Load Balancer Configuration

Use DHCP relay agents or configure switches to balance between servers:

```
# Option 1: DHCP Relay with multiple helpers
interface Vlan10
  ip helper-address 192.168.1.10  # Server 1
  ip helper-address 192.168.1.11  # Server 2

# Clients will receive OFFER from both servers, accept first one
```

```
# Option 2: Anycast (Advanced)
# Both servers share same IP using BGP anycast
# Routing directs clients to nearest server
```

---

## Testing Strategy

### Unit Tests
```go
func TestAllocatorConcurrency(t *testing.T) {
    // Simulate two servers allocating from same pool
    server1 := NewAllocator(config1, store)
    server2 := NewAllocator(config2, store)

    // Allocate 100 IPs concurrently
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(2)

        go func(i int) {
            defer wg.Done()
            mac := generateMAC(i)
            ip, err := server1.AllocateIP(ctx, mac, subnet, nil)
            assert.NoError(t, err)
            assert.NotNil(t, ip)
        }(i)

        go func(i int) {
            defer wg.Done()
            mac := generateMAC(i + 50)
            ip, err := server2.AllocateIP(ctx, mac, subnet, nil)
            assert.NoError(t, err)
            assert.NotNil(t, ip)
        }(i)
    }

    wg.Wait()

    // Verify no duplicate IPs were allocated
    leases, _ := store.GetAllLeases(ctx)
    ips := make(map[string]bool)
    for _, lease := range leases {
        assert.False(t, ips[lease.IP.String()], "Duplicate IP allocated")
        ips[lease.IP.String()] = true
    }
}
```

### Integration Tests
```bash
#!/bin/bash
# test-ha.sh

# Start two servers
./bin/irondhcp --config config-server1.yaml &
PID1=$!

./bin/irondhcp --config config-server2.yaml &
PID2=$!

sleep 5

# Simulate 1000 DHCP requests across both servers
for i in {1..1000}; do
    # Generate random MAC
    MAC=$(printf '02:00:00:%02x:%02x:%02x' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256)))

    # Send DHCP DISCOVER to random server
    if [ $((RANDOM % 2)) -eq 0 ]; then
        send_dhcp_discover $MAC server1
    else
        send_dhcp_discover $MAC server2
    fi
done

# Check for duplicate IP allocations
DUPLICATES=$(psql -h localhost -U dhcp -d godhcp -t -c \
    "SELECT ip, COUNT(*) FROM leases WHERE state='active'
     GROUP BY ip HAVING COUNT(*) > 1")

if [ -z "$DUPLICATES" ]; then
    echo "✓ No duplicate IPs found"
else
    echo "✗ Duplicate IPs detected:"
    echo "$DUPLICATES"
    exit 1
fi

kill $PID1 $PID2
```

---

## Performance Benchmarks

### Expected Performance (Approach 1: Cache Removal)

| Metric | Single Server | Active/Active (2 servers) |
|--------|--------------|---------------------------|
| Allocations/sec | 100-200 | 200-400 |
| Cache hit rate | 0% (no cache) | 0% (no cache) |
| DB queries/allocation | 2-3 | 2-3 |
| Lock contention | Low | Medium |
| Latency (p50) | 5-10ms | 5-15ms |
| Latency (p99) | 20-50ms | 50-100ms |

### Expected Performance (Approach 2: Redis Cache)

| Metric | Single Server | Active/Active (2 servers) |
|--------|--------------|---------------------------|
| Allocations/sec | 800-1000 | 1600-2000 |
| Cache hit rate | 90%+ | 85%+ |
| DB queries/allocation | 0.1-0.2 | 0.2-0.3 |
| Lock contention | Low | Low |
| Latency (p50) | 1-2ms | 1-3ms |
| Latency (p99) | 10-20ms | 20-40ms |

---

## Migration Path

### Step 1: Add Server ID (No Downtime)
1. Deploy new version with server ID configuration
2. Run migration to add `allocated_by` column
3. All new leases will track which server allocated them
4. Existing leases will have NULL `allocated_by` (no impact)

### Step 2: Deploy Second Server (Active/Passive)
1. Configure second server with cluster disabled
2. Use as hot standby
3. Test failover by stopping primary
4. Verify secondary takes over correctly

### Step 3: Enable Active/Active
1. Update both configs to enable clustering
2. Restart both servers (brief interruption)
3. Monitor for conflicts and contention
4. Adjust pool distribution if needed

### Step 4: Add Monitoring
1. Deploy Prometheus/Grafana
2. Create alerts for:
   - High lock contention
   - IP pool exhaustion
   - Server failures
   - Allocation conflicts

---

## Cost/Benefit Analysis

### Option 1: Cache Removal + Random Selection
**Effort**: 3-5 days
**Risk**: Low
**Performance**: Acceptable (100-200 leases/sec per server)
**Complexity**: Low
**Recommended for**: Most deployments (< 10,000 clients)

### Option 2: Redis + Pool Partitioning
**Effort**: 2-3 weeks
**Risk**: Medium
**Performance**: Excellent (800-1000 leases/sec per server)
**Complexity**: Medium
**Recommended for**: Large deployments (> 50,000 clients)

### Option 3: Pool Partitioning Only
**Effort**: 1-2 weeks
**Risk**: Medium
**Performance**: Good (400-600 leases/sec per server)
**Complexity**: Medium
**Recommended for**: Predictable, stable environments

---

## Questions to Consider

1. **How many clients do you need to support?**
   - < 10,000: Option 1 is sufficient
   - 10,000-50,000: Option 3 recommended
   - > 50,000: Option 2 recommended

2. **Can you tolerate Redis dependency?**
   - Yes: Option 2 provides best performance
   - No: Option 1 or 3

3. **How critical is zero-downtime deployment?**
   - Critical: Option 2 (Redis) allows rolling updates
   - Acceptable: Option 1 or 3 (brief interruption during config changes)

4. **Do you have operational expertise for Redis?**
   - Yes: Option 2
   - No: Option 1 or 3

---

## Conclusion

**Recommended Approach**: Start with **Option 1** (Cache Removal + Random Selection)

This provides:
- ✅ True active/active HA immediately
- ✅ Minimal code changes
- ✅ No new dependencies
- ✅ Good performance for most use cases
- ✅ Foundation for future enhancements

**Future Migration Path**: Add Redis caching (Option 2) if performance becomes a bottleneck.

The current advisory lock mechanism is solid and will work correctly for active/active. The main work is removing the cache dependency and implementing smarter IP selection to reduce contention.

Would you like me to start implementing any of these approaches?
