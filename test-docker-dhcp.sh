#!/bin/bash
# Main test script for running ironDHCP in Docker

set -e

echo "========================================="
echo "ironDHCP Docker Test Environment"
echo "========================================="
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker is not running"
    exit 1
fi

# Clean up any existing test environment
echo "[1/5] Cleaning up previous test environment..."
docker-compose -f docker-compose-test.yml down -v 2>/dev/null || true
echo "✓ Cleanup complete"

echo ""
echo "[2/5] Building ironDHCP Docker image..."
docker-compose -f docker-compose-test.yml build
echo "✓ Build complete"

echo ""
echo "[3/5] Starting services (PostgreSQL, ironDHCP)..."
docker-compose -f docker-compose-test.yml up -d postgres irondhcp
echo "✓ Services started"

echo ""
echo "[4/5] Waiting for ironDHCP to be ready..."
for i in {1..30}; do
    if docker-compose -f docker-compose-test.yml exec -T irondhcp wget --spider -q http://localhost:8080/api/v1/health 2>/dev/null; then
        echo "✓ ironDHCP is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "✗ Timeout waiting for ironDHCP"
        echo ""
        echo "Server logs:"
        docker-compose -f docker-compose-test.yml logs irondhcp
        exit 1
    fi
    sleep 1
done

echo ""
echo "[5/5] Starting DHCP client test..."
docker-compose -f docker-compose-test.yml up -d dhcp-client
sleep 2

echo ""
echo "========================================="
echo "Running DHCP Test"
echo "========================================="
docker-compose -f docker-compose-test.yml exec dhcp-client sh /test-dhcp-client.sh

echo ""
echo "========================================="
echo "Test Environment Running"
echo "========================================="
echo ""
echo "Services:"
echo "  • Web UI:      http://localhost:8080 (admin/admin)"
echo "  • Prometheus:  http://localhost:9090/metrics"
echo "  • PostgreSQL:  localhost:5432"
echo ""
echo "Commands:"
echo "  • View logs:        docker-compose -f docker-compose-test.yml logs -f"
echo "  • View ironDHCP:    docker-compose -f docker-compose-test.yml logs -f irondhcp"
echo "  • Exec into client: docker-compose -f docker-compose-test.yml exec dhcp-client sh"
echo "  • Re-test DHCP:     docker-compose -f docker-compose-test.yml exec dhcp-client sh /test-dhcp-client.sh"
echo "  • Stop all:         docker-compose -f docker-compose-test.yml down"
echo "  • Stop & cleanup:   docker-compose -f docker-compose-test.yml down -v"
echo ""
