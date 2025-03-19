package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"shard/internal/sharding"
	"sort"
	"strconv"
	"strings"
	"time"

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
	shardsDir string
	shardMap  map[string][]sharding.Shard
}

func New() (*P2PNode, error) {
	node := P2PNode{
		peerAddrs: make(map[peer.ID]multiaddr.Multiaddr),
		shardsDir: "shards", // TODO: make this configurable
		shardMap:  make(map[string][]sharding.Shard),
	}

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

	configureMDNS(node)

	err = configureMDNS(node)
	if err != nil {
		return nil, err
	}

	node.host.SetStreamHandler("/file/1.0.0", (&node).handleIncomingFile)

	return &node, nil
}

func configureMDNS(n P2PNode) error {
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

	// Split the file into shards
	shards, err := sharding.SplitFile(filePath, n.shardsDir)
	if err != nil {
		fmt.Printf("Failed to split file: %v\n", err)
		return
	}

	// Store shard information
	n.shardMap[filePath] = shards

	n.distributeShards(shards)
}

func (n P2PNode) distributeShards(shards []sharding.Shard) {
	peerList := make([]peer.ID, 0, len(n.peerAddrs))
	for peerID := range n.peerAddrs {
		peerList = append(peerList, peerID)
	}

	// Distribute each shard to different peers
	for i, shard := range shards {
		peerIndex := i % len(peerList)
		go func(s sharding.Shard, pid peer.ID) {
			err := n.sendFileToPeer(s.Hash, pid)
			if err != nil {
				fmt.Printf("Failed to send shard to peer %s: %v\n", pid, err)
				return
			}
			fmt.Printf("Successfully sent shard %d to peer %s\n", s.Index, pid)
		}(shard, peerList[peerIndex])
	}
}

// RequestFileFromPeers to handle shard reconstruction
func (n P2PNode) RequestFileFromPeers(hash string) error {
	fmt.Println("Requesting file from peers")
	printShardsMap(n)
	err := n.missingShards(hash)
	if err != nil {
		return fmt.Errorf("failed to retrieve missing shards: %v", err)
	}
	sortedShards := sortShards(n.shardMap[hash])
	fmt.Println("sortedShards:", sortedShards)

	// Merge shards back into the original file
	err = sharding.MergeShards(sortedShards, hash)
	if err != nil {
		return fmt.Errorf("failed to merge shards: %v", err)
	}

	return nil
}

func (n P2PNode) missingShards(hash string) error {
	_, exists := n.shardMap[hash]
	if !exists {
		// If we don't have shard information, throw an error for now
		// TODO: Implement shard discovery
		return fmt.Errorf("shard information not found")
	}
	fmt.Println("Shard info found")
	n.requestMissingShards(hash)
	return nil
}

func (n P2PNode) requestMissingShards(hash string) {
	fmt.Println("Requesting missing shards")
	// TODO: This has a problem, it will request shards until an error is thrown
	// If a retrival fails, it will not try to retrieve the next shard
	// This is a temporary solution
	i := 0
	for {
		if hasShardIndex(n.shardMap[hash], i) {
			fmt.Printf("Already have shard %d, skipping\n", i)
			i++
			continue
		}

		fmt.Println("Requesting shard", i)
		shard, err := n.requestSingleShard(hash + "." + strconv.Itoa(i))
		if err != nil {
			break
		}
		n.shardMap[hash] = append(n.shardMap[hash], shard)
		i++
	}
}

// Helper function to check if a shard with a specific index exists
func hasShardIndex(shards []sharding.Shard, index int) bool {
	for _, shard := range shards {
		if shard.Index == index {
			return true
		}
	}
	return false
}

func (n P2PNode) requestSingleShard(fullShardPath string) (sharding.Shard, error) {
	// TODO: This is a temporary solution
	// Should find a better way to add this to the shard hash
	fullShardPath = n.shardsDir + "/" + fullShardPath

	fmt.Println("Requesting shard", fullShardPath)
	fmt.Println("peer ids known", n.peerAddrs)
	for peerID := range n.peerAddrs {
		fmt.Println("requesting peer id", peerID)

		shard, err := n.requestShardFromPeer(peerID, fullShardPath)
		if err == nil {
			return shard, nil
		}
		fmt.Printf("Peer %s couldn't provide shard: %v\n", peerID, err)
	}
	return sharding.Shard{}, fmt.Errorf("shard not found in any peer")
}

