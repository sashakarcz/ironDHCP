# ironDHCP Web UI

Modern React-based web interface for ironDHCP server.

## Features

- **Dashboard**: Real-time overview of DHCP server statistics
- **Lease Management**: Browse, search, and filter DHCP leases
- **Subnet Overview**: Visual representation of subnet utilization
- **Configuration Viewer**: Display static reservations and boot options
- **GitOps Integration**: Monitor Git sync status and trigger manual syncs
- **Dark Theme**: Modern, easy-on-the-eyes interface
- **Responsive Design**: Works on desktop and mobile devices

## Building

The web UI is automatically embedded into the Go binary during build:

```bash
# From project root
make build        # Automatically builds web if needed
make build-all    # Force rebuild of both web and backend
```

## Development

For frontend development with hot reload:

```bash
cd web
npm install
npm run dev
```

This starts the development server on http://localhost:3000 with:
- Hot module replacement (HMR)
- Proxy to backend API at localhost:8080
- TypeScript type checking

## Deployment

The web UI is embedded in the single `irondhcp` binary - no separate deployment needed!

```bash
./bin/irondhcp
# Web UI available at http://localhost:8080
# API available at http://localhost:8080/api/v1
# Metrics at http://localhost:8080/metrics
```

## Project Structure

```
web/
├── src/
│   ├── api/           # API client and type definitions
│   ├── components/    # Reusable React components
│   ├── pages/         # Page components (routes)
│   ├── types/         # TypeScript type definitions
│   ├── App.tsx        # Main app component with routing
│   ├── main.tsx       # Entry point
│   └── index.css      # Global styles (Tailwind)
├── public/            # Static assets
├── index.html         # HTML template
├── vite.config.ts     # Vite configuration
├── tailwind.config.js # Tailwind CSS configuration
└── package.json       # Dependencies and scripts
```

## Technology Stack

- **React 18** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool and dev server
- **React Router** - Client-side routing
- **Tailwind CSS** - Utility-first CSS framework
- **Lucide React** - Icon library

## API Endpoints

The web UI consumes these REST API endpoints:

- `GET /api/v1/health` - Health check
- `GET /api/v1/dashboard/stats` - Dashboard statistics
- `GET /api/v1/leases` - List all leases
- `GET /api/v1/subnets` - List all subnets
- `GET /api/v1/reservations` - List static reservations
- `GET /api/v1/git/status` - Git sync status
- `GET /api/v1/git/logs` - Git sync history
- `POST /api/v1/git/sync` - Trigger manual sync
- `GET /metrics` - Prometheus metrics

## Configuration

The web UI connects to the API server configured in `vite.config.ts`:

```typescript
proxy: {
  '/api': {
    target: 'http://localhost:8080',
    changeOrigin: true,
  },
}
```

In production (embedded mode), no proxy is needed as everything is served from the same binary.
