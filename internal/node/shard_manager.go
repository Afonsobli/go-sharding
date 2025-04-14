package node

import (
	"fmt"
	"path/filepath"
	"shard/internal/sharding"
	"strconv"
	"strings"
)

// Helper function to check if a shard with a specific index exists
func hasShardIndex(shards []sharding.Shard, index int) bool {
	for _, shard := range shards {
		if shard.Index == index {
			return true
		}
	}
	return false
}

func (n *P2PNode) updateShardMetadata(filename string, byteSize int64) {
	fmt.Println("Updating shards map")

	// Extract original file hash and shard index from filename
	parts := strings.Split(filepath.Base(filename), ".")
	if len(parts) != 2 {
		fmt.Printf("Invalid shard filename format: %s\n", filename)
		return
	}

	originalFile := parts[0]
	shardIndex := parts[1]

	// Convert string index to integer
	shardIdx, err := strconv.Atoi(shardIndex)
	if err != nil {
		fmt.Printf("Error converting shard index: %v\n", err)
		return
	}

	// Create shard information
	shard := sharding.Shard{
		Index: shardIdx,
		Hash:  filename,
		Size:  byteSize,
	}

	// Add to shards map
	n.shardMapMutex.Lock()
	shards, exists := n.shardMap[originalFile]
	if !exists {
		n.shardMap[originalFile] = make([]sharding.Shard, 0)
		shards = n.shardMap[originalFile]
	}
	if !hasShardIndex(shards, shardIdx) {
		n.shardMap[originalFile] = append(shards, shard)
	} 
	n.shardMapMutex.Unlock()

	fmt.Println("After Update")
	n.printShardsMap()

	fmt.Printf("Updated shards map for file %s with shard %s\n", originalFile, shardIndex)
}
