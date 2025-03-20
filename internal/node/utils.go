package node

import "fmt"

// TODO: This is not perfect as we are using goroutines which also print outputs
func (n P2PNode) printShardsMap() {
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
