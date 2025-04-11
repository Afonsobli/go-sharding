package types

type Node interface {
	DistributeFile(filePath string)
	RequestFileFromPeers(hash string) error
	PrintShardsMap()
	Close() error
}
