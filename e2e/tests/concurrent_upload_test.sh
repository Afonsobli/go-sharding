#!/bin/bash
set -e

# Create test data directory
TEST_FOLDER="concurrent_uploads_test"
mkdir -p ./$TEST_FOLDER

echo "Waiting for peer services to start..."
sleep 3

echo "Creating test files..."
# Create 5 files of different sizes
dd if=/dev/urandom of=./$TEST_FOLDER/file1.txt bs=512k count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file2.txt bs=1M count=2 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file3.txt bs=2M count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file4.txt bs=256k count=3 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file5.txt bs=3M count=1 2>/dev/null
dd if=/dev/urandom of=./$TEST_FOLDER/file6.txt bs=1k count=1 2>/dev/null

# Calculate hashes before upload
for i in {1..6}; do
  sha256sum ./$TEST_FOLDER/file$i.txt > ./$TEST_FOLDER/file${i}_hash.txt
done

# Upload files concurrently (in background)
echo "Uploading files concurrently..."
for i in {1..6}; do
  curl -s -F "file=@./$TEST_FOLDER/file$i.txt" http://peer1:8080/upload > ./$TEST_FOLDER/upload${i}_response.txt &
done

# Wait for all uploads to complete
wait
echo "All uploads completed"

# Extract file hashes from responses or calculate them
for i in {1..6}; do
  FILE_HASH=$(cat ./$TEST_FOLDER/file${i}_hash.txt | awk '{print $1}')
  echo "File $i hash: $FILE_HASH"
  echo $FILE_HASH > ./$TEST_FOLDER/hash$i.txt
done

# Wait for files to propagate
echo "Waiting for files to propagate..."
sleep 10

# Verify all files can be retrieved from peer2 and peer3
FAILURES=0
for i in {1..6}; do
  FILE_HASH=$(cat ./$TEST_FOLDER/hash$i.txt)
  
  # Try to fetch from peer2
  echo "Fetching file $i from peer2..."
  curl -s "http://peer2:8080/file?hash=$FILE_HASH" -o ./$TEST_FOLDER/file${i}_peer2.txt
  
  # Try to fetch from peer3
  echo "Fetching file $i from peer3..."
  curl -s "http://peer3:8080/file?hash=$FILE_HASH" -o ./$TEST_FOLDER/file${i}_peer3.txt
  
  # Verify file contents match
  if ! diff ./$TEST_FOLDER/file$i.txt ./$TEST_FOLDER/file${i}_peer2.txt > /dev/null; then
    echo "❌ File $i from peer2 doesn't match original"
    FAILURES=$((FAILURES+1))
  fi
  
  if ! diff ./$TEST_FOLDER/file$i.txt ./$TEST_FOLDER/file${i}_peer3.txt > /dev/null; then
    echo "❌ File $i from peer3 doesn't match original"
    FAILURES=$((FAILURES+1))
  fi
done

# Cleanup
if [ $FAILURES -eq 0 ]; then
  echo "✅ All concurrent uploads and retrievals successful!"
  rm -rf ./$TEST_FOLDER
  exit 0
else
  echo "❌ Test failed with $FAILURES errors"
  rm -rf ./$TEST_FOLDER
  exit 1
fi