package types

type Node interface {
	DistributeFile(filePath string)
	RequestFileFromPeers(hash string) error
	Close() error
}
