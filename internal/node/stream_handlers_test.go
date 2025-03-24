package node

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
)

// TestSendGetRequest tests the sendGetRequest function
func TestSendGetRequest(t *testing.T) {
	mockStream := &mockStream{
		writeBuffer: make([]byte, 0),
	}

	node := P2PNode{}

	err := node.sendGetRequest(mockStream, "test/path.txt")
	if err != nil {
		t.Fatalf("sendGetRequest failed: %v", err)
	}

	// Verify the request was formatted correctly
	expected := "GET test/path.txt\n"
	if string(mockStream.writeBuffer) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(mockStream.writeBuffer))
	}
}

// TestHandleGetRequest tests the handleGetRequest function
func TestHandleGetRequest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-node")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testContent := "test file content"
	testFilePath := filepath.Join(tempDir, "testfile.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	mockStream := &mockStream{
		writeBuffer: make([]byte, 0),
	}

	node := P2PNode{}

	// Test handling a GET request for an existing file
	node.handleGetRequest(mockStream, testFilePath)

	// Verify the response starts with "OK\n"
	if !strings.HasPrefix(string(mockStream.writeBuffer), "OK\n") {
		t.Errorf("Expected response to start with 'OK\\n', got '%s'", string(mockStream.writeBuffer))
	}

	// Verify the response contains the file content
	if !strings.Contains(string(mockStream.writeBuffer), testContent) {
		t.Errorf("Expected response to contain '%s', got '%s'", testContent, string(mockStream.writeBuffer))
	}

	// Test handling a GET request for a non-existent file
	mockStream.writeBuffer = make([]byte, 0)
	node.handleGetRequest(mockStream, "nonexistent.txt")

	// Verify the response is "NOT FOUND\n"
	expected := "NOT FOUND\n"
	if string(mockStream.writeBuffer) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(mockStream.writeBuffer))
	}
}

// Mock implementation of network.Stream for testing
type mockStream struct {
	writeBuffer []byte
	readBuffer  string
	network.Stream
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	m.writeBuffer = append(m.writeBuffer, p...)
	return len(p), nil
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	if len(m.readBuffer) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.readBuffer)
	m.readBuffer = m.readBuffer[n:]
	return n, nil
}

func (m *mockStream) Close() error {
	return nil
}
