# ironDHCP Docker Test Environment

## Overview

I've created a complete Docker test environment for ironDHCP where you can test DHCP functionality in isolation.

## Files Created

1. **docker-compose-test.yml** - Test environment orchestration
2. **config-docker-test.yaml** - Test configuration
3. **test-dhcp-client.sh** - Client test script
4. **test-docker-dhcp.sh** - Main test runner

## Quick Start

```bash
# Run the complete test
./test-docker-dhcp.sh
```

## Manual Testing

```bash
# Start services
docker-compose -f docker-compose-test.yml up -d

# View logs
docker-compose -f docker-compose-test.yml logs -f irondhcp

# Run DHCP client test
docker-compose -f docker-compose-test.yml exec dhcp-client sh /test-dhcp-client.sh

# Stop all
docker-compose -f docker-compose-test.yml down

# Clean everything (including volumes)
docker-compose -f docker-compose-test.yml down -v
```

## Network Configuration

- **Subnet**: 10.200.100.0/24
- **Gateway**: 10.200.100.1 (Docker bridge)
- **ironDHCP Server**: 10.200.100.10
- **PostgreSQL**: 10.200.100.2
- **DHCP Pool**: 10.200.100.100 - 10.200.100.200

## Services

- **Web UI**: http://localhost:8080 (admin/admin)
- **Prometheus**: http://localhost:9090/metrics
- **PostgreSQL**: localhost:5432

## Current Issue

The DHCP server is receiving DISCOVER packets but failing with:
```
ERROR: failed to allocate IP: no available IPs in any pool
```

### Debugging Steps

1. **Verify subnet configuration loaded**:
   ```bash
   docker-compose -f docker-compose-test.yml exec irondhcp cat /etc/irondhcp/config.yaml
   ```

2. **Check database for existing leases**:
   ```bash
   docker-compose -f docker-compose-test.yml exec postgres psql -U dhcp -d irondhcp -c "SELECT * FROM leases;"
   ```

3. **Check reservations**:
   ```bash
   docker-compose -f docker-compose-test.yml exec postgres psql -U dhcp -d irondhcp -c "SELECT * FROM reservations;"
   ```

4. **Enable more verbose logging**:
   Edit `config-docker-test.yaml` and change `log_level: debug` (already done)

5. **Check how pools are loaded**:
   The issue appears to be that pools aren't being properly passed to the allocator. Check `internal/dhcp/server.go` where configuration is loaded into the `SubnetConfig` structures.

### Potential Root Causes

1. **Pools not loaded**: The `subnet.Pools` might be empty when passed to the allocator
2. **Type mismatch**: Pool structure in config vs. internal representation
3. **Parsing issue**: YAML parsing might not be correctly loading the pools section

### Next Steps for Debugging

Add debug logging in `internal/dhcp/allocator.go` line 64:
```go
logger.Debug().Int("pool_count", len(req.Pools)).Msg("Allocating from pools")
```

And in `allocateFromPool` to see what's happening:
```go
logger.Debug().
    Str("range_start", pool.RangeStart).
    Str("range_end", pool.RangeEnd).
    Msg("Trying to allocate from pool")
```

## Production Deployment Notes

For production:
1. Change passwords in `config-docker.yaml` (bcrypt hash)
2. Set `POSTGRES_PASSWORD` environment variable
3. Consider using `network_mode: host` for real DHCP functionality
4. Enable GitOps for configuration management
5. Set up proper monitoring with Prometheus

## Testing on Real Network

The main issue you encountered on your wlan0 network:

1. **Existing DHCP server** at 172.16.4.1 responding faster
2. **YAML syntax error** in your Git repo (line 91 of dhcp.yaml) - need to fix the indentation
3. **No subnet for 172.16.4.0/24** initially (you added it)

### Fix the Git Repo

Edit `https://github.com/sashakarcz/irondhcp-config/blob/main/dhcp.yaml` line 88-92:

**Before (broken)**:
```yaml
    dns_servers:
      - 172.16.5.1
      - 1.1.1.1
        # Lease time
    lease_duration: 24h
```

**After (fixed)**:
```yaml
    dns_servers:
      - 172.16.5.1
      - 1.1.1.1

    # Lease time
    lease_duration: 24h
```

Then:
```bash
rm -rf /tmp/irondhcp-git
sudo ./bin/irondhcp --config config.yaml
```

The server should now load all 4 subnets successfully.
