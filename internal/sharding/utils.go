package sharding

import "sort"

// TODO: use a better sorting algorithm
func SortShards(shards []Shard) []Shard {
	sorted := make([]Shard, len(shards))
	copy(sorted, shards)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Index < sorted[j].Index
	})
	return sorted
}
