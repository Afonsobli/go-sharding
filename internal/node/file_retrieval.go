package node

import (
	"fmt"
	"shard/internal/sharding"
	"strconv"
)

// RequestFileFromPeers to handle shard reconstruction
func (n *P2PNode) RequestFileFromPeers(hash string) error {
	fmt.Println("Requesting file from peers")
	fmt.Println("Shard map before retrieval:")
	n.printShardsMap()
	err := n.missingShards(hash)
	if err != nil {
		return fmt.Errorf("failed to retrieve missing shards: %v", err)
	}
	// TODO: use shard manager
	sortedShards := sharding.SortShards(n.shardMap[hash])
	fmt.Println("sortedShards:", sortedShards)

	// Merge shards back into the original file
	err = sharding.MergeShards(sortedShards, n.destDir, n.shardsDir, hash)
	if err != nil {
		return fmt.Errorf("failed to merge shards: %v", err)
	}

	return nil
}

func (n *P2PNode) missingShards(hash string) error {
	// TODO: use shard manager
	_, exists := n.shardMap[hash]
	if !exists {
		// If we don't have shard information, initialize shard discovery
		// TODO: use shard manager
		fmt.Println("No shard info found, initializing empty entry and attempting discovery")
		n.shardMapMutex.Lock()
		n.shardMap[hash] = []sharding.Shard{}
		n.shardMapMutex.Unlock()
	} else {
		fmt.Println("Shard info found, checking for missing shards")
	}
	n.requestMissingShards(hash)

	// Verify we found at least one shard
	// TODO: use shard manager
	if len(n.shardMap[hash]) == 0 {
		return fmt.Errorf("failed to find any shards for file %s", hash)
	}
	return nil
}

func (n *P2PNode) requestMissingShards(hash string) {
	fmt.Println("Requesting missing shards")
	maxIndex, err := n.discoverMaxIndexOfHash(hash)
	if err != nil {
		fmt.Printf("Error discovering max index of hash %s: %v\n", hash, err)
		return
	}
	for i := range maxIndex + 1 {
		// TODO: use shard manager
		if hasShardIndex(n.shardMap[hash], i) {
			fmt.Printf("Already have shard %d, skipping\n", i)
			continue
		}

		fmt.Println("Requesting shard", i)
		shard, err := n.requestSingleShard(hash + "." + strconv.Itoa(i))
		if err != nil {
			break
		}

		// TODO: use shard manager
		n.shardMapMutex.Lock()
		n.shardMap[hash] = append(n.shardMap[hash], shard)
		n.shardMapMutex.Unlock()
	}
}

func (n *P2PNode) discoverMaxIndexOfHash(shardHash string) (int, error) {
	fmt.Println("Requesting max index of shard", shardHash)
	maxIndex := -1
	for peerID := range n.peerAddrs {
		fmt.Println("requesting max index from peer id", peerID)

		index, err := n.requestMaxIndexOfShard(peerID, shardHash)
		if err != nil {
			fmt.Printf("Peer %s couldn't provide shard's max index: %v\n", peerID, err)
			continue
		}
		fmt.Printf("Peer %s provided shard's max index: %d\n", peerID, index)
		fmt.Printf("index (%d) > maxIndex (%d)\n", index, maxIndex)
		if index > maxIndex {
			fmt.Println("updating max index")
			maxIndex = index
		}
	}
	if maxIndex == -1 {
		fmt.Println("shard not found in any peer")
		return -1, fmt.Errorf("shard not found in any peer")
	}
	fmt.Printf("Max index of shard %s is %d\n", shardHash, maxIndex)
	return maxIndex, nil
}

func (n *P2PNode) requestSingleShard(shardHash string) (sharding.Shard, error) {
	fmt.Println("Requesting shard", shardHash)
	fmt.Println("peer ids known", n.peerAddrs)
	for peerID := range n.peerAddrs {
		fmt.Println("requesting single shard from peer id", peerID)

		shard, err := n.requestShardFromPeer(peerID, shardHash)
		if err == nil {
			return shard, nil
		}
		fmt.Printf("Peer %s couldn't provide shard: %v\n", peerID, err)
	}
	return sharding.Shard{}, fmt.Errorf("shard not found in any peer")
}

func (n *P2PNode) createShardMetadata(shardPath string, size int64) (sharding.Shard, error) {
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
