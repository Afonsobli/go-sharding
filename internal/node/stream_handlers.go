package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"shard/internal/sharding"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (n P2PNode) sendFileToPeer(filePath string, peerID peer.ID) error {
	fmt.Println("Sending file to peers")

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create a new stream to the peer
	stream, err := n.host.NewStream(context.Background(), peerID, "/file/1.0.0")
	if err != nil {
		return fmt.Errorf("failed to create stream to peer %s: %v", peerID, err)
	}
	defer stream.Close()

	// First send the filename
	filename := []byte(filePath + "\n")
	_, err = stream.Write(filename)
	if err != nil {
		return fmt.Errorf("failed to send filename: %v", err)
	}

	// Then send the file contents
	_, err = io.Copy(stream, file)
	if err != nil {
		return fmt.Errorf("failed to send file: %v", err)
	}

	return nil
}

func (n P2PNode) requestShardFromPeer(peerID peer.ID, shardPath string) (sharding.Shard, error) {
	// Timeout to stream creation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create stream to peer
	stream, err := n.host.NewStream(ctx, peerID, "/file/1.0.0")
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to create stream: %v", err)
	}
	defer stream.Close()

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

func (n *P2PNode) handleIncomingRequest(stream network.Stream) {
	defer stream.Close()

	// Read the first line
	reader := bufio.NewReader(stream)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading request: %v\n", err)
		return
	}
	firstLine = strings.TrimSpace(firstLine)

	// Route to appropriate handler based on request type
	if strings.HasPrefix(firstLine, "GET ") {
		filename := strings.TrimPrefix(firstLine, "GET ")
		n.handleGetRequest(stream, filename)
	} else {
		n.handleFileUpload(reader, firstLine)
	}
	fmt.Printf("Handled request: %s from peer: %s", firstLine, stream.Conn().RemotePeer())
}

func (n P2PNode) sendGetRequest(stream network.Stream, shardPath string) error {
	_, err := stream.Write([]byte("GET " + shardPath + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	return nil
}

func (n *P2PNode) handleGetRequest(stream network.Stream, filename string) {
	file, err := os.Open(filename)
	if err != nil {
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
