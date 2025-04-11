package node

import (
	"context"
	"fmt"
	"shard/internal/sharding"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
)

type P2PNode struct {
	ID   peer.ID
	Addr string
	host host.Host

	destDir string // Where files are stored

	connected map[peer.ID]bool
	peerAddrs map[peer.ID]multiaddr.Multiaddr
	peerLock  sync.Mutex

	shardsDir     string // Where shards are stored
	shardMap      map[string][]sharding.Shard
	shardMapMutex sync.RWMutex
}

// New creates a new P2P node
func New(destDir string) (*P2PNode, error) {
	node := P2PNode{
		peerAddrs: make(map[peer.ID]multiaddr.Multiaddr),
		shardsDir: "shards", // TODO: make this configurable
		destDir:   destDir,
		shardMap:  make(map[string][]sharding.Shard),
		connected: make(map[peer.ID]bool),
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.NATPortMap(),
		// Force relay option to help with NAT traversal
		libp2p.ForceReachabilityPrivate(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %v", err)
	}
	node.host = h

	node.ID = h.ID()
	fmt.Println("Node ID:", h.ID())

	node.Addr = fmt.Sprintf("%s/p2p/%s", h.Addrs()[0], h.ID().String())
	fmt.Printf("Node address: %s/p2p/%s\n", h.Addrs()[0], h.ID().String())

	// Set up connection notification callbacks
	node.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			node.peerLock.Lock()
			node.connected[conn.RemotePeer()] = true
			node.peerLock.Unlock()
			fmt.Printf("Connected to peer: %s\n", conn.RemotePeer().String())
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			node.peerLock.Lock()
			node.connected[conn.RemotePeer()] = false
			node.peerLock.Unlock()
			fmt.Printf("Disconnected from peer: %s\n", conn.RemotePeer().String())
		},
	})

	err = configureMDNS(&node)
	if err != nil {
		return nil, err
	}

	node.host.SetStreamHandler("/file/1.0.0", (&node).handleIncomingRequest)

	return &node, nil
}

func configureMDNS(n *P2PNode) error {
	mdnsService := mdns.NewMdnsService(n.host, "libp2p-file-upload", n)
	if mdnsService == nil {
		return fmt.Errorf("failed to create mDNS service")
	}

	err := mdnsService.Start()
	if err != nil {
		return fmt.Errorf("failed to start mDNS service: %v", err)
	}
	return nil
}

// HandlePeerFound is called when a new peer is discovered
func (n *P2PNode) HandlePeerFound(pi peer.AddrInfo) {
	// Skip if this is our own peer ID
	if pi.ID == n.ID {
		return
	}

	fmt.Printf("Discovered new peer %s\n", pi.ID.String())

	n.peerLock.Lock()
	if _, exists := n.peerAddrs[pi.ID]; exists {
		// We already know about this peer
		n.peerLock.Unlock()
		return
	}
	n.peerAddrs[pi.ID] = pi.Addrs[0]
	n.peerLock.Unlock()

	n.addressesKnow()

	// Only the node with the smaller ID initiates the connection
	// This prevents both nodes trying to connect simultaneously
	if n.ID.String() < pi.ID.String() {
		fmt.Printf("Initiating connection to peer %s\n", pi.ID.String())
		go func(peer peer.AddrInfo) {
			n.connectWithRetry(peer)
		}(pi)
	} else {
		// The other peer will initiate the connection
		fmt.Printf("Waiting for peer %s to initiate connection\n", pi.ID.String())
	}
}

func (n *P2PNode) connectWithRetry(pi peer.AddrInfo) {
	// Use a backoff strategy
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		n.peerLock.Lock()
		if connected, exists := n.connected[pi.ID]; exists && connected {
			n.peerLock.Unlock()
			return
		}
		n.peerLock.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := n.host.Connect(ctx, pi)
		cancel()

		if err == nil {
			return
		}

		// If this is the last attempt, log the error
		if i == maxRetries-1 {
			fmt.Printf("Failed to connect to peer %s after %d attempts: %s\n",
				pi.ID.String(), maxRetries, err)
			return
		}

		// Wait before retrying, with increasing backoff
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
	}
}

func (n *P2PNode) addressesKnow() {
	for _, addr := range n.host.Addrs() {
		fmt.Printf("Known address: %s\n", addr)
	}
}

// IsPeerConnected checks if we have an active connection to a peer
func (n *P2PNode) IsPeerConnected(id peer.ID) bool {
	n.peerLock.Lock()
	defer n.peerLock.Unlock()

	if connected, exists := n.connected[id]; exists && connected {
		return true
	}

	// Double-check with libp2p
	return n.host.Network().Connectedness(id) == network.Connected
}

// WaitForConnection waits until we have a connection to the specified peer or timeout
func (n *P2PNode) WaitForConnection(id peer.ID, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if n.IsPeerConnected(id) {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// Close shuts down the P2P node
func (n *P2PNode) Close() error {
	err := n.host.Close()
	if err != nil {
		return fmt.Errorf("failed to close host: %v", err)
	}
	return nil
}
