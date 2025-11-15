# Multi-stage build for ironDHCP
# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

# Copy package files
COPY web/package*.json ./

# Install dependencies
RUN npm ci

# Copy web source
COPY web/ ./

# Build frontend
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.23-alpine AS go-builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code (includes migrations in internal/storage/migrations/)
COPY . .

# Copy built frontend from previous stage
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN cp -r web/dist internal/api/dist

# Build the binary (migrations and frontend are embedded via go:embed)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o irondhcp ./cmd/godhcp

# Stage 3: Runtime image
FROM alpine:latest

# Install runtime dependencies (including git for GitOps)
RUN apk add --no-cache ca-certificates tzdata git openssh-client

# Create non-root user (but we'll need root for DHCP port 67)
RUN addgroup -g 1000 irondhcp && \
    adduser -D -u 1000 -G irondhcp irondhcp

WORKDIR /app

# Copy binary from builder (migrations are now embedded)
COPY --from=go-builder /app/irondhcp .

# Create config directory
RUN mkdir -p /etc/irondhcp /var/lib/irondhcp && \
    chown -R irondhcp:irondhcp /app /etc/irondhcp /var/lib/irondhcp

# Expose ports
# 67/udp - DHCP server
# 8080/tcp - Web UI and API
# 9090/tcp - Prometheus metrics
EXPOSE 67/udp 8080/tcp 9090/tcp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Note: DHCP requires binding to port 67, which needs root privileges
# In production, consider using CAP_NET_BIND_SERVICE capability instead
USER root

ENTRYPOINT ["/app/irondhcp"]
CMD ["--config", "/etc/irondhcp/config.yaml"]
