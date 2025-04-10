package node

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"shard/internal/sharding"
)

func (n *P2PNode) downloadShardFile(shardPath string, reader *bufio.Reader) (sharding.Shard, error) {
	_, written, err := n.createAndWriteFile(shardPath, reader)
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to write file: %v", err)
	}

	// Create shard metadata
	return n.createShardMetadata(shardPath, written)
}

func (n *P2PNode) handleFileUpload(reader *bufio.Reader, filename string) {
	fmt.Println("Received file:", filename)

	// Create and write to file
	file, byteSize, err := n.createAndWriteFile(filename, reader)
	if err != nil {
		fmt.Printf("Error with file handling: %v\n", err)
		return
	}
	defer file.Close()

	n.updateShardMetadata(filename, byteSize)
}

func (n *P2PNode) createAndWriteFile(filename string, reader *bufio.Reader) (*os.File, int64, error) {
	// Create the file
	fmt.Println("Writing to:", filepath.Join(n.shardsDir, filename))

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(n.shardsDir, 0755); err != nil {
		return nil, 0, fmt.Errorf("error creating directory: %v", err)
	}
	fmt.Println("Created:", n.shardsDir)
		
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
