package node

import (
	"fmt"
	"shard/internal/sharding"
	"strconv"
)

// RequestFileFromPeers to handle shard reconstruction
func (n P2PNode) RequestFileFromPeers(hash string) error {
	fmt.Println("Requesting file from peers")
	n.printShardsMap()
	err := n.missingShards(hash)
	if err != nil {
		return fmt.Errorf("failed to retrieve missing shards: %v", err)
	}
	sortedShards := sharding.SortShards(n.shardMap[hash])
	fmt.Println("sortedShards:", sortedShards)

	// Merge shards back into the original file
	err = sharding.MergeShards(sortedShards, n.destDir, n.shardsDir, hash)
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
		fmt.Println("no info")
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

func (n P2PNode) requestSingleShard(shardHash string) (sharding.Shard, error) {
	fmt.Println("Requesting shard", shardHash)
	fmt.Println("peer ids known", n.peerAddrs)
	for peerID := range n.peerAddrs {
		fmt.Println("requesting peer id", peerID)

		shard, err := n.requestShardFromPeer(peerID, shardHash)
		if err == nil {
			return shard, nil
		}
		fmt.Printf("Peer %s couldn't provide shard: %v\n", peerID, err)
	}
	return sharding.Shard{}, fmt.Errorf("shard not found in any peer")
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
