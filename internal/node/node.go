package node

import (
	"context"
	"fmt"
	"shard/internal/sharding"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
)

type P2PNode struct {
	ID        peer.ID
	Addr      string
	host      host.Host
	peerAddrs map[peer.ID]multiaddr.Multiaddr
	shardsDir string // Where shards are stored
	destDir   string // Where files are stored
	shardMap  map[string][]sharding.Shard
}

func New(destDir string) (*P2PNode, error) {
	node := P2PNode{
		peerAddrs: make(map[peer.ID]multiaddr.Multiaddr),
		shardsDir: "shards", // TODO: make this configurable
		destDir:   destDir,
		shardMap:  make(map[string][]sharding.Shard),
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

func (n *P2PNode) HandlePeerFound(pi peer.AddrInfo) {
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

func (n *P2PNode) Close() error {
	err := n.host.Close()
	if err != nil {
		return fmt.Errorf("failed to close host: %v", err)
	}
	return nil
}
