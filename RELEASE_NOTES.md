# ironDHCP v1.0.0 Release Notes

## Overview

First stable release of ironDHCP - a modern, production-ready DHCP server with GitOps integration and embedded web interface.

## What's New in v1.0.0

### Embedded Architecture
- All static assets are now embedded in the binary
- Web UI (React SPA) embedded via Go embed directive
- SQL migrations embedded in the binary
- Single binary deployment - only config.yaml required externally

### Mobile-Responsive Web UI
- Fully responsive design that works on desktop, tablet, and mobile devices
- Hamburger menu navigation for mobile screens
- Optimized layout and spacing for small screens
- Touch-friendly interface elements

### Activity Stream Improvements
- Fixed Server-Sent Events (SSE) authentication
- Support for query parameter tokens (EventSource compatibility)
- Real-time event streaming with proper auth handling

### Production Readiness
- Comprehensive error handling and validation
- Database connection pooling and health checks
- Graceful shutdown with context cancellation
- Production-ready Docker configuration
- Complete API documentation

## Features

### Core DHCP Server
- RFC 2131/2132 compliant DHCP implementation
- LRU-based IP allocation with 10,000-entry in-memory cache
- Static MAC-to-IP reservations
- Multiple subnet support with per-subnet configuration
- PXE/iPXE network boot support
- High availability with shared PostgreSQL backend

### GitOps Integration
- Git repository polling and automatic sync
- Configuration validation before apply
- Atomic configuration reload with zero downtime
- Complete audit trail in PostgreSQL
- Manual sync trigger via API and web UI
- Support for SSH and HTTPS Git authentication

### Web Interface
- Modern React SPA with dark theme
- Mobile-responsive design
- Real-time dashboard with statistics
- Lease browser with search and filtering
- Subnet overview with utilization metrics
- Live activity log with Server-Sent Events
- Git sync status and history
- Bearer token authentication

### Observability
- Prometheus metrics endpoint
- Structured JSON logging with zerolog
- Real-time activity stream via SSE
- Git sync audit log with full history
- Automatic lease expiry worker

## Deployment

### Binary Size
- 23MB (includes web UI and SQL migrations)

### Requirements
- PostgreSQL 12+
- Linux (or any OS supporting Go binaries)
- Only external file needed: config.yaml

### Installation
```bash
# Download binary
wget https://github.com/sashakarcz/irondhcp/releases/download/v1.0.0/irondhcp

# Make executable
chmod +x irondhcp

# Create config
cp example-config.yaml config.yaml
# Edit config.yaml with your settings

# Run
sudo ./irondhcp --config config.yaml
```

## Breaking Changes from v0.2.0

### Database Migrations
- Migrations moved from `/migrations/` to `/internal/storage/migrations/`
- Migrations are now embedded in the binary
- No action required - migrations run automatically on startup

### Build Process
- Frontend build artifacts now git-ignored
- `make build` automatically embeds frontend if not present
- Recommended: `make build-all` to force rebuild of everything

## Known Issues

None at this time.

## Upgrade Path from v0.2.0

1. Backup your database
2. Stop the old ironDHCP server
3. Replace the binary with v1.0.0
4. Start the new server
5. Database migrations will run automatically

The configuration file format is fully compatible.

## Documentation

- [README.md](README.md) - Complete documentation
- [API.md](API.md) - Full API reference
- [API-QUICKREF.md](API-QUICKREF.md) - Quick API reference

## Contributors

Sasha Karcz

## License

MIT License - See LICENSE file for details
