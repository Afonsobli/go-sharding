package node

import (
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

	// Wait for peer discovery and connection
	fmt.Println("Waiting for peer discovery...")
	success := node1.WaitForConnection(node2.ID, 5*time.Second)
	if !success {
		t.Fatalf("Nodes failed to discover and connect to each other")
	}
	fmt.Println("Nodes connected successfully")

	// Create a test file in node1's directory
	testFileName := "testfile.txt"
	testContent := "This is a test file for P2P transfer"
	err = os.WriteFile(testFileName, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fmt.Println("wrote to testFileName", testFileName)
	defer os.RemoveAll(testFileName)

	if !node1.IsPeerConnected(node2.ID) {
		t.Fatalf("Connection to peer lost before file transfer")
	}	

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
	defer os.RemoveAll(receivedFilePath)

	if string(receivedContent) != testContent {
		t.Errorf("File content mismatch. Expected '%s', got '%s'", testContent, string(receivedContent))
	}
}
