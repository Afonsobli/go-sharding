package node

import (
	"fmt"
	"os"
)

func (n *P2PNode) PrintShardsMap() {
	n.printShardsMap()
}

// TODO: This is not perfect as we are using goroutines which also print outputs
func (n *P2PNode) printShardsMap() {
	hostname, _ := os.Hostname()
	fmt.Printf("Shards map from %s ===========================\n", hostname)
	for shardHash, shards := range n.shardMap {
		fmt.Printf("💠 shard: %s +++++++++++++++++++++++\n", shardHash)
		for _, shardInfo := range shards {
			fmt.Println("\tshardInfo.Index:", shardInfo.Index)
			fmt.Println("\tshardInfo.Hash:", shardInfo.Hash)
			fmt.Println("\t---------------------------")
		}
	}
	fmt.Println("=====================================")
}
