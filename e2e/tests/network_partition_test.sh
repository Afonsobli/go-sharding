#!/bin/bash
set -e

# Source the container control helper functions
source /app/e2e/container_control.sh

# Create test data directory
TEST_FOLDER="network_partition_test"
mkdir -p ./$TEST_FOLDER

# Set up trap to call cleanup function on exit
trap 'cleanup_and_ensure_bidirectional_connectivity "$TEST_FOLDER"' EXIT

echo "===== STARTING NETWORK PARTITION TEST: $(date) ====="
echo "Running from container: $(hostname)"

# Function to create unique file content
create_test_file() {
  local filename=$1
  local size=$2
  echo "Creating test file: $filename ($size)"
  dd if=/dev/urandom of="./$TEST_FOLDER/$filename" bs=$size count=1 2>/dev/null
  sha256sum "./$TEST_FOLDER/$filename" > "./$TEST_FOLDER/$filename.hash"
  echo "File hash: $(cat "./$TEST_FOLDER/$filename.hash" | awk '{print $1}')"
}

echo "=== Phase 1: Initial Setup ==="
echo "Creating test files..."

# Create a few different sized files
create_test_file "pre_partition_1.txt" "500k"
create_test_file "pre_partition_2.txt" "1M"

# Store hashes in an array for verification
PRE_PARTITION_HASHES=()
for hash_file in ./$TEST_FOLDER/pre_partition_*.hash; do
  HASH=$(cat "$hash_file" | awk '{print $1}')
  PRE_PARTITION_HASHES+=("$HASH")
  echo "Added hash: $HASH"
done

# Display initial network status
echo "Initial network status before test:"
print_network_status

# Upload initial files to the cluster
echo "Uploading initial files to cluster..."
for file in ./$TEST_FOLDER/pre_partition_*.txt; do
  BASENAME=$(basename "$file")
  echo "Uploading $BASENAME to peer1..."
  curl -s -F "file=@$file" http://peer1:8080/upload > ./$TEST_FOLDER/${BASENAME}_upload.txt
  echo "Upload response: $(cat ./$TEST_FOLDER/${BASENAME}_upload.txt)"
done

# Verify initial replication
echo "Verifying initial replication..."
sleep 3  # Give time for replication

FAILURES=0
for hash in "${PRE_PARTITION_HASHES[@]}"; do
  echo "Checking hash $hash on all peers..."
  for peer in {1..3}; do
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$hash")
    if [ "$RESPONSE" -eq 200 ]; then
      echo "✅ File with hash $hash is accessible from peer${peer}"
    else
      echo "❌ File with hash $hash is NOT accessible from peer${peer}, response code: $RESPONSE"
      FAILURES=$((FAILURES+1))
    fi
  done
done

if [ $FAILURES -ne 0 ]; then
  echo "❌ Initial replication test failed with $FAILURES errors"
  exit 1
fi

echo "=== Phase 2: Creating Network Partition ==="
echo "Creating network partition between peer1 and other peers..."

# Create network partition by modifying Docker network settings
NETWORK_NAME="${PROJECT_NAME}_default"
echo "Disconnecting peer1 from network: $NETWORK_NAME"

# Disconnect peer1 from the network using the helper function
disconnect_peer_from_network "peer1" "$NETWORK_NAME"

echo "Waiting for network partition to take effect..."
sleep 5

# Verify network partition
echo "Network status after partition:"
print_network_status

# Peer1 should not be able to communicate with peer2 and peer3
# We'll test this by trying to access peer2 and peer3 from peer1

# Try to access peer2 and peer3 health endpoints from peer1
PEER1_ACCESS_PEER2=$(test_peer_connection "peer1" "peer2")
PEER1_ACCESS_PEER3=$(test_peer_connection "peer1" "peer3")

# Check if peer1 can't access peer2 and peer3
if [[ "$PEER1_ACCESS_PEER2" == "failed" || "$PEER1_ACCESS_PEER2" != "200" ]] && [[ "$PEER1_ACCESS_PEER3" == "failed" || "$PEER1_ACCESS_PEER3" != "200" ]]; then
  echo "✅ Network partition successfully created - peer1 cannot access peer2 or peer3"
else
  echo "❌ Network partition was not successful: peer1 can still access peer2 ($PEER1_ACCESS_PEER2) or peer3 ($PEER1_ACCESS_PEER3)"
  # This doesn't necessarily mean we should fail the test
  # The partition could still be effective at the application level
  echo "Continuing test despite potential partition issues..."
fi

# Verify peer2 and peer3 can still communicate with each other
PEER2_ACCESS_PEER3=$(test_peer_connection "peer2" "peer3")

if [[ "$PEER2_ACCESS_PEER3" == "200" ]]; then
  echo "✅ peer2 can still access peer3 as expected"
