package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	tls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
)

type P2PNode struct {
	ID        peer.ID
	Addr      string
	host      host.Host
	peerAddrs map[peer.ID]multiaddr.Multiaddr
}

func New() (*P2PNode, error) {
	node := P2PNode{peerAddrs: make(map[peer.ID]multiaddr.Multiaddr)}
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Security(tls.ID, tls.New),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %v", err)
	}
	node.host = h

	node.ID = h.ID()
	fmt.Println("Node ID:", h.ID())

	node.Addr = fmt.Sprintf("%s/p2p/%s", h.Addrs()[0], h.ID().String())
	fmt.Printf("Node address: %s/p2p/%s\n", h.Addrs()[0], h.ID().String())

	mdnsService := mdns.NewMdnsService(h, "libp2p-file-upload", node)
	if mdnsService == nil {
		return nil, fmt.Errorf("failed to create mDNS service")
	}

	err = mdnsService.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start mDNS service: %v", err)
	}

	node.host.SetStreamHandler("/file/1.0.0", node.handleIncomingFile)

	return &node, nil
}

func (n P2PNode) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("Discovered new peer %s\n", pi.ID.String())
	n.peerAddrs[pi.ID] = pi.Addrs[0]
	n.addressesKnow()
	if err := n.host.Connect(context.Background(), pi); err != nil {
		fmt.Printf("Failed to connect to peer %s: %s\n", pi.ID.String(), err)
	}
}

func (n *P2PNode) addressesKnow() {
	for _, addr := range n.host.Addrs() {
		fmt.Printf("Known address: %s\n", addr)
	}
}

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

func (n P2PNode) DistributeFile(filePath string) {
	fmt.Println("Distributing file to peers")
	fmt.Println("len(n.peerAddrs):", len(n.peerAddrs))

	for peerID := range n.peerAddrs {
		go func(pid peer.ID) {
			err := n.sendFileToPeer(filePath, pid)
			if err != nil {
				fmt.Printf("Failed to send file to peer %s: %v\n", pid, err)
				return
			}
			fmt.Printf("Successfully sent file to peer %s\n", pid)
		}(peerID)
	}
}

func (n P2PNode) RequestFileFromPeers(hash string) error {
	for peerID := range n.peerAddrs {
		stream, err := n.host.NewStream(context.Background(), peerID, "/file/1.0.0")
		if err != nil {
			fmt.Printf("Failed to create stream to peer %s: %v\n", peerID, err)
			continue
		}
		defer stream.Close()

		// Send request for file
		_, err = stream.Write([]byte("GET " + hash + "\n"))
		if err != nil {
			fmt.Printf("Failed to send request to peer %s: %v\n", peerID, err)
			continue
		}

		// Read response
		reader := bufio.NewReader(stream)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Failed to read response from peer %s: %v\n", peerID, err)
			continue
		}

		if strings.TrimSpace(response) == "OK" {
			// Copy the file contents
			file, err := os.Create(hash)
			if err != nil {
				return fmt.Errorf("failed to create file: %v", err)
			}
			defer file.Close()

			_, err = io.Copy(file, reader)
			if err != nil {
				return fmt.Errorf("failed to write file: %v", err)
			}
			return nil
		}
	}
	return fmt.Errorf("file not found in any peer")
}

func (n P2PNode) handleIncomingFile(stream network.Stream) {
	defer stream.Close()

	// Read the first line
	reader := bufio.NewReader(stream)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading request: %v\n", err)
		return
	}
	firstLine = strings.TrimSpace(firstLine)

	// Check if this is a GET request
	if strings.HasPrefix(firstLine, "GET ") {
		filename := strings.TrimPrefix(firstLine, "GET ")
		file, err := os.Open(filename)
		if err != nil {
			stream.Write([]byte("NOT FOUND\n"))
			return
		}
		defer file.Close()

		// Send OK response
		_, err = stream.Write([]byte("OK\n"))
		if err != nil {
			return
		}

		// Send file contents
		_, err = io.Copy(stream, file)
		if err != nil {
			fmt.Printf("Error sending file: %v\n", err)
		}
		return
	}

	// Handle regular file upload
	filename := firstLine

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	// Copy the contents to the file
	_, err = io.Copy(file, reader)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Printf("Received file: %s from peer: %s\n", filename, stream.Conn().RemotePeer())
}

func (n P2PNode) Close() error {
	err := n.host.Close()
	if err != nil {
		return fmt.Errorf("failed to close host: %v", err)
	}
	return nil
}
