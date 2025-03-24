package node

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestNewNode(t *testing.T) {
	node, err := New("out")
	if err != nil {
		t.Fatalf("Failed to create new node: %v", err)
	}
	defer node.Close()

	if node.ID == "" {
		t.Error("Node ID is empty")
	}

	if node.host == nil {
		t.Error("Node host is nil")
	}

	if len(node.peerAddrs) != 0 {
		t.Error("New node should have no peer addresses")
	}

	if node.shardsDir != "shards" {
		t.Errorf("Expected shardsDir to be 'shards', got '%s'", node.shardsDir)
	}
}

func TestHandlePeerFound(t *testing.T) {
	// Create two nodes
	node1, err := New("out")
	if err != nil {
		t.Fatalf("Failed to create first node: %v", err)
	}
	defer node1.Close()

	node2, err := New("out")
	if err != nil {
		t.Fatalf("Failed to create second node: %v", err)
	}
	defer node2.Close()

	// Manually trigger peer discovery
	addrInfo := peer.AddrInfo{
		ID:    node2.ID,
		Addrs: node2.host.Addrs(),
	}
	node1.HandlePeerFound(addrInfo)

	// We verify that the peer was added
	if len(node1.peerAddrs) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(node1.peerAddrs))
	}

	// Expecting the peer ID matches
	if _, exists := node1.peerAddrs[node2.ID]; !exists {
		t.Error("Peer ID was not added correctly")
	}
}
