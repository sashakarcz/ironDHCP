#!/bin/bash

echo "Starting ironDHCP with GitOps config..."
./bin/irondhcp --config config.yaml > /tmp/irondhcp-gitops.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
sleep 6

echo -e "\nTesting subnets endpoint:"
curl -s http://localhost:8080/api/v1/subnets | jq '.[] | {network, description, gateway, total_ips, active_leases}'

echo -e "\nTesting leases endpoint:"
curl -s http://localhost:8080/api/v1/leases | jq '.[0] | {ip, mac, hostname, state}'

echo -e "\nKilling server..."
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
echo "Done"
