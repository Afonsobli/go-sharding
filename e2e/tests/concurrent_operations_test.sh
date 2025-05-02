#!/bin/bash
set -e

echo "===== STARTING CONCURRENT OPERATIONS TEST: $(date) ====="
echo "Running from container: $(hostname)"

# We need a more reliable way to determine if we're in the test container
# First, let's get more information about our environment
HOSTNAME=$(hostname)
echo "Debug - Current hostname: $HOSTNAME"

# Create test data directory
TEST_FOLDER="concurrent_operations_test"
mkdir -p ./$TEST_FOLDER

echo "Creating test files..."
# Create a mix of small and large files
dd if=/dev/urandom of=./$TEST_FOLDER/small1.txt bs=100k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/small2.txt bs=300k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/small3.txt bs=500k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/medium1.txt bs=2M count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/medium2.txt bs=5M count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/large1.txt bs=10M count=1 2>/dev/null

# Calculate hashes before upload
for file in ./$TEST_FOLDER/*.txt; do
  BASENAME=$(basename "$file")
  sha256sum "$file" > ./$TEST_FOLDER/${BASENAME}.hash
done

#Test 1: Measure time for sequential uploads
echo "=== Test 1: Sequential Uploads ==="
echo "Starting sequential uploads..."

echo "Sequential upload time:"
time (
  for file in ./$TEST_FOLDER/*.txt; do
    BASENAME=$(basename "$file")
    echo "Uploading $BASENAME sequentially..."
    curl -s -F "file=@$file" http://peer1:8080/upload > ./$TEST_FOLDER/${BASENAME}.seq_response
  done
)

# Test 2: Measure time for concurrent uploads
echo "=== Test 2: Concurrent Uploads ==="
curl -s "http://peer1:8080/shardMap"
echo "Starting concurrent uploads..."

echo "Concurrent upload time:"
time (
  # Upload all files concurrently
  for file in ./$TEST_FOLDER/*.txt; do
    BASENAME=$(basename "$file")
    echo "Starting upload of $BASENAME concurrently..."
    curl -s -F "file=@$file" http://peer1:8080/upload > ./$TEST_FOLDER/${BASENAME}.conc_response &
  done
  # Wait for all uploads to complete
  wait
)

# Note: We can't calculate the exact speedup with this approach as we don't capture the values
# But the output will still show the times clearly for manual comparison
echo "Speedup from concurrency: Compare the real times above"

# Test 3: Concurrent Downloads - simultaneously download from different peers
echo "=== Test 3: Concurrent Downloads ==="
mkdir -p ./$TEST_FOLDER/downloads

# Get list of all file hashes
HASHES=()
for file in ./$TEST_FOLDER/*.txt; do
  # Skip any non-regular files or files generated during the test process
  if [[ "$file" == *"time.txt"* ]] || [[ ! -f "$file" ]]; then
    continue
  fi
  
  BASENAME=$(basename "$file")
  HASH=$(cat ./$TEST_FOLDER/${BASENAME}.hash | awk '{print $1}')
  HASHES+=("$HASH")
done

echo "Concurrent download time:"
time (
  # Start concurrent downloads from different peers
  for hash in "${HASHES[@]}"; do
    # Download some files from peer2, some from peer3 to test load distribution
    if [ $(($RANDOM % 2)) -eq 0 ]; then
      echo "Downloading $hash from peer2..."
      curl -s "http://peer2:8080/file?hash=$hash" -o ./$TEST_FOLDER/downloads/${hash}_from_peer2 &
    else
      echo "Downloading $hash from peer3..."
      curl -s "http://peer3:8080/file?hash=$hash" -o ./$TEST_FOLDER/downloads/${hash}_from_peer3 &
    fi
  done
  # Wait for all downloads to complete
  wait
)

# Test 4: Concurrent Mixed Operations
echo "=== Test 4: Concurrent Mixed Operations ==="

# Create some additional files for this test
dd if=/dev/urandom of=./$TEST_FOLDER/mixed1.txt bs=100k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/mixed2.txt bs=200k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/mixed3.txt bs=300k count=1 2>/dev/null

# Calculate hashes
for i in {1..3}; do
  sha256sum ./$TEST_FOLDER/mixed$i.txt > ./$TEST_FOLDER/mixed${i}.hash
done

echo "Concurrent mixed operations time:"
time (
  # Start a mix of uploads and downloads concurrently
  # Upload new files
  for i in {1..3}; do
    echo "Uploading mixed$i.txt..."
    curl -s -F "file=@./$TEST_FOLDER/mixed$i.txt" http://peer1:8080/upload > /dev/null &
  done

  # Download previous files
  for hash in "${HASHES[@]:0:3}"; do  # Just use first 3 hashes
    echo "Downloading $hash during mixed operations..."
    curl -s "http://peer2:8080/file?hash=$hash" -o /dev/null &
  done

  # Wait for all operations to complete
  wait
)

# Verify data integrity
echo "=== Verifying Data Integrity ==="
FAILURES=0

# Check all original files
echo "Verifying original files..."
for file in ./$TEST_FOLDER/*.txt; do
  if [[ "$file" == *"mixed"* ]]; then
    continue  # Skip mixed files they are processed separately
  fi
  
  BASENAME=$(basename "$file")
  HASH=$(cat ./$TEST_FOLDER/${BASENAME}.hash | awk '{print $1}')
  
  # Try to fetch from both peers
  echo "Verifying $BASENAME (hash: $HASH)..."
  
  curl -s "http://peer2:8080/file?hash=$HASH" -o ./$TEST_FOLDER/verify_peer2_${BASENAME}
  curl -s "http://peer3:8080/file?hash=$HASH" -o ./$TEST_FOLDER/verify_peer3_${BASENAME}
  
  # Verify integrity
  if ! diff "$file" ./$TEST_FOLDER/verify_peer2_${BASENAME} > /dev/null; then
    echo "❌ File $BASENAME from peer2 doesn't match original"
    FAILURES=$((FAILURES+1))
  else
    echo "✅ File $BASENAME verified on peer2"
  fi
  
  if ! diff "$file" ./$TEST_FOLDER/verify_peer3_${BASENAME} > /dev/null; then
    echo "❌ File $BASENAME from peer3 doesn't match original"
    FAILURES=$((FAILURES+1))
  else
    echo "✅ File $BASENAME verified on peer3"
  fi
done

# Now verify the mixed files
echo "Verifying mixed files..."
for i in {1..3}; do
  MIXFILE="./$TEST_FOLDER/mixed$i.txt"
  # Check if the hash file exists before trying to read it
  if [[ -f "./$TEST_FOLDER/mixed${i}.hash" ]]; then
    HASH=$(cat ./$TEST_FOLDER/mixed${i}.hash | awk '{print $1}')
    
    echo "Verifying mixed$i.txt (hash: $HASH)..."
    
    # Try to fetch from both peers
    curl -s "http://peer2:8080/file?hash=$HASH" -o ./$TEST_FOLDER/verify_peer2_mixed${i}.txt
    curl -s "http://peer3:8080/file?hash=$HASH" -o ./$TEST_FOLDER/verify_peer3_mixed${i}.txt
    
    # Verify integrity
    if ! diff "$MIXFILE" ./$TEST_FOLDER/verify_peer2_mixed${i}.txt > /dev/null; then
      echo "❌ File mixed$i.txt from peer2 doesn't match original"
      FAILURES=$((FAILURES+1))
    else
      echo "✅ File mixed$i.txt verified on peer2"
    fi
    
    if ! diff "$MIXFILE" ./$TEST_FOLDER/verify_peer3_mixed${i}.txt > /dev/null; then
      echo "❌ File mixed$i.txt from peer3 doesn't match original"
      FAILURES=$((FAILURES+1))
    else
      echo "✅ File mixed$i.txt verified on peer3"
    fi
  else
    echo "⚠️ Warning: Hash file for mixed$i.txt not found, skipping verification"
    FAILURES=$((FAILURES+1))
  fi
done

# Summary
echo "=== Test Results ==="
echo "See the timing results above for each test."
echo "Note: Compare 'real' times for performance measurement."

# Cleanup
if [ $FAILURES -eq 0 ]; then
  echo "✅ All concurrency tests passed! ($((6+3)) files verified on 2 peers)"
  rm -rf ./$TEST_FOLDER
  exit 0
else
  echo "❌ Test failed with $FAILURES integrity errors"
  rm -rf ./$TEST_FOLDER
  exit 1
fi
