package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"shard/internal/sharding"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (n *P2PNode) sendShardToPeer(shardPath string, peerID peer.ID) error {
	fmt.Println("Sending shard to peers")

	// Open the shardFile
	shardFile, err := os.Open(shardPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer shardFile.Close()

	// Create a new stream to the peer
	stream, err := n.host.NewStream(context.Background(), peerID, "/file/1.0.0")
	if err != nil {
		return fmt.Errorf("failed to create stream to peer %s: %v", peerID, err)
	}
	defer stream.Close()

	// First send the shard name
	shardName := []byte(filepath.Base(shardPath) + "\n")
	_, err = stream.Write([]byte(requestTypeUpload + " " + string(shardName) + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send shard name: %v", err)
	}

	// Then send the shard contents
	_, err = io.Copy(stream, shardFile)
	if err != nil {
		return fmt.Errorf("failed to send shard file: %v", err)
	}

	return nil
}

func (n *P2PNode) requestShardFromPeer(peerID peer.ID, shardPath string) (sharding.Shard, error) {
	// Timeout to stream creation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create stream to peer
	stream, err := n.host.NewStream(ctx, peerID, "/file/1.0.0")
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to create stream: %v", err)
	}
	defer stream.Close()

	fmt.Println("requesting shardpath", shardPath)

	if err := n.sendGetRequest(stream, shardPath); err != nil {
		return sharding.Shard{}, err
	}

	reader := bufio.NewReader(stream)
	response, err := reader.ReadString('\n')
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to read response: %v", err)
	}

	if strings.TrimSpace(response) != "OK" {
		return sharding.Shard{}, fmt.Errorf("peer does not have shard")
	}

	// Download the file and create shard metadata
	return n.downloadShardFile(shardPath, reader)
}

func (n *P2PNode) requestMaxIndexOfShard(peerID peer.ID, shardPath string) (int, error) {
	// Timeout to stream creation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create stream to peer
	stream, err := n.host.NewStream(ctx, peerID, "/file/1.0.0")
	if err != nil {
		return -1, fmt.Errorf("failed to create stream: %v", err)
	}
	defer stream.Close()

	fmt.Println("requesting max index of shard", shardPath)

	if err := n.sendMaxIndexRequest(stream, shardPath); err != nil {
		return -1, err
	}

	reader := bufio.NewReader(stream)
	response, err := reader.ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Println("response", response)
	if strings.TrimSpace(response) != "OK" {
		return -1, fmt.Errorf("peer does not have shard, could not receive max index")
	}

	// Read the max index
	maxIndexStr, err := reader.ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("failed to read max index: %v", err)
	}

	fmt.Println("max index string", maxIndexStr)

	maxIndexStr = strings.TrimSpace(maxIndexStr)
	maxIndex, err := strconv.Atoi(maxIndexStr)
	if err != nil {
		return -1, fmt.Errorf("failed to convert max index to int: %v", err)
	}
	return maxIndex, nil
}

const (
	requestTypeUpload   = "SHARD"
	requestTypeGet      = "GET"
	requestTypeMaxIndex = "MAX_INDEX"
)

func (n *P2PNode) handleIncomingRequest(stream network.Stream) {
	defer stream.Close()

	// Read the first line
	reader := bufio.NewReader(stream)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading request: %v\n", err)
		return
	}
	fmt.Println("FirstLine", firstLine)
	firstLine = strings.TrimSpace(firstLine)

	parts := strings.SplitN(firstLine, " ", 2)
	if len(parts) < 2 {
		fmt.Println("Invalid request format")
		return
	}

	requestType := parts[0]
	payload := parts[1]

	switch requestType {
	case requestTypeGet:
		n.handleGetRequest(stream, payload)
	case requestTypeMaxIndex:
		n.handleMaxIndexRequest(stream, payload)
	default:
		n.handleFileUpload(reader, firstLine)
	}

	fmt.Printf("Handled %s request from peer: %s\n", requestType, stream.Conn().RemotePeer())
}

// Update the send request function to match the new format
func (n *P2PNode) sendGetRequest(stream network.Stream, shardPath string) error {
	fmt.Print("Sending GET request for shard:", requestTypeGet+" "+shardPath+"\n")
	_, err := stream.Write([]byte(requestTypeGet + " " + shardPath + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	return nil
}

func (n *P2PNode) handleGetRequest(stream network.Stream, filename string) {
	filepath := filepath.Join(n.shardsDir, filename)
	file, err := os.Open(filepath)
	if err != nil {
		fmt.Println("File not found in this peer")
		stream.Write([]byte("NOT FOUND\n"))
		return
	}
	defer file.Close()

	// Send OK response
	_, err = stream.Write([]byte("OK\n"))
	if err != nil {
		fmt.Printf("Error sending OK response: %v\n", err)
		return
	}

	// Send file contents
	_, err = io.Copy(stream, file)
	if err != nil {
		fmt.Printf("Error sending file: %v\n", err)
	}
}

// sendMaxIndexRequest sends a request to a peer to get the maximum index of a shard
func (n *P2PNode) sendMaxIndexRequest(stream network.Stream, shardHash string) error {
	fmt.Print("Sending MaxIndex request for shard:", requestTypeMaxIndex+" "+shardHash+"\n")
	_, err := stream.Write([]byte(requestTypeMaxIndex + " " + shardHash + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send max index request: %v", err)
	}
	return nil
}

// handleMaxIndexRequest handles incoming requests for the maximum index of a shard
func (n *P2PNode) handleMaxIndexRequest(stream network.Stream, shardHash string) {
	maxIndex := n.getMaxShardIndex(shardHash)
	if maxIndex == -1 {
		fmt.Println("While handling, no shards were found for hash:", shardHash)
		stream.Write([]byte("NOT FOUND\n"))
		return
	}
	// Send OK response
	_, err := stream.Write([]byte("OK\n"))
	if err != nil {
		fmt.Printf("Error sending OK response: %v\n", err)
		return
	}
	_, err = stream.Write([]byte(fmt.Sprintf("%d\n", maxIndex)))
	if err != nil {
		fmt.Printf("Error sending max index response: %v\n", err)
	}
}
