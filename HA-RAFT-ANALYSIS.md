# Raft Consensus for ironDHCP High Availability

Analysis of using Raft-based leader election (Consul-style) for HA DHCP.

## Raft Overview

Raft is a consensus algorithm that provides:
- **Leader Election**: Automatic election of a single leader node
- **Strong Consistency**: All nodes agree on the same state
- **Fault Tolerance**: Continues operating with majority (quorum) available
- **Log Replication**: Leader replicates state changes to followers

**Popular Implementations:**
- HashiCorp Consul (service mesh with Raft)
- etcd (Kubernetes backing store)
- HashiCorp Nomad (orchestrator)

## Architecture: Active/Passive with Raft

### How It Works

```
┌──────────────────────────────────────────────────────────┐
│                    Raft Cluster                          │
│                                                           │
│  ┌───────────┐      ┌───────────┐      ┌───────────┐   │
│  │  DHCP-01  │      │  DHCP-02  │      │  DHCP-03  │   │
│  │  LEADER   │◄────►│ FOLLOWER  │◄────►│ FOLLOWER  │   │
│  │  Active   │      │  Standby  │      │  Standby  │   │
│  └─────┬─────┘      └───────────┘      └───────────┘   │
│        │                                                 │
│        │ Only leader serves DHCP                        │
│        ▼                                                 │
│  ┌─────────────┐                                        │
│  │ PostgreSQL  │◄───────────────────────────────────────┤
│  │  (Shared)   │  All nodes read/write                  │
│  └─────────────┘                                        │
└──────────────────────────────────────────────────────────┘

Client DHCP Requests → VIP (floating) → Current Leader
```

### Key Characteristics

1. **Active/Passive**: Only ONE server handles DHCP at a time (the leader)
2. **Automatic Failover**: When leader fails, followers elect new leader (~1-3 seconds)
3. **Floating IP**: Virtual IP moves to current leader
4. **No Split-Brain**: Raft guarantees single leader per term
5. **Shared Database**: All nodes use PostgreSQL for persistent state

---

## Option 4: Raft-Based Leader Election (Consul/etcd Style)

### Implementation Approaches

#### Approach A: Use Consul Directly
**Use HashiCorp Consul for service discovery and leader election**

**Configuration:**
```yaml
# config.yaml
server:
  id: "dhcp-01"

consul:
  enabled: true
  address: "localhost:8500"
  service_name: "dhcp-server"
  leader_election:
    enabled: true
    session_ttl: "10s"
    lock_key: "service/dhcp/leader"
```

**Implementation:**
```go
// internal/cluster/consul.go
package cluster

import (
    "context"
    "time"
    consulapi "github.com/hashicorp/consul/api"
)

type ConsulLeaderElection struct {
    client      *consulapi.Client
    sessionID   string
    lockKey     string
    isLeader    bool
    onElected   func()
    onDeposed   func()
}

func NewConsulLeaderElection(config *ConsulConfig) (*ConsulLeaderElection, error) {
    consulConfig := consulapi.DefaultConfig()
    consulConfig.Address = config.Address

    client, err := consulapi.NewClient(consulConfig)
    if err != nil {
        return nil, err
    }

    return &ConsulLeaderElection{
        client:  client,
        lockKey: config.LockKey,
    }, nil
}

func (c *ConsulLeaderElection) Start(ctx context.Context) error {
    // Create session
    session := c.client.Session()
    sessionID, _, err := session.Create(&consulapi.SessionEntry{
        Name:      "dhcp-leader-election",
        TTL:       "10s",
        Behavior:  "delete",
        LockDelay: time.Second,
    }, nil)
    if err != nil {
        return err
    }
    c.sessionID = sessionID

    // Start leader election loop
    go c.leaderElectionLoop(ctx)

    // Start session renewal
    go c.renewSession(ctx)

    return nil
}

func (c *ConsulLeaderElection) leaderElectionLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.tryAcquireLock(ctx)
        }
    }
}

func (c *ConsulLeaderElection) tryAcquireLock(ctx context.Context) {
    kv := c.client.KV()

    // Try to acquire lock
    acquired, _, err := kv.Acquire(&consulapi.KVPair{
        Key:     c.lockKey,
        Value:   []byte(c.sessionID),
        Session: c.sessionID,
    }, nil)

    if err != nil {
        logger.Error().Err(err).Msg("Failed to acquire leader lock")
        return
    }

    if acquired && !c.isLeader {
        // We just became leader
        c.isLeader = true
        logger.Info().Msg("Elected as DHCP leader")
        if c.onElected != nil {
            c.onElected()
        }
    } else if !acquired && c.isLeader {
        // We lost leadership
        c.isLeader = false
        logger.Warn().Msg("Lost DHCP leadership")
        if c.onDeposed != nil {
            c.onDeposed()
        }
    }
}

func (c *ConsulLeaderElection) renewSession(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            _, _, err := c.client.Session().Renew(c.sessionID, nil)
            if err != nil {
                logger.Error().Err(err).Msg("Failed to renew session")
            }
        }
    }
}

func (c *ConsulLeaderElection) IsLeader() bool {
    return c.isLeader
}
```

