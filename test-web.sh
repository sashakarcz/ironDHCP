#!/bin/bash

# Test script to verify web UI is working

echo "Testing ironDHCP Web UI..."
echo ""

# Start the server in background (will fail on port 67 but API will work)
./bin/irondhcp &
SERVER_PID=$!

# Wait for server to start
echo "Waiting for server to start..."
sleep 3

echo ""
echo "Testing endpoints:"
echo ""

# Test root HTML
echo "1. Testing / (HTML)"
curl -s -o /dev/null -w "Status: %{http_code}\n" http://localhost:8080/

# Test JS asset
echo "2. Testing /assets/index-sstwcZ58.js"
curl -s -o /dev/null -w "Status: %{http_code}, Size: %{size_download} bytes\n" http://localhost:8080/assets/index-sstwcZ58.js

# Test CSS asset
echo "3. Testing /assets/index-Ds-wrY1T.css"
curl -s -o /dev/null -w "Status: %{http_code}, Size: %{size_download} bytes\n" http://localhost:8080/assets/index-Ds-wrY1T.css

# Test API health
echo "4. Testing /api/v1/health"
curl -s http://localhost:8080/api/v1/health | head -1

echo ""
echo "Kill server..."
kill $SERVER_PID 2>/dev/null

echo ""
echo "✓ Test complete"
echo "Open http://localhost:8080 in your browser"
