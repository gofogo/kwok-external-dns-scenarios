// Package distribute provides weighted distribution of labels across a total count.
// Ported from sigs.k8s.io/external-dns/internal/testutils.distributeByWeight.
package distribute

import (
	"maps"
	"slices"
)

// Weights maps a category name to its relative share.
// Example: Weights{"headless": 1, "node-port": 1} splits 50/50.
// Example: Weights{"headless": 1, "node-port": 2} splits ~33/67.
type Weights map[string]int

// Distribute returns a slice of n category labels allocated proportionally to weights.
// Keys are sorted for determinism; remaining slots (due to integer rounding) go to the last key.
// Returns nil if weights is empty or n is zero.
func Distribute(n int, weights Weights) []string {
	if len(weights) == 0 || n == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(weights))
	total := 0
	for _, k := range keys {
		total += weights[k]
	}
	if total == 0 {
		return nil
	}
	result := make([]string, 0, n)
	for _, k := range keys {
		count := (weights[k] * n) / total
		for range count {
			result = append(result, k)
		}
	}
	// Fill remaining slots (integer rounding) with the last key.
	for len(result) < n {
		result = append(result, keys[len(keys)-1])
	}
	return result
}