**DHCP Server Integration:**
```go
// internal/dhcp/server.go
func (s *Server) Start(ctx context.Context) error {
    if s.config.Consul.Enabled {
        // Initialize Consul leader election
        le, err := cluster.NewConsulLeaderElection(s.config.Consul)
        if err != nil {
            return err
        }

        le.SetCallbacks(
            func() { s.startDHCPListening() },  // On elected
            func() { s.stopDHCPListening() },   // On deposed
        )

        if err := le.Start(ctx); err != nil {
            return err
        }

        s.leaderElection = le
    } else {
        // No leader election, always serve DHCP
        return s.startDHCPListening()
    }

    return nil
}

func (s *Server) startDHCPListening() error {
    if s.isListening {
        return nil
    }

    logger.Info().Msg("Starting DHCP listener as leader")

    // Bind to port 67 and start serving
    conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 67})
    if err != nil {
        return err
    }

    s.conn = conn
    s.isListening = true

    go s.serve()

    return nil
}

func (s *Server) stopDHCPListening() error {
    if !s.isListening {
        return nil
    }

    logger.Warn().Msg("Stopping DHCP listener (no longer leader)")

    if s.conn != nil {
        s.conn.Close()
        s.conn = nil
    }

    s.isListening = false

    return nil
}
```

**Pros:**
- ✅ Proven technology (Consul is battle-tested)
- ✅ Built-in health checking
- ✅ Service discovery for free
- ✅ Web UI for monitoring
- ✅ Automatic failover (1-3 seconds)
- ✅ No split-brain possible
- ✅ Supports multi-datacenter

**Cons:**
- ❌ Additional infrastructure (Consul cluster)
- ❌ New dependency to manage
- ❌ Active/Passive only (not active/active)
- ❌ Consul itself needs to be HA (3-5 nodes)
- ❌ More complex operations

---

#### Approach B: Use etcd
**Similar to Consul but with etcd**

```yaml
# config.yaml
etcd:
  enabled: true
  endpoints:
    - "http://etcd-01:2379"
    - "http://etcd-02:2379"
    - "http://etcd-03:2379"
  leader_election:
    lease_ttl: 10
    election_key: "/dhcp/leader"
```

