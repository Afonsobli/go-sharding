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

inspect_network() {
  local network=$1
  docker network inspect $network
}

# Disconnect peer from the network
disconnect_peer_from_network() {
  local peer=$1
  local network=${2:-"${PROJECT_NAME}_default"}
  echo "Disconnecting peer $peer from network: $network"
  
  # Check if peer is already disconnected
  if ! inspect_network $network | grep -q "${PROJECT_NAME}-${peer}-1"; then
    echo "Peer $peer is already disconnected from network $network"
    return 0
  fi
  
  # Disconnect peer from network
  disconnect_from_network "$peer" "$network" || {
    echo "Failed to disconnect peer $peer from network $network"
    return 1
  }
  
  return 0
}

disconnect_from_network() {
  local peer=$1
  local network=$2
  docker network disconnect $network "${PROJECT_NAME}-${peer}-1"
}

# Connect peer to the network
connect_peer_to_network() {
  local peer=$1
  local network=${2:-"${PROJECT_NAME}_default"}
  echo "Connecting peer $peer to network: $network"
  
  # Check if peer is already connected
  if inspect_network $network | grep -q "${PROJECT_NAME}-${peer}-1"; then
    echo "Peer $peer is already connected to network $network"
    return 0
  fi
  
  # Connect peer to network
  connect_to_network "$peer" "$network" || {
    echo "Failed to connect peer $peer to network $network"
    return 1
  }
  
  return 0
}

connect_to_network() {
  local peer=$1
  local network=$2
  docker network connect $network "${PROJECT_NAME}-${peer}-1"
}

# Get peer IP address
get_peer_ip() {
  local peer=$1
  local network=${2:-"${PROJECT_NAME}_default"}
  docker inspect -f "{{with index .NetworkSettings.Networks \"${network}\"}}{{.IPAddress}}{{end}}" "${PROJECT_NAME}-${peer}-1"
}

# Test connection between peers
test_peer_connection() {
  local orig=$1
  local dest=$2
  
  # Add retry logic
  local max_retries=3
  local retry=0
  local result=""
  
  while [ $retry -lt $max_retries ]; do
    result=$(docker exec "${PROJECT_NAME}-${orig}-1" curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 "http://${dest}:8080/health" 2>/dev/null || echo "failed")
    
    # If successful, break out of loop
    if [ "$result" = "200" ]; then
      break
    fi
    
    retry=$((retry + 1))
    sleep 1
  done
  
  echo "$result"
}

check_file_exists_in_peer() {
  local peer=$1
  local hash=$2
  docker exec "${PROJECT_NAME}-${peer}-1" sh -c "curl -s -o /dev/null -w \"%{http_code}\" http://localhost:8080/file?hash=$hash" 2>/dev/null || echo "failed"
}

# More robust implementation of DNS resolution
ensure_dns_resolution() {
  local peer1_ip=$1
  local peer2_ip=$2
  local peer3_ip=$3

  # Only proceed with host entries if we have IP addresses
  if [ -n "$peer1_ip" ] && [ -n "$peer2_ip" ] && [ -n "$peer3_ip" ]; then
      echo "Adding host entries to ensure DNS resolution..."
      
      # Update the current container's hosts file first
      # Remove any existing entries to avoid duplicates
      sed -i '/peer[1-3]/d' /etc/hosts 2>/dev/null || true
      
      echo "$peer1_ip peer1" >> /etc/hosts
      echo "$peer2_ip peer2" >> /etc/hosts
      echo "$peer3_ip peer3" >> /etc/hosts
      echo "Added host entries to current container"
      
      # Update peer containers' hosts files
      for container in "${PROJECT_NAME}-peer1-1" "${PROJECT_NAME}-peer2-1" "${PROJECT_NAME}-peer3-1"; do
          if docker inspect "$container" &>/dev/null; then
              # Remove any existing entries
              docker exec $container sh -c "sed -i '/peer[1-3]/d' /etc/hosts" 2>/dev/null || true
              
              docker exec $container sh -c "echo '$peer1_ip peer1' >> /etc/hosts"
              docker exec $container sh -c "echo '$peer2_ip peer2' >> /etc/hosts"
              docker exec $container sh -c "echo '$peer3_ip peer3' >> /etc/hosts"
              echo "Added host entries to $container"
          else
              echo "Warning: Container $container not found, skipping host entry update"
          fi
      done
      
      # Verify host entries were added correctly to this container
      echo "Verifying host entries in current container:"
      grep -E "peer[1-3]" /etc/hosts || echo "No peer entries found in hosts file"
  else
      echo "Warning: Could not get IP addresses for all peers. DNS resolution may still be an issue."
  fi
}