func (n P2PNode) requestShardFromPeer(peerID peer.ID, shardPath string) (sharding.Shard, error) {
	// Adding timeout to stream creation
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

func (n P2PNode) sendGetRequest(stream network.Stream, shardPath string) error {
	_, err := stream.Write([]byte("GET " + shardPath + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	return nil
}

func (n P2PNode) downloadShardFile(shardPath string, reader *bufio.Reader) (sharding.Shard, error) {
	// Create file
	file, err := os.Create(shardPath)
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Download file content
	written, err := io.Copy(file, reader)
	if err != nil {
		// If we got a partial file, we might want to keep it and report the error
		return sharding.Shard{}, fmt.Errorf("failed to write file: %v", err)
	}

	// Create shard metadata
	return n.createShardMetadata(shardPath, written)
}

func (n P2PNode) createShardMetadata(shardPath string, size int64) (sharding.Shard, error) {
	index, err := sharding.ShardIndex(shardPath)
	if err != nil {
		return sharding.Shard{}, fmt.Errorf("failed to get shard index: %v", err)
	}

	return sharding.Shard{
		Index: index,
		Hash:  shardPath,
		Size:  size,
	}, nil
}

func (n *P2PNode) handleIncomingFile(stream network.Stream) {
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
        n.handleFileUpload(stream, reader, firstLine)
	}
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

func (n *P2PNode) handleFileUpload(stream network.Stream, reader *bufio.Reader, filename string) {
	// Create shards directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", dir, err)
		return
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	// Copy the contents to the file
	byteSize, err := io.Copy(file, reader)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	printShardsMap(*n)

	// Update shards map if this is a shard file
	if strings.Contains(filename, n.shardsDir+"/") {
		fmt.Println("Updating shards map")
		// Extract original file hash and shard index from filename
		parts := strings.Split(filepath.Base(filename), ".")
		if len(parts) == 2 {
			originalFile := parts[0]
			shardIndex := parts[1]
			// Convert string index to integer
			shardIdx, err := strconv.Atoi(shardIndex)
			if err != nil {
				fmt.Printf("Error converting shard index: %v\n", err)
				return
			}

			// Create or update shard information
			shard := sharding.Shard{
				Index: shardIdx,
				Hash:  filename,
				Size:  byteSize,
			}

			// Add to shards map
			if _, exists := n.shardMap[originalFile]; !exists {
				n.shardMap[originalFile] = make([]sharding.Shard, 0)
			}
			n.shardMap[originalFile] = append(n.shardMap[originalFile], shard)

			fmt.Printf("Updated shards map for file %s with shard %s\n", originalFile, shardIndex)
		}
	}

	fmt.Printf("Received file: %s from peer: %s\n with: %d bytes\n", filename, stream.Conn().RemotePeer(), byteSize)
}

// func (n *P2PNode) handleIncomingFile(stream network.Stream) {
// 	defer stream.Close()

// 	// Read the first line
// 	reader := bufio.NewReader(stream)
// 	firstLine, err := reader.ReadString('\n')
// 	if err != nil {
// 		fmt.Printf("Error reading request: %v\n", err)
// 		return
// 	}
// 	firstLine = strings.TrimSpace(firstLine)

// 	// Check if this is a GET request
// 	if strings.HasPrefix(firstLine, "GET ") {
// 		filename := strings.TrimPrefix(firstLine, "GET ")
// 		file, err := os.Open(filename)
// 		if err != nil {
// 			stream.Write([]byte("NOT FOUND\n"))
// 			return
// 		}
// 		defer file.Close()

// 		// Send OK response
// 		_, err = stream.Write([]byte("OK\n"))
// 		if err != nil {
// 			return
// 		}

// 		// Send file contents
// 		_, err = io.Copy(stream, file)
// 		if err != nil {
// 			fmt.Printf("Error sending file: %v\n", err)
// 		}
// 		return
// 	}

// 	// Handle regular file upload
// 	filename := firstLine

// 	// Create shards directory if it doesn't exist
// 	dir := filepath.Dir(filename)
// 	if err := os.MkdirAll(dir, 0755); err != nil {
// 		fmt.Printf("Error creating directory %s: %v\n", dir, err)
// 		return
// 	}

// 	// Create the file
// 	file, err := os.Create(filename)
// 	if err != nil {
// 		fmt.Printf("Error creating file: %v\n", err)
// 		return
// 	}
// 	defer file.Close()

// 	// Copy the contents to the file
// 	byteSize, err := io.Copy(file, reader)
// 	if err != nil {
// 		fmt.Printf("Error writing file: %v\n", err)
// 		return
// 	}

// 	printShardsMap(*n)

// 	// Update shards map if this is a shard file
// 	if strings.Contains(filename, n.shardsDir+"/") {
// 		fmt.Println("Updating shards map")
// 		// Extract original file hash and shard index from filename
// 		parts := strings.Split(filepath.Base(filename), ".")
// 		if len(parts) == 2 {
// 			originalFile := parts[0]
// 			shardIndex := parts[1]
// 			// Convert string index to integer
// 			shardIdx, err := strconv.Atoi(shardIndex)
// 			if err != nil {
// 				fmt.Printf("Error converting shard index: %v\n", err)
// 				return
// 			}

// 			// Create or update shard information
// 			shard := sharding.Shard{
// 				Index: shardIdx,
// 				Hash:  filename,
// 				Size:  byteSize,
// 			}

// 			// Add to shards map
// 			if _, exists := n.shardMap[originalFile]; !exists {
// 				n.shardMap[originalFile] = make([]sharding.Shard, 0)
// 			}
// 			n.shardMap[originalFile] = append(n.shardMap[originalFile], shard)

// 			fmt.Printf("Updated shards map for file %s with shard %s\n", originalFile, shardIndex)
// 		}
// 	}

// 	fmt.Printf("Received file: %s from peer: %s\n with: %d bytes\n", filename, stream.Conn().RemotePeer(), byteSize)
// }

// TODO: This is not perfect as we are using goroutines which also print outputs
func printShardsMap(n P2PNode) {
	fmt.Println("Shards map ----------------")
	for shard, shards := range n.shardMap {
		fmt.Println("shard:", shard)
		for _, shard := range shards {
			fmt.Println("shard.Index:", shard.Index)
			fmt.Println("shard.Hash:", shard.Hash)
		}
	}
	fmt.Println("---------------------------")
}

func (n P2PNode) Close() error {
	err := n.host.Close()
	if err != nil {
		return fmt.Errorf("failed to close host: %v", err)
	}
	return nil
}

// TODO: use a better sorting algorithm
func sortShards(shards []sharding.Shard) []sharding.Shard {
	sorted := make([]sharding.Shard, len(shards))
	copy(sorted, shards)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Index < sorted[j].Index
	})
	return sorted
}