```go
// internal/cluster/etcd.go
package cluster

import (
    "context"
    "time"
    clientv3 "go.etcd.io/etcd/client/v3"
    "go.etcd.io/etcd/client/v3/concurrency"
)

type EtcdLeaderElection struct {
    client   *clientv3.Client
    session  *concurrency.Session
    election *concurrency.Election
    isLeader bool
}

func NewEtcdLeaderElection(endpoints []string, prefix string) (*EtcdLeaderElection, error) {
    client, err := clientv3.New(clientv3.Config{
        Endpoints:   endpoints,
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        return nil, err
    }

    session, err := concurrency.NewSession(client, concurrency.WithTTL(10))
    if err != nil {
        return nil, err
    }

    election := concurrency.NewElection(session, prefix)

    return &EtcdLeaderElection{
        client:   client,
        session:  session,
        election: election,
    }, nil
}

func (e *EtcdLeaderElection) Campaign(ctx context.Context, identity string) error {
    // Campaign to become leader (blocks until elected)
    if err := e.election.Campaign(ctx, identity); err != nil {
        return err
    }

    e.isLeader = true
    logger.Info().Str("identity", identity).Msg("Elected as leader")

    return nil
}

func (e *EtcdLeaderElection) Observe(ctx context.Context) <-chan string {
    // Watch for leader changes
    return e.election.Observe(ctx)
}

func (e *EtcdLeaderElection) IsLeader() bool {
    return e.isLeader
}

func (e *EtcdLeaderElection) Resign(ctx context.Context) error {
    if !e.isLeader {
        return nil
    }

    logger.Info().Msg("Resigning from leadership")
    e.isLeader = false

    return e.election.Resign(ctx)
}
```

**Pros:**
- ✅ Kubernetes-native (used by K8s itself)
- ✅ Simple API
- ✅ Strong consistency guarantees
- ✅ Good performance
- ✅ Built-in watch API

**Cons:**
- ❌ Additional infrastructure (etcd cluster)
- ❌ Active/Passive only
- ❌ etcd requires 3-5 nodes for HA
- ❌ Less feature-rich than Consul (no web UI, no health checks)

---

#### Approach C: Embedded Raft (hashicorp/raft Library)
**Build Raft directly into ironDHCP**