# Function to reset the test environment
reset_test_environment() {
  echo "Resetting test environment..."
  
  local network=${1:-"${PROJECT_NAME}_default"}
  
  # Refresh all network connections to fix potential one-way connectivity issues
  for peer in peer1 peer2 peer3; do
    if inspect_network "$network" | grep -q "${PROJECT_NAME}-${peer}-1"; then
      # Already connected, refresh by disconnecting and reconnecting
      echo "Refreshing network connection for $peer..."
      disconnect_from_network "$peer" "$network" 2>/dev/null || true
      sleep 1
    fi
    # Connect/reconnect to network
    echo "Connecting $peer to network..."
    connect_to_network "$peer" "$network" 2>/dev/null || true
    sleep 1
  done
  
  # Restart all peers
  echo "Restarting all peers..."
  for peer in peer1 peer2 peer3; do
    restart_peer "$peer"
    sleep 2  # Give a bit more time between restarts
  done
  
  # Wait for all peers to be healthy
  for peer in peer1 peer2 peer3; do
    wait_for_peer_health "$peer" 30
  done
  
  # Get IP addresses and update DNS resolution
  PEER1_IP=$(get_peer_ip "peer1" "$network")
  PEER2_IP=$(get_peer_ip "peer2" "$network")
  PEER3_IP=$(get_peer_ip "peer3" "$network")
  
  if [ -n "$PEER1_IP" ] && [ -n "$PEER2_IP" ] && [ -n "$PEER3_IP" ]; then
    ensure_dns_resolution "$PEER1_IP" "$PEER2_IP" "$PEER3_IP"
  else
    echo "Warning: Could not get IP addresses for all peers during reset"
  fi
  
  # Add hosts entries directly to all containers for more reliable resolution
  update_hosts_direct "$PEER1_IP" "$PEER2_IP" "$PEER3_IP"
  
  # Verify bidirectional connectivity
  echo "Verifying bidirectional connectivity after reset..."
  for src in {1..3}; do
    for dst in {1..3}; do
      if [ $src -ne $dst ]; then
        resp=$(test_peer_connection "peer$src" "peer$dst")
        echo "peer$src → peer$dst: $resp"
      fi
    done
  done
}

# Direct hosts file update without using DNS (more reliable)
update_hosts_direct() {
  local peer1_ip=$1
  local peer2_ip=$2
  local peer3_ip=$3
  
  if [ -z "$peer1_ip" ] || [ -z "$peer2_ip" ] || [ -z "$peer3_ip" ]; then
    echo "Warning: Missing IP addresses for direct hosts update"
    return 1
  fi
  
  # Update hosts file in each container directly
  for peer_num in {1..3}; do
    echo "Updating hosts file in peer${peer_num}..."
    
    # Create a hosts file entry for each peer
    local hosts_entries="127.0.0.1 localhost\n"
    hosts_entries+="${peer1_ip} peer1\n"
    hosts_entries+="${peer2_ip} peer2\n"
    hosts_entries+="${peer3_ip} peer3\n"
    
    # Write to container's hosts file (replace it completely for consistency)
    docker exec "${PROJECT_NAME}-peer${peer_num}-1" bash -c "echo -e '${hosts_entries}' > /etc/hosts"
    
    # Verify the hosts file was updated
    docker exec "${PROJECT_NAME}-peer${peer_num}-1" cat /etc/hosts
  done
}

# Print network connectivity status for debugging
print_network_status() {
  echo "--- Network Connectivity Status ---"
  for src in {1..3}; do
    for dst in {1..3}; do
      if [ $src -ne $dst ]; then
        resp=$(test_peer_connection "peer$src" "peer$dst")
        echo "peer$src → peer$dst: $resp"
        if [ "$resp" != "200" ]; then
          echo "Connection from peer$src to peer$dst failing"
        fi
      fi
    done
  done
  echo "--------------------------------"
}

# Function to check network connectivity between peers
check_network_connectivity() {
  echo "Checking network connectivity between peers..."
  local PROJECT_NAME=${DOCKER_COMPOSE_PROJECT_NAME:-shard_test}
  
  print_network_status
  
  # Also check if containers are healthy
  for peer in {1..3}; do
    local health=$(docker inspect --format '{{.State.Health.Status}}' "${PROJECT_NAME}-peer${peer}-1" 2>/dev/null)
    echo "peer${peer} health status: $health"
  done
  echo "--------------------------------"
}

cleanup_and_ensure_bidirectional_connectivity() {
  local test_folder=$1
  local NETWORK_NAME="${PROJECT_NAME}_default"
  
  echo "=== Cleaning up after test ==="
  
  # Fix one-way connectivity issues by refreshing all peer network connections
  echo "Refreshing network connections to fix one-way connectivity issues..."
  for peer in peer1 peer2 peer3; do
    if inspect_network "$NETWORK_NAME" | grep -q "${PROJECT_NAME}-${peer}-1"; then
      echo "Refreshing network connection for $peer..."
      # Disconnect and reconnect to refresh network settings
      disconnect_from_network "$peer" "$NETWORK_NAME" || echo "Could not disconnect $peer"
      sleep 1
      connect_to_network "$peer" "$NETWORK_NAME" || echo "Could not reconnect $peer"
    fi
  done
  
  # Restart all containers to ensure clean state
  echo "Restarting all peers to ensure clean state..."
  for peer in peer1 peer2 peer3; do
    restart_peer "$peer"
  done
  
  # Wait for all peers to become healthy
  echo "Waiting for all peers to become healthy..."
  for peer in peer1 peer2 peer3; do
    if ! wait_for_peer_health "$peer" 30; then
      echo "Warning: $peer did not become healthy during cleanup"
    else
      echo "$peer is healthy after restart"
    fi
  done
  
  # Get IP addresses after restart
  PEER1_IP=$(get_peer_ip "peer1" "$NETWORK_NAME")
  PEER2_IP=$(get_peer_ip "peer2" "$NETWORK_NAME")
  PEER3_IP=$(get_peer_ip "peer3" "$NETWORK_NAME")
  
  # Update DNS resolution to ensure all peers can find each other
  if [ -n "$PEER1_IP" ] && [ -n "$PEER2_IP" ] && [ -n "$PEER3_IP" ]; then
    ensure_dns_resolution "$PEER1_IP" "$PEER2_IP" "$PEER3_IP"
  fi
  
  # Verify bidirectional connectivity after cleanup
  print_network_status
  
  # Remove test data
  echo "Removing test folder..."
  rm -rf ./$test_folder
}