package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestShardTransfer is an integration test that tests shard transfer between two nodes
func TestShardTransfer(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directories for the nodes
	node1Dir, err := os.MkdirTemp("", "node1")
	if err != nil {
		t.Fatalf("Failed to create temp dir for node1: %v", err)
	}
	defer os.RemoveAll(node1Dir)

	node2Dir, err := os.MkdirTemp("", "node2")
	if err != nil {
		t.Fatalf("Failed to create temp dir for node2: %v", err)
	}
	defer os.RemoveAll(node2Dir)

	// Create the nodes
	node1, err := New(node1Dir)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	defer node1.Close()

	node2, err := New(node2Dir)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	defer node2.Close()

	// Create a test file in node1's directory
	testFileName := "testfile.txt"
	testFilePath := filepath.Join(node1Dir, testFileName)
	testContent := "This is a test file for P2P transfer"
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fmt.Println("wrote to testfilepath", testFilePath)

	// Manually connect the nodes
	addrInfo2 := node2.host.Peerstore().PeerInfo(node2.ID)
	err = node1.host.Connect(context.Background(), addrInfo2)
	if err != nil {
		t.Fatalf("Failed to connect node1 to node2: %v", err)
	}

	// Register node2 in node1's peer list
	node1.peerAddrs[node2.ID] = node2.host.Addrs()[0]

	// Send file from node1 to node2 (emulating sending shards)
	err = node1.sendShardToPeer(testFileName, node2.ID)
	if err != nil {
		t.Fatalf("Failed to send file: %v", err)
	}

	// Wait for file transfer to complete
	time.Sleep(1 * time.Second)

	// Verify file was received by node2
	receivedFilePath := filepath.Join(node2.shardsDir, testFileName)
	fmt.Println("receivedfilepath", receivedFilePath)
	receivedContent, err := os.ReadFile(receivedFilePath)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}

	if string(receivedContent) != testContent {
		t.Errorf("File content mismatch. Expected '%s', got '%s'", testContent, string(receivedContent))
	}
}