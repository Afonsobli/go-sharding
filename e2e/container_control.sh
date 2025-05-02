#!/bin/bash

# Helper functions to control peer containers
# This script can be sourced in test scripts

# Get the project name prefix
PROJECT_NAME=${DOCKER_COMPOSE_PROJECT_NAME:-shard_test}

# Stop a peer container
stop_peer() {
  local peer=$1
  echo "Stopping peer container: $peer"
  docker stop "${PROJECT_NAME}-${peer}-1"
}

# Start a peer container
start_peer() {
  local peer=$1
  echo "Starting peer container: $peer"
  docker start "${PROJECT_NAME}-${peer}-1"
}

# Restart a peer container
restart_peer() {
  local peer=$1
  echo "Restarting peer container: $peer"
  docker restart "${PROJECT_NAME}-${peer}-1"
}

# Get peer container logs
get_peer_logs() {
  local peer=$1
  local lines=${2:-100}
  echo "Getting logs for peer container: $peer (last $lines lines)"
  docker logs "${PROJECT_NAME}-${peer}-1" --tail "$lines"
}

# Check if a peer container is running
is_peer_running() {
  local peer=$1
  docker inspect --format '{{.State.Running}}' "${PROJECT_NAME}-${peer}-1" 2>/dev/null || echo "false"
}

# Wait for a peer to be healthy
wait_for_peer_health() {
  local peer=$1
  local timeout=${2:-30}
  local count=0
  
  echo "Waiting for peer $peer to be healthy (timeout: ${timeout}s)..."
  while [ $count -lt $timeout ]; do
    if [ "$(docker inspect --format '{{.State.Health.Status}}' "${PROJECT_NAME}-${peer}-1" 2>/dev/null)" = "healthy" ]; then
      echo "Peer $peer is healthy"
      return 0
    fi
    sleep 1
    count=$((count+1))
  done
  
  echo "Timeout waiting for peer $peer to be healthy"
  return 1
}