```go
// internal/cluster/raft.go
package cluster

import (
    "fmt"
    "net"
    "os"
    "path/filepath"
    "time"

    "github.com/hashicorp/raft"
    raftboltdb "github.com/hashicorp/raft-boltdb"
)

type RaftCluster struct {
    raft        *raft.Raft
    fsm         *DHCPStateMachine
    isLeader    bool
    onElected   func()
    onDeposed   func()
}

func NewRaftCluster(config *RaftConfig) (*RaftCluster, error) {
    // Setup Raft configuration
    raftConfig := raft.DefaultConfig()
    raftConfig.LocalID = raft.ServerID(config.NodeID)

    // Setup Raft log store (BoltDB)
    logStore, err := raftboltdb.NewBoltStore(
        filepath.Join(config.DataDir, "raft-log.db"))
    if err != nil {
        return nil, err
    }

    // Setup stable store (BoltDB)
    stableStore, err := raftboltdb.NewBoltStore(
        filepath.Join(config.DataDir, "raft-stable.db"))
    if err != nil {
        return nil, err
    }

    // Setup snapshot store
    snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 3, os.Stderr)
    if err != nil {
        return nil, err
    }

    // Setup transport
    addr, err := net.ResolveTCPAddr("tcp", config.BindAddr)
    if err != nil {
        return nil, err
    }
    transport, err := raft.NewTCPTransport(config.BindAddr, addr, 3, 10*time.Second, os.Stderr)
    if err != nil {
        return nil, err
    }

    // Create FSM (Finite State Machine)
    fsm := NewDHCPStateMachine()

    // Create Raft node
    raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
    if err != nil {
        return nil, err
    }

    cluster := &RaftCluster{
        raft: raftNode,
        fsm:  fsm,
    }

    // Bootstrap cluster if this is the first node
    if config.Bootstrap {
        configuration := raft.Configuration{
            Servers: []raft.Server{
                {
                    ID:      raft.ServerID(config.NodeID),
                    Address: raft.ServerAddress(config.BindAddr),
                },
            },
        }
        raftNode.BootstrapCluster(configuration)
    }

    // Watch for leadership changes
    go cluster.watchLeadership()

    return cluster, nil
}

func (r *RaftCluster) watchLeadership() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for range ticker.C {
        isLeader := r.raft.State() == raft.Leader

        if isLeader && !r.isLeader {
            // Became leader
            r.isLeader = true
            logger.Info().Msg("This node is now the Raft leader")
            if r.onElected != nil {
                r.onElected()
            }
        } else if !isLeader && r.isLeader {
            // Lost leadership
            r.isLeader = false
            logger.Warn().Msg("This node is no longer the Raft leader")
            if r.onDeposed != nil {
                r.onDeposed()
            }
        }
    }
}

func (r *RaftCluster) IsLeader() bool {
    return r.raft.State() == raft.Leader
}

func (r *RaftCluster) GetLeader() string {
    addr, _ := r.raft.LeaderWithID()
    return string(addr)
}

func (r *RaftCluster) AddServer(id, addr string) error {
    future := r.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(addr), 0, 0)
    return future.Error()
}

func (r *RaftCluster) RemoveServer(id string) error {
    future := r.raft.RemoveServer(raft.ServerID(id), 0, 0)
    return future.Error()
}

// DHCPStateMachine implements raft.FSM
type DHCPStateMachine struct {
    // State that needs to be replicated across cluster
    // (For DHCP, we use PostgreSQL for state, so this is minimal)
}

func NewDHCPStateMachine() *DHCPStateMachine {
    return &DHCPStateMachine{}
}

func (f *DHCPStateMachine) Apply(log *raft.Log) interface{} {
    // For DHCP, we don't need to replicate state via Raft
    // We only use Raft for leader election
    // State is in PostgreSQL which all nodes access
    return nil
}

func (f *DHCPStateMachine) Snapshot() (raft.FSMSnapshot, error) {
    // No snapshot needed for leader election only
    return &DHCPSnapshot{}, nil
}

func (f *DHCPStateMachine) Restore(snapshot io.ReadCloser) error {
    // No restore needed
    return nil
}

type DHCPSnapshot struct{}

func (s *DHCPSnapshot) Persist(sink raft.SnapshotSink) error {
    return sink.Cancel()
}

func (s *DHCPSnapshot) Release() {}
```

**Configuration:**
```yaml
# config.yaml
server:
  id: "dhcp-01"

raft:
  enabled: true
  node_id: "dhcp-01"
  bind_addr: "192.168.1.10:7000"
  data_dir: "/var/lib/irondhcp/raft"
  bootstrap: true  # Only true for first node
  peers:
    - id: "dhcp-02"
      addr: "192.168.1.11:7000"
    - id: "dhcp-03"
      addr: "192.168.1.12:7000"
```

**Pros:**
- ✅ No external dependencies (embedded)
- ✅ Full control over behavior
- ✅ Single binary deployment
- ✅ Proven library (used by Consul, Nomad)
- ✅ Automatic failover

**Cons:**
- ❌ Most complex to implement correctly
- ❌ Raft cluster management is non-trivial
- ❌ Need to handle membership changes carefully
- ❌ Still Active/Passive only
- ❌ No web UI unless you build one

---

## Comparison: Raft vs Active/Active

### Active/Passive with Raft

```
Scenario: 3 nodes, 10,000 clients, 1000 leases/sec peak

┌─────────────────────────────────────────────────┐
│ DHCP-01 (Leader)    │ 1000 leases/sec          │
│ DHCP-02 (Follower)  │ 0 leases/sec (standby)   │
│ DHCP-03 (Follower)  │ 0 leases/sec (standby)   │
└─────────────────────────────────────────────────┘

Total Capacity: 1000 leases/sec (only leader serves)
Failover Time: 1-3 seconds
Resource Utilization: 33% (2 nodes idle)
```

