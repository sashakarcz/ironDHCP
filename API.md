# ironDHCP REST API Documentation

Complete API reference for ironDHCP v0.2.0

## Table of Contents

- [Authentication](#authentication)
- [Endpoints](#endpoints)
  - [Health Check](#health-check)
  - [Dashboard Statistics](#dashboard-statistics)
  - [Leases](#leases)
  - [Subnets](#subnets)
  - [Reservations](#reservations)
  - [GitOps](#gitops)
  - [Activity Stream](#activity-stream)
- [Data Models](#data-models)
- [Error Responses](#error-responses)

## Base URL

All API endpoints are relative to the server's base URL:

```
http://localhost:8080/api/v1
```

## Authentication

ironDHCP uses Bearer token authentication (when `web_auth` is enabled in configuration).

### Login

Obtain an authentication token.

**Endpoint:** `POST /api/v1/login`

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin"
  }'
```

**Response:** `200 OK`
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwiZXhwIjoxNzAwMDAwMDAwfQ.abc123",
  "expires_at": "2025-11-12T12:00:00Z"
}
```

**Using the Token:**

Include the token in the `Authorization` header for all subsequent requests:

```bash
curl -H "Authorization: Bearer eyJhbGc..." \
  http://localhost:8080/api/v1/dashboard/stats
```

**Note:** If `web_auth.enabled` is `false`, authentication is not required and all endpoints are publicly accessible.

---

## Endpoints

### Health Check

Check server health and database connectivity.

**Endpoint:** `GET /api/v1/health`

**Authentication:** Not required

**Request:**
```bash
curl http://localhost:8080/api/v1/health
```

**Response:** `200 OK`
```json
{
  "status": "healthy",
  "database": {
    "status": "healthy",
    "connections": 3,
    "max_conns": 20
  },
  "cache": {
    "size": 0,
    "hit_rate": 0
  },
  "time": "2025-11-11T19:30:54Z"
}
```

**Response:** `503 Service Unavailable` (when unhealthy)
```json
{
  "status": "unhealthy",
  "database": {
    "status": "unhealthy"
  },
  "details": {
    "database_error": "connection refused"
  },
  "time": "2025-11-11T19:30:54Z"
}
```

---

### Dashboard Statistics

Get overview statistics for the dashboard.

**Endpoint:** `GET /api/v1/dashboard/stats`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/dashboard/stats
```

**Response:** `200 OK`
```json
{
  "total_leases": 45,
  "active_leases": 42,
  "expired_leases": 3,
  "total_subnets": 3,
  "total_reservations": 5,
  "uptime": "N/A"
}
```

**Fields:**
- `total_leases`: Total number of leases (active + expired)
- `active_leases`: Number of currently active leases
- `expired_leases`: Number of expired leases
- `total_subnets`: Number of configured subnets
- `total_reservations`: Number of static MAC-to-IP reservations
- `uptime`: Server uptime (currently not tracked)

---

### Leases

List all DHCP leases (both dynamic and static).

**Endpoint:** `GET /api/v1/leases`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/leases
```

**Response:** `200 OK`
```json
[
  {
    "id": 1,
    "ip": "192.168.1.100",
    "mac": "aa:bb:cc:dd:ee:11",
    "hostname": "laptop-01",
    "subnet": "192.168.1.0/24",
    "issued_at": "2025-11-11T10:30:00Z",
    "expires_at": "2025-11-12T10:30:00Z",
    "last_seen": "2025-11-11T18:45:23Z",
    "state": "active",
    "client_id": "01:aa:bb:cc:dd:ee:11",
    "vendor_class": "MSFT 5.0"
  },
  {
    "id": 2,
    "ip": "192.168.1.10",
    "mac": "aa:bb:cc:dd:ee:ff",
    "hostname": "test-server",
    "subnet": "192.168.1.0/24",
    "issued_at": "0001-01-01T00:00:00Z",
    "expires_at": "0001-01-01T00:00:00Z",
    "last_seen": "0001-01-01T00:00:00Z",
    "state": "static",
    "client_id": "",
    "vendor_class": ""
  }
]
```

**Lease States:**
- `active`: Lease is currently active and not expired
- `expired`: Lease has expired but not yet reclaimed
- `released`: Client released the lease
- `static`: Static reservation (never expires)

**Notes:**
- Static reservations have state `"static"` and zero timestamps
- Static reservations only appear if they don't have an active dynamic lease
- Results are not paginated (returns all leases)

---

### Subnets

List all configured subnets with utilization metrics.

**Endpoint:** `GET /api/v1/subnets`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/subnets
```

**Response:** `200 OK`
```json
[
  {
    "network": "192.168.1.0/24",
    "description": "Example Office Network",
    "gateway": "192.168.1.1",
    "dns_servers": [
      "8.8.8.8",
      "8.8.4.4"
    ],
    "lease_duration": "24h0m0s",
    "active_leases": 42,
    "total_ips": 254,
    "utilization": 16.54
  },
  {
    "network": "10.0.10.0/24",
    "description": "IoT Devices",
    "gateway": "10.0.10.1",
    "dns_servers": [
      "10.0.10.1"
    ],
    "lease_duration": "12h0m0s",
    "active_leases": 8,
    "total_ips": 254,
    "utilization": 3.15
  }
]
```

**Fields:**
- `network`: Subnet in CIDR notation
- `description`: Human-readable description
- `gateway`: Default gateway (router) for the subnet
- `dns_servers`: List of DNS servers
- `lease_duration`: DHCP lease duration (Go duration format)
- `active_leases`: Number of currently active leases in this subnet
- `total_ips`: Total usable IP addresses (excludes network and broadcast)
- `utilization`: Percentage of IPs currently allocated (0-100)

---

### Reservations

List all static MAC-to-IP reservations.

**Endpoint:** `GET /api/v1/reservations`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/reservations
```

**Response:** `200 OK`
```json
[
  {
    "id": 1,
    "mac": "aa:bb:cc:dd:ee:ff",
    "ip": "192.168.1.10",
    "hostname": "test-server",
    "subnet": "192.168.1.0/24",
    "description": "Production web server",
    "tftp_server": "",
    "boot_filename": ""
  },
  {
    "id": 2,
    "mac": "11:22:33:44:55:66",
    "ip": "192.168.1.20",
    "hostname": "pxe-client",
    "subnet": "192.168.1.0/24",
    "description": "PXE boot client",
    "tftp_server": "192.168.1.5",
    "boot_filename": "pxelinux.0"
  }
]
```

**Fields:**
- `id`: Database ID
- `mac`: MAC address (canonical format with colons)
- `ip`: Reserved IP address
- `hostname`: Hostname for the reservation
- `subnet`: Subnet this reservation belongs to
- `description`: Human-readable description
- `tftp_server`: Optional TFTP server for PXE boot (DHCP option 66)
- `boot_filename`: Optional boot filename for PXE boot (DHCP option 67)

**Notes:**
- Reservations are persistent and never expire
- When GitOps is enabled, reservations are managed via Git configuration
- PXE boot options override subnet-level settings

---

### GitOps

#### Get Git Status

Get current Git repository status and last sync information.

**Endpoint:** `GET /api/v1/git/status`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/git/status
```

**Response:** `200 OK`
```json
{
  "current_commit": "50b40e89b119f5e9d6ba1da4036244f5f7ea5e72",
  "commit_message": "Add new IoT subnet",
  "commit_author": "John Doe <john@example.com>",
  "commit_time": "2025-11-11T15:30:00Z",
  "last_sync_time": "2025-11-11T15:35:12Z",
  "last_sync_status": "success"
}
```

**Response:** `200 OK` (no sync yet)
```json
{
  "current_commit": "",
  "commit_message": "",
  "commit_author": "",
  "commit_time": "0001-01-01T00:00:00Z",
  "last_sync_time": "0001-01-01T00:00:00Z",
  "last_sync_status": ""
}
```

**Response:** `503 Service Unavailable` (GitOps not enabled)
```json
{
  "success": false,
  "message": "GitOps is not enabled"
}
```

---

#### Trigger Git Sync

Manually trigger a Git repository sync.

**Endpoint:** `POST /api/v1/git/sync`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -X POST \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/git/sync \
  -d '{
    "triggered_by": "admin"
  }'
```

**Request Body:**
```json
{
  "triggered_by": "admin"
}
```

**Fields:**
- `triggered_by`: Username or identifier of who triggered the sync (optional, defaults to "api")

**Response:** `200 OK` (success)
```json
{
  "success": true,
  "message": "Sync completed successfully",
  "commit_hash": "50b40e89b119f5e9d6ba1da4036244f5f7ea5e72",
  "commit_message": "Add new IoT subnet",
  "has_changes": true,
  "changes_applied": {
    "reservations_added": 2,
    "reservations_updated": 1,
    "reservations_deleted": 0,
    "total_subnets": 3,
    "config_reloaded": true
  }
}
```

**Response:** `200 OK` (no changes)
```json
{
  "success": true,
  "message": "Sync completed successfully",
  "commit_hash": "50b40e89b119f5e9d6ba1da4036244f5f7ea5e72",
  "commit_message": "Add new IoT subnet",
  "has_changes": false,
  "changes_applied": {}
}
```

**Response:** `500 Internal Server Error` (sync failed)
```json
{
  "success": false,
  "message": "Configuration validation failed: subnet 0: network is required",
  "has_changes": false
}
```

**Response:** `503 Service Unavailable` (GitOps not enabled)
```json
{
  "success": false,
  "message": "GitOps is not enabled"
}
```

---

#### Get Git Sync Logs

Retrieve recent Git sync operation history.

**Endpoint:** `GET /api/v1/git/logs`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/git/logs
```

**Response:** `200 OK`
```json
{
  "logs": [
    {
      "id": 15,
      "sync_started_at": "2025-11-11T15:35:12Z",
      "sync_completed_at": "2025-11-11T15:35:14Z",
      "status": "success",
      "commit_hash": "50b40e89b119f5e9d6ba1da4036244f5f7ea5e72",
      "commit_message": "Add new IoT subnet",
      "commit_author": "John Doe <john@example.com>",
      "error_message": "",
      "changes_applied": {
        "reservations_added": 2,
        "reservations_updated": 0,
        "reservations_deleted": 0,
        "total_subnets": 3,
        "config_reloaded": true
      },
      "triggered_by": "automatic",
      "triggered_by_user": ""
    },
    {
      "id": 14,
      "sync_started_at": "2025-11-11T14:30:00Z",
      "sync_completed_at": "2025-11-11T14:30:02Z",
      "status": "failed",
      "commit_hash": "abc123def456",
      "commit_message": "Update subnet config",
      "commit_author": "Jane Smith <jane@example.com>",
      "error_message": "Configuration validation failed: subnet 1: range_start and range_end are required",
      "changes_applied": {},
      "triggered_by": "manual",
      "triggered_by_user": "admin"
    }
  ]
}
```

**Sync Statuses:**
- `in_progress`: Sync is currently running
- `success`: Sync completed successfully
- `failed`: Sync failed (see `error_message`)

**Triggered By:**
- `automatic`: Triggered by scheduled polling
- `manual`: Triggered manually via API or web UI

**Notes:**
- Returns up to 50 most recent sync operations
- Ordered by most recent first
- Complete audit trail for compliance

---

### Activity Stream

Stream real-time DHCP activity events via Server-Sent Events (SSE).

**Endpoint:** `GET /api/v1/activity/stream`

**Authentication:** Required (if enabled)

**Request:**
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/activity/stream
```

**Response:** `200 OK` (streaming)
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

data: {"id":"init","timestamp":"2025-11-11T19:30:54Z","type":"connection","message":"Connected to activity stream"}

data: {"id":"dhcp-123","timestamp":"2025-11-11T19:31:05Z","type":"dhcp_discover","message":"DISCOVER from aa:bb:cc:dd:ee:11","details":{"mac":"aa:bb:cc:dd:ee:11","hostname":"laptop-01","subnet":"192.168.1.0/24"}}

data: {"id":"dhcp-124","timestamp":"2025-11-11T19:31:05Z","type":"dhcp_offer","message":"OFFER 192.168.1.100 to aa:bb:cc:dd:ee:11","details":{"mac":"aa:bb:cc:dd:ee:11","ip":"192.168.1.100","subnet":"192.168.1.0/24"}}

data: {"id":"dhcp-125","timestamp":"2025-11-11T19:31:06Z","type":"dhcp_request","message":"REQUEST 192.168.1.100 from aa:bb:cc:dd:ee:11","details":{"mac":"aa:bb:cc:dd:ee:11","ip":"192.168.1.100","subnet":"192.168.1.0/24"}}

data: {"id":"dhcp-126","timestamp":"2025-11-11T19:31:06Z","type":"dhcp_ack","message":"ACK 192.168.1.100 to aa:bb:cc:dd:ee:11 (laptop-01)","details":{"mac":"aa:bb:cc:dd:ee:11","ip":"192.168.1.100","hostname":"laptop-01","subnet":"192.168.1.0/24","lease_duration":"24h"}}
```

**Event Types:**
- `connection`: Initial connection event
- `dhcp_discover`: Client sent DISCOVER packet
- `dhcp_offer`: Server sent OFFER packet
- `dhcp_request`: Client sent REQUEST packet
- `dhcp_ack`: Server sent ACK packet
- `dhcp_nak`: Server sent NAK packet
- `dhcp_release`: Client released lease
- `dhcp_inform`: Client sent INFORM packet
- `lease_expired`: Lease expired
- `git_sync_started`: Git sync operation started
- `git_sync_completed`: Git sync operation completed

**Notes:**
- Long-lived connection (keep-alive)
- Events are sent as they occur in real-time
- Automatic reconnection recommended if connection drops
- Maximum one event per line, newline-delimited

**Client-Side Example (JavaScript):**
```javascript
const eventSource = new EventSource('/api/v1/activity/stream', {
  headers: {
    'Authorization': 'Bearer ' + token
  }
});

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Activity:', data.message);
};

eventSource.onerror = () => {
  console.error('Connection lost, reconnecting...');
};
```

---

## Data Models

### Lease

```typescript
interface Lease {
  id: number;              // Database ID
  ip: string;              // IPv4 address
  mac: string;             // MAC address (canonical format)
  hostname: string;        // Client hostname
  subnet: string;          // Subnet in CIDR notation
  issued_at: string;       // ISO 8601 timestamp
  expires_at: string;      // ISO 8601 timestamp
  last_seen: string;       // ISO 8601 timestamp
  state: string;           // "active", "expired", "released", "static"
  client_id: string;       // DHCP client identifier
  vendor_class: string;    // Vendor class identifier
}
```

### Subnet

```typescript
interface Subnet {
  network: string;         // CIDR notation (e.g., "192.168.1.0/24")
  description: string;     // Human-readable description
  gateway: string;         // Default gateway IP
  dns_servers: string[];   // List of DNS server IPs
  lease_duration: string;  // Go duration format (e.g., "24h0m0s")
  active_leases: number;   // Current number of active leases
  total_ips: number;       // Total usable IPs (excludes network/broadcast)
  utilization: number;     // Percentage 0-100
}
```

### Reservation

```typescript
interface Reservation {
  id: number;              // Database ID
  mac: string;             // MAC address (canonical format)
  ip: string;              // Reserved IPv4 address
  hostname: string;        // Hostname
  subnet: string;          // Subnet in CIDR notation
  description: string;     // Human-readable description
  tftp_server?: string;    // Optional TFTP server IP
  boot_filename?: string;  // Optional PXE boot filename
}
```

### Git Sync Log

```typescript
interface GitSyncLog {
  id: number;                      // Database ID
  sync_started_at: string;         // ISO 8601 timestamp
  sync_completed_at?: string;      // ISO 8601 timestamp (null if in progress)
  status: string;                  // "in_progress", "success", "failed"
  commit_hash: string;             // Git commit SHA
  commit_message: string;          // Commit message
  commit_author: string;           // Commit author
  error_message?: string;          // Error message if failed
  changes_applied?: object;        // Changes applied (JSON)
  triggered_by: string;            // "automatic" or "manual"
  triggered_by_user?: string;      // Username if manual
}
```

### Activity Event

```typescript
interface ActivityEvent {
  id: string;              // Unique event ID
  timestamp: string;       // ISO 8601 timestamp
  type: string;            // Event type (see Activity Stream)
  message: string;         // Human-readable message
  details?: object;        // Additional event-specific data
}
```

---

## Error Responses

### Standard Error Format

All error responses follow this format:

```json
{
  "error": "Error description",
  "code": "ERROR_CODE",
  "details": {}
}
```

### HTTP Status Codes

- `200 OK`: Request succeeded
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: Valid token but insufficient permissions
- `404 Not Found`: Resource not found
- `405 Method Not Allowed`: HTTP method not allowed for this endpoint
- `500 Internal Server Error`: Server error
- `503 Service Unavailable`: Service temporarily unavailable

### Common Errors

**Invalid Credentials:**
```json
{
  "error": "Invalid username or password"
}
```

**Missing Authentication:**
```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer

Unauthorized
```

**Invalid Token:**
```json
{
  "error": "Invalid or expired token"
}
```

**GitOps Not Enabled:**
```json
{
  "success": false,
  "message": "GitOps is not enabled"
}
```

**Database Connection Failed:**
```json
{
  "status": "unhealthy",
  "database": {
    "status": "unhealthy"
  },
  "details": {
    "database_error": "connection refused"
  }
}
```

---

## Rate Limiting

Currently, there is no rate limiting implemented. This may be added in future versions.

## Versioning

The API is versioned via the URL path (`/api/v1/`). Breaking changes will result in a new version (`/api/v2/`).

## Support

- **Issues**: https://github.com/sashakarcz/irondhcp/issues
- **Documentation**: https://github.com/sashakarcz/irondhcp
- **Author**: Sasha Karcz

---

*Last updated: 2025-11-11*
*API Version: v1*
*ironDHCP Version: 0.2.0*
