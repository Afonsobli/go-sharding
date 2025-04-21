package sharding

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const ShardSize = 1 * 1024 * 1024 // 1MB per shard

type Shard struct {
	Index int
	Hash  string
	Size  int64
}

// ShardContext contains all data needed for shard processing
type ShardContext struct {
	File      *os.File
	FilePath  string
	ShardsDir string
	FileSize  int64
	NumShards int64
	Shards    []Shard
}

// ShardJob represents a single shard processing job
type ShardJob struct {
	Ctx      *ShardContext
	ShardIdx int64
}

// MergeContext contains parameters for merging shards
type MergeContext struct {
	Shards     []Shard
	OutputDir  string
	ShardsDir  string
	OutputPath string
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

	// Get file size to pre-allocate resources
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}
	fileSize := fileInfo.Size()
	numShards := calculateNumberOfShards(fileSize)

	// Pre-allocate shards slice
	shards := make([]Shard, numShards)

	ctx := &ShardContext{
		File:      file,
		FilePath:  filePath,
		ShardsDir: shardsDir,
		FileSize:  fileSize,
		NumShards: numShards,
		Shards:    shards,
	}

	err = processShards(ctx)
	if err != nil {
		return nil, err
	}

	return shards, nil
}

// calculateNumberOfShards determines how many shards are needed for the given file size
func calculateNumberOfShards(fileSize int64) int64 {
	// Round up division
	return (fileSize + ShardSize - 1) / ShardSize
}

// processShards handles the parallel processing of file shards
func processShards(ctx *ShardContext) error {
	var wg sync.WaitGroup
	errChan := make(chan error, ctx.NumShards)

	// Create a worker pool for parallel shard writing
	for shardIndex := int64(0); shardIndex < ctx.NumShards; shardIndex++ {
		wg.Add(1)
		job := &ShardJob{
			Ctx:      ctx,
			ShardIdx: shardIndex,
		}
		go func(j *ShardJob) {
			defer wg.Done()
			err := processOneShard(j)
			if err != nil {
				errChan <- err
			}
		}(job)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// processOneShard handles the reading and writing of a single shard
func processOneShard(job *ShardJob) error {
	ctx := job.Ctx
	idx := job.ShardIdx

	offset := idx * ShardSize
	currentShardSize := calculateShardSize(offset, ctx.FileSize)

	buffer := make([]byte, currentShardSize)

	err := readFileSegment(ctx.File, buffer, offset, currentShardSize, idx)
	if err != nil {
		return err
	}

	shardHash := buildShardHash(ctx.FilePath, idx)
	shardPath := filepath.Join(ctx.ShardsDir, shardHash)
	err = os.WriteFile(shardPath, buffer, 0644)
	if err != nil {
		return fmt.Errorf("failed to write shard %d: %v", idx, err)
	}

	ctx.Shards[idx] = Shard{
		Index: int(idx),
		Hash:  shardHash,
		Size:  currentShardSize,
	}

	return nil
}

// calculateShardSize determines the size of a particular shard
func calculateShardSize(offset int64, fileSize int64) int64 {
	if offset+ShardSize > fileSize {
		return fileSize - offset
	}
	return ShardSize
}

// readFileSegment reads a segment of the file into a buffer
func readFileSegment(file *os.File, buffer []byte, offset int64, size int64, idx int64) error {
	segReader := io.NewSectionReader(file, offset, size)
	_, err := io.ReadFull(segReader, buffer)
	if err != nil {
		return fmt.Errorf("error reading file segment %d: %v", idx, err)
	}
	return nil
}

// buildShardHash constructs the full hash for a shard file
func buildShardHash(filePath string, idx int64) string {
	return fmt.Sprintf("%s.%d", filepath.Base(filePath), idx)
}

// TODO: potentially has too many arguments
// MergeShards combines multiple shards back into the original file
func MergeShards(sortedShards []Shard, outputDir, shardsDir, outputPath string) error {
	fmt.Println("Merging Shards...")

	// Create merge context to hold all relevant data
	ctx := &MergeContext{
		Shards:     sortedShards,
		OutputDir:  outputDir,
		ShardsDir:  shardsDir,
		OutputPath: outputPath,
	}

	// Prepare output directory and file
	outFile, err := prepareOutputFile(ctx)
	if err != nil {
		return err
	}
	defer outFile.Close()

	shardBuffers, err := loadShardContents(sortedShards, ctx.ShardsDir)
	if err != nil {
		return err
	}

	// Write the shards to the output file
	return writeShardBuffers(outFile, sortedShards, shardBuffers)
}

// prepareOutputFile creates the output directory and file
func prepareOutputFile(ctx *MergeContext) (*os.File, error) {
	err := os.MkdirAll(ctx.OutputDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create output directory: %v", err)
	}
	fmt.Println("Created outputDir", ctx.OutputDir)

	fullOutputPath := filepath.Join(ctx.OutputDir, ctx.OutputPath)
	fmt.Println("outputPath", fullOutputPath)

	outFile, err := os.Create(fullOutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %v", err)
	}

	return outFile, nil
}

// loadShardContents reads all shard contents into memory in parallel
func loadShardContents(shards []Shard, shardsDir string) ([][]byte, error) {
	var wg sync.WaitGroup
	shardBuffers := make([][]byte, len(shards))
	errChan := make(chan error, len(shards))

	for i, shard := range shards {
		wg.Add(1)
		go func(idx int, s Shard) {
			defer wg.Done()
			// Read shard contents into memory
			shardPath := filepath.Join(shardsDir, s.Hash)
			buffer, err := os.ReadFile(shardPath)
			if err != nil {
				errChan <- fmt.Errorf("failed to read shard %d: %v", s.Index, err)
				return
			}
			shardBuffers[idx] = buffer
		}(i, shard)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return shardBuffers, nil
}

// writeShardBuffers writes the shard buffers to the output file in order
func writeShardBuffers(outFile *os.File, shards []Shard, shardBuffers [][]byte) error {
	for i, shard := range shards {
		fmt.Printf("Writing shard %d and size %d\n", shard.Index, shard.Size)
		_, err := outFile.Write(shardBuffers[i])
		if err != nil {
			return fmt.Errorf("failed to write shard %d: %v", shard.Index, err)
		}
	}
	return nil
}
