# Quick Start Guide

This guide will help you get ironDHCP running in development mode.

## Prerequisites

- Go 1.21 or later
- Docker and Docker Compose (for PostgreSQL)
- Root/sudo access (DHCP requires binding to port 67)

## Step 1: Start PostgreSQL

Start the local PostgreSQL database:

```bash
make dev-db
```

This will:
- Start PostgreSQL on port 5432
- Create the `irondhcp` database
- Run migrations automatically
- Use credentials: `dhcp` / `dhcp_dev_password`

To check if it's running:
```bash
docker ps
```

## Step 2: Build ironDHCP

```bash
make build
```

This creates the binary at `bin/irondhcp`.

## Step 3: Review Configuration

The example configuration is in `example-config.yaml`. Key settings:

- **Interface**: `eth0` (change to your actual interface, e.g., `eth0`, `ens33`, `wlan0`)
- **Subnet**: `192.168.1.0/24` (adjust to match your network)
- **Pool**: `192.168.1.100-200` (IPs to hand out)
- **Database**: Already configured for local dev database

Edit `example-config.yaml` to match your network.

## Step 4: Run ironDHCP

⚠️ **Important**: DHCP requires root/sudo to bind to port 67:

```bash
sudo ./bin/irondhcp --config example-config.yaml
```

You should see output like:
```
   ___  ___  ___  _  _  ___ ___
  / _ \/ _ \|   \| || |/ __| _ \
 | (_) (_) | |) | __ | (__|  _/
  \___/\___/|___/|_||_|\___|_|

  Simple, Production-Ready DHCP Server
  Version: 0.1.0 (Phase 1 MVP)

{"level":"info","time":"...","message":"Starting ironDHCP server"}
{"level":"info","time":"...","message":"Database connection established"}
{"level":"info","time":"...","message":"Starting DHCP server"}
{"level":"info","time":"...","message":"Starting API server","port":8080}
{"level":"info","time":"...","message":"ironDHCP server is running. Press Ctrl+C to stop."}
```

## Step 5: Test the Server

### Check Health Endpoint

```bash
curl http://localhost:8080/api/v1/health
```

Expected output:
```json
{
  "status": "healthy",
  "database": {
    "status": "healthy",
    "connections": 1,
    "max_conns": 20
  },
  "time": "2025-01-15T10:30:00Z"
}
```

### Test DHCP with a Client

From another machine on the same network (or a VM):

```bash
# Release current DHCP lease
sudo dhclient -r

# Request new lease
sudo dhclient -v
```

Check the ironDHCP logs - you should see DISCOVER, OFFER, REQUEST, ACK messages.

### View Leases in Database

```bash
docker exec -it irondhcp-postgres-1 psql -U dhcp -d irondhcp -c "SELECT ip, mac, hostname, state FROM leases;"
```

## Troubleshooting

### "Permission denied" when starting

DHCP requires root to bind to port 67:
```bash
sudo ./bin/irondhcp --config example-config.yaml
```

### "Failed to connect to database"

Make sure PostgreSQL is running:
```bash
make dev-db
docker ps
```

### "Cannot determine subnet for request"

- Check that your `example-config.yaml` interface matches your actual network interface (`ip a` to list)
- Ensure the subnet CIDR matches your network

### No DHCP clients getting IPs

1. Check firewall (allow UDP port 67)
2. Ensure you're on the correct network interface
3. Check logs for errors: `sudo ./bin/irondhcp --config example-config.yaml 2>&1 | tee dhcp.log`

## Stopping

Press `Ctrl+C` in the ironDHCP terminal to gracefully shut down.

To stop PostgreSQL:
```bash
make dev-db-down
```

## Next Steps

- Review [CONFIGURATION.md](CONFIGURATION.md) for detailed config options (coming soon)
- Set up multiple subnets
- Add static reservations
- Configure GitOps (Phase 2)
- Set up monitoring with Prometheus (Phase 2)

## Development

Run tests:
```bash
make test
```

View logs in JSON format:
```bash
sudo ./bin/irondhcp --config example-config.yaml | jq
```

View logs in human-readable format (edit `example-config.yaml` and set `log_format: text`):
```bash
sudo ./bin/irondhcp --config example-config.yaml
```
