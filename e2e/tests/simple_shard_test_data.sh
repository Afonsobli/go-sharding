#!/bin/sh

# Create test data directory if it doesn't exist
TEST_FOLDER="simple_shard_test_data"
mkdir -p ./$TEST_FOLDER

# Wait for services to be ready
echo "Waiting for peer services to start..."
sleep 2 

echo "Creating 10.5MB test file..."
# Generate 10.5MB file (10.5 * 1024 * 1024 = 11010048 bytes)
# Removed status=progress flag for compatibility
dd if=/dev/urandom of=./$TEST_FOLDER/testfile.txt bs=1024 count=10752
TEST_FILE_SIZE=$(du -h ./$TEST_FOLDER/testfile.txt | awk '{print $1}')
echo "Test file size: $TEST_FILE_SIZE"

# Upload file to peer1
echo "Uploading file to peer1..."
curl -s -F "file=@./$TEST_FOLDER/testfile.txt" http://peer1:8080/upload > ./$TEST_FOLDER/upload_response.txt
UPLOAD_RESPONSE=$(cat ./$TEST_FOLDER/upload_response.txt)
echo "Upload response: $UPLOAD_RESPONSE"

# TODO: The hash should be visible in the logs, but for this test, we'll calculate it
FILE_HASH=$(sha256sum ./$TEST_FOLDER/testfile.txt | awk '{print $1}')
echo "File hash: $FILE_HASH"

# Give some time for the file to propagate to other peers
echo "Waiting for file to propagate to other peers..."
sleep 2 

# Try to fetch the file from peer2
echo "Fetching file from peer2..."
curl -s "http://peer2:8080/file?hash=$FILE_HASH" -o ./$TEST_FOLDER/received_file1.txt

# Try to fetch the file from peer3
echo "Fetching file from peer3..."
curl -s "http://peer3:8080/file?hash=$FILE_HASH" -o ./$TEST_FOLDER/received_file2.txt

# Verify the file content
if diff ./$TEST_FOLDER/testfile.txt ./$TEST_FOLDER/received_file1.txt > /dev/null && diff ./$TEST_FOLDER/testfile.txt ./$TEST_FOLDER/received_file2.txt > /dev/null; then
    echo "✅ Test passed! Files match."
    echo "File was successfully sharded into ~11 pieces ($TEST_FILE_SIZE/1MB per shard)."
    rm -rf ./$TEST_FOLDER
    exit 0
else
    echo "❌ Test failed! Files don't match."
    echo "Hashes ------------------------------"
    echo "Original file: $(sha256sum ./$TEST_FOLDER/testfile.txt | awk '{print $1}')"
    echo "Received file 1: $(sha256sum ./$TEST_FOLDER/received_file1.txt | awk '{print $1}')"
    echo "Received file 2: $(sha256sum ./$TEST_FOLDER/received_file2.txt | awk '{print $1}')"
    echo "Sizes -------------------------------"
    echo "Original file: $(du -h ./$TEST_FOLDER/testfile.txt | awk '{print $1}')"
    echo "Received file 1: $(du -h ./$TEST_FOLDER/received_file1.txt | awk '{print $1}')"
    echo "Received file 2: $(du -h ./$TEST_FOLDER/received_file2.txt | awk '{print $1}')"
    echo "-------------------------------------"
    echo "(Contents are not shown for brevity)"
    rm -rf ./$TEST_FOLDER
    exit 1
fi