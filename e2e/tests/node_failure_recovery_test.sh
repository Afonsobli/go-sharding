#!/bin/bash
set -e

# Source the container control helper functions
source /app/e2e/container_control.sh

echo "===== STARTING NODE FAILURE RECOVERY TEST: $(date) ====="
echo "Running from container: $(hostname)"

# Create test data directory
TEST_FOLDER="node_failure_recovery_test"
mkdir -p ./$TEST_FOLDER

echo "Creating test files..."
# Create test files of different sizes
dd if=/dev/urandom of=./$TEST_FOLDER/file1.txt bs=500k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file2.txt bs=1M count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file3.txt bs=2M count=1 2>/dev/null

# Calculate hashes for verification
echo "Calculating file hashes..."
for file in ./$TEST_FOLDER/*.txt; do
  BASENAME=$(basename "$file")
  sha256sum "$file" > ./$TEST_FOLDER/${BASENAME}.hash
done

# Store hashes in an array
HASHES=()
for hash_file in ./$TEST_FOLDER/*.hash; do
  HASH=$(cat "$hash_file" | awk '{print $1}')
  HASHES+=("$HASH")
  echo "Added hash: $HASH"
done

echo "=== Phase 1: Initial Upload ==="

# Upload all files to the cluster via peer1
echo "Uploading files to peer1..."
for file in ./$TEST_FOLDER/*.txt; do
  BASENAME=$(basename "$file")
  echo "Uploading $BASENAME..."
  curl -s -F "file=@$file" http://peer1:8080/upload > ./$TEST_FOLDER/${BASENAME}_upload.txt
  echo "Upload response for $BASENAME: $(cat ./$TEST_FOLDER/${BASENAME}_upload.txt)"
done

# Give time for replication to complete across all peers
echo "Waiting for initial replication..."
sleep 1

# Verify files are accessible from all peers
echo "Verifying initial accessibility..."
FAILURES=0
for hash in "${HASHES[@]}"; do
  echo "Checking hash $hash on all peers..."
  for peer in {1..3}; do
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$hash")
    if [ "$RESPONSE" -eq 200 ]; then
      echo "✅ File with hash $hash is accessible from peer${peer}"
    else
      echo "❌ File with hash $hash is NOT accessible from peer${peer}"
      FAILURES=$((FAILURES+1))
    fi
  done
done

if [ $FAILURES -ne 0 ]; then
  echo "❌ Initial replication test failed with $FAILURES errors"
  rm -rf ./$TEST_FOLDER
  exit 1
fi

echo "=== Phase 2: Simulating Node Failure ==="

# Simulate node failure by stopping peer2 using helper function
echo "Stopping peer2 to simulate node failure..."
stop_peer peer2

# Verify peer2 is actually down
if [ "$(is_peer_running peer2)" = "false" ]; then
  echo "✅ Peer2 is successfully stopped"
else
  echo "❌ Failed to stop peer2"
  FAILURES=$((FAILURES+1))
fi

# Give time for the system to recognize the node is down
echo "Waiting for system to recognize the node failure..."
sleep 5

echo "=== Phase 3: Operations During Node Failure ==="

# Try to download files while a node is down - should work from other peers
echo "Attempting to download files while peer2 is down..."

for hash in "${HASHES[@]}"; do
  echo "Downloading hash $hash from peer1..."
  curl -s "http://peer1:8080/file?hash=$hash" -o ./$TEST_FOLDER/${hash}_from_peer1_during_failure.dat
  
  echo "Downloading hash $hash from peer3..."
  curl -s "http://peer3:8080/file?hash=$hash" -o ./$TEST_FOLDER/${hash}_from_peer3_during_failure.dat
  
  echo "Verifying peer2 is down (using hash: $hash)..."
  
  # More robust check for peer2 status - add retry logic and proper error handling
  PEER2_STATUS="unknown"
  MAX_RETRIES=3
  RETRY_COUNT=0
  
  while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Use a separate variable to prevent issues if the command fails
    TEMP_RESPONSE=""
    
    # Redirect stderr to /dev/null to suppress connection error messages
    TEMP_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 "http://peer2:8080/file?hash=$hash" 2>/dev/null || echo "connection_failed")
    
    echo "Attempt $(($RETRY_COUNT + 1))/$MAX_RETRIES - Response from peer2: $TEMP_RESPONSE"
    
    # Check if we got a non-200 response (as expected when peer is down)
    if [[ "$TEMP_RESPONSE" == "connection_failed" || "$TEMP_RESPONSE" != "200" ]]; then
      PEER2_STATUS="down"
      break
    fi
    
    # If we got 200, try again after a short delay
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep 1
  done
  
  # Evaluate the final status
  if [[ "$PEER2_STATUS" == "down" ]]; then
    echo "✅ As expected, cannot access peer2 (down), final response: $TEMP_RESPONSE"
  else
    echo "❌ Unexpected: Could still access peer2 even though it should be down"
    FAILURES=$((FAILURES+1))
  fi
done

# Upload a new file during the node failure
echo "Uploading new file during node failure..."
dd if=/dev/urandom of=./$TEST_FOLDER/during_failure.txt bs=1M count=1 2>/dev/null
sha256sum ./$TEST_FOLDER/during_failure.txt > ./$TEST_FOLDER/during_failure.txt.hash
FAILURE_HASH=$(cat ./$TEST_FOLDER/during_failure.txt.hash | awk '{print $1}')
HASHES+=("$FAILURE_HASH")

echo "Uploading file to peer1 during failure state..."
curl -s -F "file=@./$TEST_FOLDER/during_failure.txt" http://peer1:8080/upload > ./$TEST_FOLDER/during_failure_upload.txt
echo "Upload response: $(cat ./$TEST_FOLDER/during_failure_upload.txt)"

# Verify the file is available on peer1 and peer3
for peer in 1 3; do
  RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$FAILURE_HASH")
  echo "Response from peer${peer}: $RESPONSE"
  
  # Use explicit string comparison
  if [[ "$RESPONSE" == "200" ]]; then
    echo "✅ File uploaded during failure is accessible from peer${peer}"
  else
    echo "❌ File uploaded during failure is NOT accessible from peer${peer}"
    FAILURES=$((FAILURES+1))
  fi
done

echo "=== Phase 4: Node Recovery ==="

# Restart the failed node using helper function
echo "Restarting peer2..."
start_peer peer2

# Wait for the peer to become healthy
echo "Waiting for peer2 to recover and sync..."
if ! wait_for_peer_health peer2 30; then
  echo "❌ Peer2 failed to become healthy within timeout"
  FAILURES=$((FAILURES+1))
else
  echo "✅ Peer2 is healthy after restart"
fi

echo "=== Phase 5: Verifying Recovery ==="

# Check if peer2 is back online
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://peer2:8080/health")
echo "Health check response code: $RESPONSE"

# Use double brackets for safer string comparison
if [[ "$RESPONSE" == "200" ]]; then
  echo "✅ Peer2 is back online"
else
  echo "❌ Peer2 failed to recover, response code: $RESPONSE"
  FAILURES=$((FAILURES+1))
fi

# Verify all files are accessible from all peers after recovery
echo "Verifying all files are accessible after recovery..."

for hash in "${HASHES[@]}"; do
  echo "Checking hash $hash on all peers after recovery..."
  for peer in {1..3}; do
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$hash")
    echo "Response from peer${peer} for hash $hash: $RESPONSE"
    
    # Use double brackets for safer string comparison
    if [[ "$RESPONSE" == "200" ]]; then
      echo "✅ File with hash $hash is accessible from peer${peer} after recovery"
      
      # Download for integrity check
      curl -s "http://peer${peer}:8080/file?hash=$hash" -o ./$TEST_FOLDER/${hash}_from_peer${peer}_after_recovery.dat
    else
      echo "❌ File with hash $hash is NOT accessible from peer${peer} after recovery, response code: $RESPONSE"
      FAILURES=$((FAILURES+1))
    fi
  done
done

# Verify data integrity
echo "=== Phase 6: Verifying Data Integrity ==="

for hash in "${HASHES[@]}"; do
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
    RECOVERED_FILE="./$TEST_FOLDER/${hash}_from_peer${peer}_after_recovery.dat"
    if [ -f "$RECOVERED_FILE" ]; then
      if ! diff "$ORIGINAL_FILE" "$RECOVERED_FILE" > /dev/null; then
        echo "❌ File from peer${peer} with hash $hash doesn't match original after recovery"
        FAILURES=$((FAILURES+1))
      else
        echo "✅ File integrity verified for hash $hash from peer${peer} after recovery"
      fi
    else
      echo "❌ Recovery file for hash $hash from peer${peer} not found"
      FAILURES=$((FAILURES+1))
    fi
  done
done

# Upload another new file after recovery to test full system functionality
echo "=== Phase 7: Testing Post-Recovery Operations ==="

echo "Uploading new file after recovery..."
dd if=/dev/urandom of=./$TEST_FOLDER/post_recovery.txt bs=1M count=1 2>/dev/null
sha256sum ./$TEST_FOLDER/post_recovery.txt > ./$TEST_FOLDER/post_recovery.txt.hash
POST_RECOVERY_HASH=$(cat ./$TEST_FOLDER/post_recovery.txt.hash | awk '{print $1}')

echo "Uploading file to peer2 (previously failed node)..."
curl -s -F "file=@./$TEST_FOLDER/post_recovery.txt" http://peer2:8080/upload > ./$TEST_FOLDER/post_recovery_upload.txt
echo "Upload response: $(cat ./$TEST_FOLDER/post_recovery_upload.txt)"

# Verify the file is replicated to all peers
sleep 5
for peer in {1..3}; do
  RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://peer${peer}:8080/file?hash=$POST_RECOVERY_HASH")
  echo "Post-recovery check from peer${peer}: $RESPONSE"
  
  # Use double brackets for safer string comparison
  if [[ "$RESPONSE" == "200" ]]; then
    echo "✅ Post-recovery file is accessible from peer${peer}"
  else
    echo "❌ Post-recovery file is NOT accessible from peer${peer}, response code: $RESPONSE"
    FAILURES=$((FAILURES+1))
  fi
done

# Summary
echo "=== Test Results ==="
echo "Total files tested: $((${#HASHES[@]} + 1))" # +1 for post-recovery file

if [ $FAILURES -eq 0 ]; then
  echo "✅ Node failure recovery test passed! System successfully handled node failure and recovered."
  rm -rf ./$TEST_FOLDER
  exit 0
else
  echo "❌ Test failed with $FAILURES errors"
  rm -rf ./$TEST_FOLDER
  exit 1
fi
