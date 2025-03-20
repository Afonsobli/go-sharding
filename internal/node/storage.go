package node

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"shard/internal/sharding"
	"strings"
)

func (n P2PNode) downloadShardFile(shardPath string, reader *bufio.Reader) (sharding.Shard, error) {
	// Create file
	file, err := os.Create(shardPath)
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Download file content
	written, err := io.Copy(file, reader)
	if err != nil {
		// If we got a partial file, we might want to keep it and report the error
		return sharding.Shard{}, fmt.Errorf("failed to write file: %v", err)
	}

	// Create shard metadata
	return n.createShardMetadata(shardPath, written)
}

func (n *P2PNode) handleFileUpload(reader *bufio.Reader, filename string) {
	// Create directories if needed
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", dir, err)
		return
	}

	// Create and write to file
	file, byteSize, err := n.createAndWriteFile(filename, reader)
	if err != nil {
		fmt.Printf("Error with file handling: %v\n", err)
		return
	}
	defer file.Close()

	n.printShardsMap()

	// Update shards map if this is a shard file
	if strings.Contains(filename, n.shardsDir+"/") {
		n.updateShardMetadata(filename, byteSize)
	}
}

func (n *P2PNode) createAndWriteFile(filename string, reader *bufio.Reader) (*os.File, int64, error) {
	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating file: %v", err)
	}

	// Copy the contents to the file
	byteSize, err := io.Copy(file, reader)
	if err != nil {
		file.Close() // Close file on error
		return nil, 0, fmt.Errorf("error writing file: %v", err)
	}

	return file, byteSize, nil
}
