package node

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"shard/internal/sharding"
)

func (n P2PNode) downloadShardFile(shardPath string, reader *bufio.Reader) (sharding.Shard, error) {
	_, written, err := n.createAndWriteFile(shardPath, reader)
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to write file: %v", err)
	}

	// Create shard metadata
	return n.createShardMetadata(shardPath, written)
}

func (n *P2PNode) handleFileUpload(reader *bufio.Reader, filename string) {
	// Create directories if needed
	if err := os.MkdirAll(n.shardsDir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", n.destDir, err)
		return
	}

	fmt.Println("Received file:", filename)
	fmt.Println("Created:", n.shardsDir)

	// Create and write to file
	file, byteSize, err := n.createAndWriteFile(filename, reader)
	if err != nil {
		fmt.Printf("Error with file handling: %v\n", err)
		return
	}
	defer file.Close()

	// Update shards map if this is a shard file
	// TODO: Change the detection method
	// Check for dot followed by a number (e.g., ".1", ".2", etc.)
	if matched, _ := filepath.Match("*.[0-9]*", filename); matched {
		n.updateShardMetadata(filename, byteSize)
	}
}

func (n *P2PNode) createAndWriteFile(filename string, reader *bufio.Reader) (*os.File, int64, error) {
	// Create the file
	fmt.Println("Writing to:", filepath.Join(n.shardsDir, filename))
	file, err := os.Create(filepath.Join(n.shardsDir, filename))
	if err != nil {
		return nil, 0, fmt.Errorf("error creating file: %v", err)
	}

	fmt.Println("Created file:", file.Name())

	// Copy the contents to the file
	byteSize, err := io.Copy(file, reader)
	if err != nil {
		file.Close() // Close file on error
		return nil, 0, fmt.Errorf("error writing file: %v", err)
	}

	return file, byteSize, nil
}
