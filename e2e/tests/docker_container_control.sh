#!/bin/bash
set -e

# Source the container control helper functions
source /app/e2e/container_control.sh

echo "Testing container control functionality..."

# Check status of peers
echo "Current status of peer containers:"
for peer in peer1 peer2 peer3; do
  status=$(is_peer_running "$peer")
  echo "- $peer is running: $status"
done

# Test stopping and starting peer2
echo "Testing stop/start functionality with peer2..."
stop_peer peer2
sleep 2
status=$(is_peer_running "peer2")
echo "- peer2 is running after stop: $status"

start_peer peer2
sleep 2
status=$(is_peer_running "peer2")
echo "- peer2 is running after start: $status"

# Wait for peer2 to be healthy again
if wait_for_peer_health peer2 60; then
  echo "✅ Successfully controlled peer container"
  exit 0
else
  echo "❌ Failed to bring peer container back to healthy state"
  exit 1
fi