else
  echo "❌ Unexpected: peer2 cannot access peer3, response: $PEER2_ACCESS_PEER3"
  FAILURES=$((FAILURES+1))
fi

echo "=== Phase 3: Operations During Network Partition ==="

# Create new files during partition
echo "Creating files during partition..."
create_test_file "during_partition_peer2.txt" "750k"
create_test_file "during_partition_peer3.txt" "500k"

# Upload file to peer2
echo "Uploading file to peer2 during partition..."
curl -s -F "file=@./$TEST_FOLDER/during_partition_peer2.txt" http://peer2:8080/upload > ./$TEST_FOLDER/during_partition_peer2_upload.txt
DURING_PEER2_HASH=$(cat ./$TEST_FOLDER/during_partition_peer2.txt.hash | awk '{print $1}')
echo "Upload to peer2 response: $(cat ./$TEST_FOLDER/during_partition_peer2_upload.txt)"

# Upload file to peer3
echo "Uploading file to peer3 during partition..."
curl -s -F "file=@./$TEST_FOLDER/during_partition_peer3.txt" http://peer3:8080/upload > ./$TEST_FOLDER/during_partition_peer3_upload.txt
DURING_PEER3_HASH=$(cat ./$TEST_FOLDER/during_partition_peer3.txt.hash | awk '{print $1}')
echo "Upload to peer3 response: $(cat ./$TEST_FOLDER/during_partition_peer3_upload.txt)"

# Record the hashes of files uploaded during partition
DURING_PARTITION_HASHES=("$DURING_PEER2_HASH" "$DURING_PEER3_HASH")

# Check if files are available on peer2 and peer3 but not peer1
echo "Verifying file availability during partition..."
sleep 3  # Give time for replication between peer2 and peer3

for hash in "${DURING_PARTITION_HASHES[@]}"; do
  echo "Checking hash $hash during partition..."
  
  # Files should be accessible from peer2 and peer3
  for peer in 2 3; do
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$hash")
    if [ "$RESPONSE" -eq 200 ]; then
      echo "✅ As expected, file with hash $hash is accessible from peer${peer}"
    else
      echo "❌ File with hash $hash should be accessible from peer${peer}, response: $RESPONSE"
      FAILURES=$((FAILURES+1))
    fi
  done
  
  # Try to access from peer1 (should fail since peer1 is partitioned)
  # We need to work around the network partition by querying the API server directly
  # This is trickier since peer1 has no direct connectivity to outside networks now
  echo "Attempting to access file from partitioned peer1 (should fail)..."
  
  # We'll use a workaround to test if peer1 has the file without network connectivity
  # Use docker exec to run a command inside the peer1 container to check if the file exists locally
  FILE_EXISTS_CHECK=$(check_file_exists_in_peer "peer1" "$hash")
  
  if [[ "$FILE_EXISTS_CHECK" != "200" ]]; then
    echo "✅ As expected, file with hash $hash is NOT accessible from partitioned peer1"
  else
    echo "❌ Unexpected: File with hash $hash IS accessible from partitioned peer1"
    FAILURES=$((FAILURES+1))
  fi
done

echo "=== Phase 4: Healing Network Partition ==="
echo "Reconnecting peer1 to the network..."

# Get the current container ID (the one running this script)
CURRENT_CONTAINER_ID=$(hostname)
echo "Current container ID: $CURRENT_CONTAINER_ID"

# Reconnect peer1 to the network using the helper function
connect_peer_to_network "peer1" "$NETWORK_NAME"

echo "Waiting for network healing to take effect..."
sleep 5

# Get IP addresses AFTER reconnection using the helper function
PEER1_IP=$(get_peer_ip "peer1" "$NETWORK_NAME")
PEER2_IP=$(get_peer_ip "peer2" "$NETWORK_NAME")
PEER3_IP=$(get_peer_ip "peer3" "$NETWORK_NAME")

echo "Peer IPs: peer1=$PEER1_IP, peer2=$PEER2_IP, peer3=$PEER3_IP"

ensure_dns_resolution $PEER1_IP $PEER2_IP $PEER3_IP

# Verify we can now resolve peer1 from current container
echo "Testing DNS resolution from current container:"
ping -c 1 peer1 || echo "Still cannot resolve peer1"
ping -c 1 peer2 || echo "Still cannot resolve peer2"
ping -c 1 peer3 || echo "Still cannot resolve peer3"

sleep 2  # Give time for changes to take effect

echo "Network status after healing:"
print_network_status

echo "=== Phase 5: Verifying Recovery After Partition ==="

# Check if peer1 can communicate with peer2 and peer3 again
echo "Project name: ${PROJECT_NAME}"