### Active/Active (No Raft)

```
Scenario: 3 nodes, 10,000 clients, 1000 leases/sec peak

┌─────────────────────────────────────────────────┐
│ DHCP-01 (Active)    │ ~333 leases/sec          │
│ DHCP-02 (Active)    │ ~333 leases/sec          │
│ DHCP-03 (Active)    │ ~333 leases/sec          │
└─────────────────────────────────────────────────┘

Total Capacity: 1000 leases/sec (distributed)
Failover Time: 0 seconds (no failover needed)
Resource Utilization: 100% (all nodes serve)
```

---

## When to Use Raft

### ✅ Good Fit For:
1. **Simplicity > Performance**: Easier to reason about (only one server active)
2. **Stateful Operations**: Need to coordinate complex state changes
3. **Small Scale**: 1-3 servers, < 10,000 clients
4. **Existing Consul/etcd**: Already have infrastructure
5. **Strong Consistency**: Absolutely cannot tolerate any conflicts

### ❌ Poor Fit For:
1. **High Performance**: Need to handle > 1000 leases/sec
2. **Large Scale**: 10,000+ clients with burst traffic
3. **Resource Efficiency**: Want to use all server capacity
4. **Zero Downtime**: Cannot tolerate 1-3 second failover
5. **Minimal Dependencies**: Want simple deployment

---

## Hybrid Approach: Best of Both Worlds?

### Use Raft for Coordination, Active/Active for Serving

```
┌────────────────────────────────────────────────────────┐
│             Raft Cluster (Coordination)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │ DHCP-01  │  │ DHCP-02  │  │ DHCP-03  │            │
│  │ Raft     │  │ Raft     │  │ Raft     │            │
│  │ Leader   │  │ Follower │  │ Follower │            │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
│       │             │             │                    │
│       └─────────────┴─────────────┘                    │
│              Raft Consensus                            │
│         (For config sync, not DHCP)                    │
└────────────────────────────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│           DHCP Serving (Active/Active)                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │ DHCP-01  │  │ DHCP-02  │  │ DHCP-03  │            │
│  │ ACTIVE   │  │ ACTIVE   │  │ ACTIVE   │            │
│  │ Serving  │  │ Serving  │  │ Serving  │            │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
│       │             │             │                    │
│       └─────────────┴─────────────┘                    │
│              Shared PostgreSQL                         │
└────────────────────────────────────────────────────────┘

Use Cases:
- Raft coordinates: Config changes, cluster membership, health monitoring
- Active/Active handles: DHCP requests (all servers serve simultaneously)
```

**Implementation:**
```go
// All nodes serve DHCP (active/active)
// Raft is used ONLY for:
// 1. Config synchronization (instead of GitOps)
// 2. Cluster membership management
// 3. Health monitoring and alerting
// 4. Coordinating maintenance operations

func (s *Server) Start(ctx context.Context) error {
    // Always start DHCP listener (active/active)
    if err := s.startDHCPListening(); err != nil {
        return err
    }

    // Start Raft for cluster coordination (not for serving)
    if s.config.Raft.Enabled {
        raftCluster, err := cluster.NewRaftCluster(s.config.Raft)
        if err != nil {
            return err
        }

        // Raft leader handles:
        // - Config updates and distribution
        // - Cluster membership changes
        // - Health monitoring coordination
        raftCluster.SetCallbacks(
            func() { s.becomeConfigLeader() },
            func() { s.resignConfigLeader() },
        )

        s.raftCluster = raftCluster
    }

    return nil
}
```

---

## Recommendation

### For ironDHCP Specifically:

**I recommend: Active/Active WITHOUT Raft (Option 1 from previous doc)**

**Reasoning:**

1. **DHCP is Stateless at Protocol Level**
   - Each request is independent
   - Advisory locks handle conflicts
   - No complex state machine needed

