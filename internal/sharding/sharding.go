package sharding

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const ShardSize = 1 * 1024 * 1024 // 1MB per shard

type Shard struct {
	Index            int
	Hash             string
	Size             int64
}

// TODO: Refactor other parts of the code to use this function
// TODO: Create other functions to handle index and path
// ShardIndex returns the index of a shard from its path
func ShardIndex(shardPath string) (int, error) {
	fmt.Println("shardPath", shardPath)
	parts := strings.Split(shardPath, ".")
	fmt.Println("parts", parts)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid shard path: %s", shardPath)
	}
	var index int
	_, err := fmt.Sscanf(parts[len(parts)-1], "%d", &index)
	if err != nil {
		return 0, fmt.Errorf("invalid shard index: %v", err)
	}
	return index, nil
}

// SplitFile splits a file into multiple shards
func SplitFile(filePath string, shardsDir string) ([]Shard, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create shards directory if it doesn't exist
	err = os.MkdirAll(shardsDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create shards directory: %v", err)
	}

	var shards []Shard
	buffer := make([]byte, ShardSize)
	shardIndex := 0

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading file: %v", err)
		}

		shardPath := filepath.Join(shardsDir, fmt.Sprintf("%s.%d", filepath.Base(filePath), shardIndex))
		err = os.WriteFile(shardPath, buffer[:n], 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write shard: %v", err)
		}

		shards = append(shards, Shard{
			Index: shardIndex,
			Hash:  shardPath,
			Size:  int64(n),
		})
		shardIndex++
	}

	return shards, nil
}

// TODO: potentially has too many arguments
// MergeShards combines multiple shards back into the original file
func MergeShards(shards []Shard, outputDir, shardsDir, outputPath string) error {
	fmt.Println("Merging Shards...")
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}
	fmt.Println("Created outputDir", outputDir)

	outputPath = filepath.Join(outputDir, outputPath)
	fmt.Println("outputPath", outputPath)

    outFile, err := os.Create(outputPath)
    if err != nil {
        return fmt.Errorf("failed to create output file: %v", err)
    }
    defer outFile.Close()

	fmt.Println("shards", shards)
    for _, shard := range shards {
        shardFile, err := os.Open(filepath.Join(shardsDir, shard.Hash))
        if err != nil {
            return fmt.Errorf("failed to open shard %d: %v", shard.Index, err)
        }
        
        // Copy only the actual size of the shard
		fmt.Printf("Copying shard %d and size %d\n", shard.Index, shard.Size)
        _, err = io.CopyN(outFile, shardFile, shard.Size)
        shardFile.Close()
        if err != nil {
            return fmt.Errorf("failed to write shard %d: %v", shard.Index, err)
        }
    }

    return nil
}
