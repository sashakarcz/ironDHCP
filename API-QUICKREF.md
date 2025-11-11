# ironDHCP API Quick Reference

Fast reference for common API operations.

## Authentication

```bash
# Get token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

# Use token in requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/leases
```

## Endpoints Summary

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| `POST` | `/api/v1/login` | Get authentication token | No |
| `GET` | `/api/v1/health` | Health check | No |
| `GET` | `/api/v1/dashboard/stats` | Dashboard statistics | Yes |
| `GET` | `/api/v1/leases` | List all leases | Yes |
| `GET` | `/api/v1/subnets` | List subnets | Yes |
| `GET` | `/api/v1/reservations` | List reservations | Yes |
| `GET` | `/api/v1/git/status` | Git repository status | Yes |
| `GET` | `/api/v1/git/logs` | Git sync history | Yes |
| `POST` | `/api/v1/git/sync` | Trigger Git sync | Yes |
| `GET` | `/api/v1/activity/stream` | Real-time activity (SSE) | Yes |

## Common Operations

### Check Health
```bash
curl http://localhost:8080/api/v1/health
```

### Get Dashboard Stats
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/dashboard/stats | jq
```

### List Active Leases
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/leases | jq '.[] | select(.state=="active")'
```

### Find Lease by IP
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/leases | jq '.[] | select(.ip=="192.168.1.100")'
```

### Find Lease by MAC
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/leases | jq '.[] | select(.mac=="aa:bb:cc:dd:ee:ff")'
```

### Get Subnet Utilization
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subnets | \
  jq '.[] | "\(.network): \(.utilization)% (\(.active_leases)/\(.total_ips))"'
```

### List Static Reservations
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/reservations | jq
```

### Get Git Status
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/git/status | jq
```

### Trigger Manual Sync
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/git/sync \
  -d '{"triggered_by":"admin"}' | jq
```

### View Recent Git Syncs
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/git/logs | jq '.logs[] | "\(.status): \(.commit_hash[0:7]) - \(.commit_message)"'
```

### Monitor Activity Stream
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/activity/stream
```

## Response Examples

### Health Check
```json
{
  "status": "healthy",
  "database": {
    "status": "healthy",
    "connections": 3,
    "max_conns": 20
  },
  "time": "2025-11-11T19:30:54Z"
}
```

### Dashboard Stats
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

### Lease
```json
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
}
```

### Subnet
```json
{
  "network": "192.168.1.0/24",
  "description": "Example Office Network",
  "gateway": "192.168.1.1",
  "dns_servers": ["8.8.8.8", "8.8.4.4"],
  "lease_duration": "24h0m0s",
  "active_leases": 42,
  "total_ips": 254,
  "utilization": 16.54
}
```

## Error Handling

```bash
# Check response status
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/leases)

if [ "$HTTP_CODE" -eq 200 ]; then
  echo "Success"
elif [ "$HTTP_CODE" -eq 401 ]; then
  echo "Unauthorized - token expired or invalid"
elif [ "$HTTP_CODE" -eq 500 ]; then
  echo "Server error"
fi
```

## Automation Examples

### Monitor Subnet Usage
```bash
#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subnets | \
  jq -r '.[] | select(.utilization > 80) |
    "WARNING: \(.network) is \(.utilization)% full (\(.active_leases)/\(.total_ips))"'
```

### Export Leases to CSV
```bash
#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

echo "IP,MAC,Hostname,State,Subnet,Expires"
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/leases | \
  jq -r '.[] | [.ip, .mac, .hostname, .state, .subnet, .expires_at] | @csv'
```

### Sync Config from Git
```bash
#!/bin/bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

RESULT=$(curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/git/sync \
  -d '{"triggered_by":"automation"}')

SUCCESS=$(echo "$RESULT" | jq -r '.success')
MESSAGE=$(echo "$RESULT" | jq -r '.message')

if [ "$SUCCESS" = "true" ]; then
  echo "✓ Sync successful: $MESSAGE"
  echo "$RESULT" | jq '.changes_applied'
else
  echo "✗ Sync failed: $MESSAGE"
  exit 1
fi
```

## See Also

- Full API documentation: [API.md](./API.md)
- Main README: [README.md](./README.md)
- Configuration examples: [example-config.yaml](./example-config.yaml)