2. **Performance Matters**
   - DHCP has strict latency requirements (< 10ms)
   - Active/passive wastes 50%+ capacity
   - Failover creates 1-3 second outage

3. **Simplicity**
   - Raft adds significant complexity
   - Need to manage Raft cluster
   - More things to break

4. **Your Current Architecture**
   - Already have PostgreSQL (shared state)
   - Already have advisory locks (conflict prevention)
   - Already have GitOps (config distribution)

**When Raft WOULD Make Sense:**

If ironDHCP had:
- Complex state machine (like a full DNS server)
- Need for strong consistency (financial transactions)
- Cannot tolerate any conflicts ever
- Very small scale (< 1000 clients)

**Consul/etcd Integration WOULD Make Sense if:**
- You already run Consul/etcd for other services
- Want service discovery integration
- Need multi-datacenter support
- Have ops team familiar with Consul/etcd

---

## Final Architecture Recommendation

### Option 5: Active/Active + Consul/etcd (Optional)

```yaml
# Best of both worlds
server:
  id: "dhcp-01"
  cluster:
    enabled: true
    mode: "active-active"  # All nodes serve DHCP
    coordination: "consul"  # Optional, for enhanced features

# If consul.enabled = false: Use database-only coordination (Option 1)
# If consul.enabled = true: Use Consul for service discovery and monitoring
consul:
  enabled: false  # Optional enhancement
  address: "localhost:8500"
  service_name: "dhcp-server"
  health_check:
    interval: "10s"
    timeout: "5s"
```

**Key Points:**
1. **DHCP Serving**: Always active/active (all nodes serve requests)
2. **State Storage**: PostgreSQL with advisory locks (required)
3. **Coordination**: Consul/etcd optional (nice-to-have for ops visibility)
4. **Failover**: Not needed (active/active inherently HA)

This gives you:
- ✅ Maximum performance (all nodes serve)
- ✅ Maximum availability (no failover delay)
- ✅ Simple core (works without Consul)
- ✅ Enhanced ops (with Consul if desired)
- ✅ Best resource utilization

---

## Code Changes for Consul Integration (Optional)

If you want Consul for service discovery and health checks:

```go
// internal/cluster/consul_integration.go
type ConsulIntegration struct {
    client *consulapi.Client
    config *ConsulConfig
}

func (c *ConsulIntegration) RegisterService(ctx context.Context) error {
    // Register DHCP service with Consul
    registration := &consulapi.AgentServiceRegistration{
        ID:      c.config.ServiceID,
        Name:    "dhcp-server",
        Port:    67,
        Tags:    []string{"dhcp", "active"},
        Check: &consulapi.AgentServiceCheck{
            HTTP:     fmt.Sprintf("http://%s:8080/api/v1/health", c.config.ServiceID),
            Interval: "10s",
            Timeout:  "5s",
        },
    }

    return c.client.Agent().ServiceRegister(registration)
}

// This gives you:
// - Service discovery (clients can find any healthy DHCP server)
// - Health monitoring (Consul tracks which servers are up)
// - Metrics (via Consul's built-in telemetry)
// - Web UI (see all DHCP servers at a glance)

// BUT: Consul is NOT used for leader election
// All servers remain active simultaneously
```

---

## Conclusion

**For ironDHCP:**
- ✅ **Use Active/Active** (Option 1 from HA-DESIGN.md)
- ✅ **Skip Raft** for now (adds complexity without benefit)
- ✅ **Optionally add Consul** later for service discovery and ops visibility
- ✅ **Keep it simple** - PostgreSQL + advisory locks is sufficient

**Raft/Consul is excellent technology**, but it solves a different problem (coordinating state machines). For DHCP, where requests are independent and conflicts are handled by database locks, active/active without Raft is simpler, faster, and more appropriate.

Would you like me to implement Option 1 (active/active without Raft) or would you prefer to explore Consul integration for service discovery and monitoring?