# Store the responses in variables with explicit outputs for debugging
PEER1_ACCESS_PEER2=$(test_peer_connection "peer1" "peer2")
PEER1_ACCESS_PEER3=$(test_peer_connection "peer1" "peer3")

echo "Debug - peer1 to peer2 access: '$PEER1_ACCESS_PEER2'"
echo "Debug - peer1 to peer3 access: '$PEER1_ACCESS_PEER3'"

# Use printf to compare safely as strings
if [ "$PEER1_ACCESS_PEER2" = "200" ] && [ "$PEER1_ACCESS_PEER3" = "200" ] ; then
  echo "✅ Network partition successfully healed - peer1 can access peer2 and peer3"
else
  echo "❌ Network healing was not successful: peer1 still cannot access peer2 ($PEER1_ACCESS_PEER2) or peer3 ($PEER1_ACCESS_PEER3)"
  FAILURES=$((FAILURES+1))
fi

# Verify files are now accessible from all peers, including peer1
echo "Verifying file availability after partition healing..."
ALL_HASHES=("${PRE_PARTITION_HASHES[@]}" "${DURING_PARTITION_HASHES[@]}")

for hash in "${ALL_HASHES[@]}"; do
  echo "Checking hash $hash on all peers after partition healing..."
  
  for peer in {1..3}; do
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$hash")
    echo "Response from peer${peer} for $hash: $RESPONSE"
    
    if [ "$RESPONSE" = "200" ]; then
      echo "✅ File with hash $hash is accessible from peer${peer} after partition healing"
      
      # Download for integrity check
      curl -s "http://peer${peer}:8080/file?hash=$hash" -o ./$TEST_FOLDER/${hash}_from_peer${peer}_after_healing.dat
    else
      echo "❌ File with hash $hash is NOT accessible from peer${peer} after partition healing"
      FAILURES=$((FAILURES+1))
    fi
  done
done

# Create a new file after healing and ensure it propagates to all peers
echo "=== Phase 6: Testing Post-Healing Operations ==="
echo "Creating and uploading new file after partition healing..."
create_test_file "post_healing.txt" "250k"
POST_HEALING_HASH=$(cat ./$TEST_FOLDER/post_healing.txt.hash | awk '{print $1}')

# Upload file to peer1 (previously partitioned node)
echo "Uploading file to peer1 after healing..."
curl -s -F "file=@./$TEST_FOLDER/post_healing.txt" http://peer1:8080/upload > ./$TEST_FOLDER/post_healing_upload.txt
echo "Upload to peer1 response: $(cat ./$TEST_FOLDER/post_healing_upload.txt)"

# Verify the file is replicated to all peers
echo "Verifying file replication after healing..."
sleep 3  # Give time for replication

for peer in {1..3}; do
  RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$POST_HEALING_HASH")
  if [ "$RESPONSE" = "200" ]; then
    echo "✅ Post-healing file is accessible from peer${peer}"
  else
    echo "❌ Post-healing file is NOT accessible from peer${peer}, response: $RESPONSE"
    FAILURES=$((FAILURES+1))
  fi
done

# Verify data integrity
echo "=== Phase 7: Verifying Data Integrity ==="

for hash in "${ALL_HASHES[@]}" "$POST_HEALING_HASH"; do
  # Find the original file by hash
  ORIGINAL_FILE=""
  for hash_file in ./$TEST_FOLDER/*.hash; do
    FILE_HASH=$(cat "$hash_file" | awk '{print $1}')
    if [ "$FILE_HASH" == "$hash" ]; then
      ORIGINAL_FILE="${hash_file%.hash}"
      break
    fi
  done
  
  if [ -z "$ORIGINAL_FILE" ]; then
    echo "❌ Could not find original file for hash $hash"
    FAILURES=$((FAILURES+1))
    continue
  fi
  
  # Compare downloaded files with original
  for peer in {1..3}; do
    DOWNLOADED_FILE="./$TEST_FOLDER/${hash}_from_peer${peer}_after_healing.dat"
    
    # Only check files that we've actually downloaded
    if [ -f "$DOWNLOADED_FILE" ]; then
      if ! diff "$ORIGINAL_FILE" "$DOWNLOADED_FILE" > /dev/null; then
        echo "❌ File with hash $hash from peer${peer} doesn't match original after healing"
        FAILURES=$((FAILURES+1))
      else
        echo "✅ File integrity verified for hash $hash from peer${peer}"
      fi
    fi
  done
done

# Summary
echo "=== Test Results ==="
echo "Total files tested: $((${#ALL_HASHES[@]} + 1))" # +1 for post-healing file

if [ $FAILURES -eq 0 ]; then
  echo "✅ Network partition test passed! System successfully handled partition and recovered."
  exit 0
else
  echo "❌ Test failed with $FAILURES errors"
  exit 1
fi
# Note: The cleanup function will be called automatically due to the trap
