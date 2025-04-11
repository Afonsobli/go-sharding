package node

import (
	"fmt"
	"shard/internal/sharding"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (n *P2PNode) DistributeFile(filePath string) {
	fmt.Println("Distributing file to peers")
	fmt.Println("len(n.peerAddrs):", len(n.peerAddrs))

	// Split the file into shards
	shards, err := sharding.SplitFile(filePath, n.shardsDir)
	if err != nil {
		fmt.Printf("Failed to split file: %v\n", err)
		return
	}

	n.shardMapMutex.Lock()
	// Store shard information
	n.shardMap[filePath] = shards
	n.shardMapMutex.Unlock()
	
	n.distributeShards(shards)
}

func (n *P2PNode) distributeShards(shards []sharding.Shard) {
	peerList := make([]peer.ID, 0, len(n.peerAddrs))
	if len(n.peerAddrs) == 0 {
		fmt.Println("No peers available to distribute shards")
		return
	}
	for peerID := range n.peerAddrs {
		peerList = append(peerList, peerID)
	}

	// Distribute each shard to different peers
	for i, shard := range shards {
		peerIndex := i % len(peerList)
		go func(s sharding.Shard, pid peer.ID) {
			err := n.sendShardToPeer(s.Hash, pid)
			if err != nil {
				fmt.Printf("Failed to send shard to peer %s: %v\n", pid, err)
				return
			}
			fmt.Printf("Successfully sent shard %d to peer %s\n", s.Index, pid)
		}(shard, peerList[peerIndex])
	}
}
