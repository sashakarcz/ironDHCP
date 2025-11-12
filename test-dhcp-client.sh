#!/bin/sh
# Test script for DHCP client in Docker

echo "========================================="
echo "ironDHCP Test Client"
echo "========================================="
echo ""

# Install required packages
echo "[1/4] Installing tools..."
apk add --no-cache iproute2 curl jq > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✓ Tools installed"
else
    echo "✗ Failed to install tools"
    exit 1
fi

echo ""
echo "[2/4] Current network configuration:"
ip addr show eth0 | grep -E "inet |link/ether"

echo ""
echo "[3/4] Releasing any existing DHCP lease..."
ip addr flush dev eth0 2>/dev/null
ip addr add 0.0.0.0/0 dev eth0 2>/dev/null

echo ""
echo "[4/4] Requesting DHCP lease from ironDHCP server..."
echo "----------------------------------------"
# Use Alpine's built-in udhcpc
udhcpc -i eth0 -f -q

echo ""
echo "========================================="
echo "DHCP Test Results"
echo "========================================="
ip addr show eth0 | grep "inet " && echo "✓ IP address assigned successfully" || echo "✗ No IP address assigned"

# Get assigned IP
ASSIGNED_IP=$(ip addr show eth0 | grep "inet " | awk '{print $2}' | cut -d/ -f1)

if [ -n "$ASSIGNED_IP" ]; then
    echo ""
    echo "Assigned IP: $ASSIGNED_IP"
    echo "Gateway: $(ip route | grep default | awk '{print $3}')"
    echo "DNS Servers: $(cat /etc/resolv.conf | grep nameserver | awk '{print $2}' | tr '\n' ' ')"

    echo ""
    echo "========================================="
    echo "Checking ironDHCP API"
    echo "========================================="

    # Query ironDHCP API for lease info
    echo ""
    echo "Active leases on server:"
    curl -s http://10.200.100.1:8080/api/v1/leases | jq '.[] | {ip, mac, hostname, state}' 2>/dev/null || echo "Could not query API (auth may be required)"

    echo ""
    echo "Subnets configured:"
    curl -s http://10.200.100.1:8080/api/v1/subnets | jq '.[] | {network, active_leases, total_ips}' 2>/dev/null || echo "Could not query API (auth may be required)"
fi

echo ""
echo "========================================="
echo "Test complete!"
echo "========================================="
