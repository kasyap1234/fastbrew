package brew

import (
	"encoding/gob"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sahilm/fuzzy"
)

const (
	minPrefixLength    = 2
	maxPrefixLength    = 3
	prefixIndexVersion = 1
)

type PrefixIndex struct {
	prefixes   map[string][]int
	items      []SearchItem
	version    int
	totalItems int
	mu         sync.RWMutex
}

type prefixIndexData struct {
	Prefixes   map[string][]int
	Items      []SearchItem
	Version    int
	TotalItems int
}

func NewPrefixIndex() *PrefixIndex {
	return &PrefixIndex{
		prefixes: make(map[string][]int),
		version:  prefixIndexVersion,
	}
}

func (pi *PrefixIndex) BuildIndex(items []SearchItem) error {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	pi.items = items
	pi.totalItems = len(items)
	pi.prefixes = make(map[string][]int)

	for idx, item := range items {
		name := strings.ToLower(item.Name)

		for length := minPrefixLength; length <= maxPrefixLength && length <= len(name); length++ {
			for i := 0; i <= len(name)-length; i++ {
				prefix := name[i : i+length]
				pi.prefixes[prefix] = append(pi.prefixes[prefix], idx)
			}
		}
	}

	for prefix := range pi.prefixes {
		indices := pi.prefixes[prefix]
		seen := make(map[int]bool, len(indices))
		unique := make([]int, 0, len(indices))
		for _, idx := range indices {
			if !seen[idx] {
				seen[idx] = true
				unique = append(unique, idx)
			}
		}
		pi.prefixes[prefix] = unique
	}

	return nil
}

func (pi *PrefixIndex) SearchPrefix(prefix string) []SearchItem {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if len(prefix) < minPrefixLength {
		return pi.getAllItems()
	}

	prefix = strings.ToLower(prefix)

	if len(prefix) > maxPrefixLength {
		searchPrefix := prefix[:maxPrefixLength]
		candidates := pi.prefixes[searchPrefix]
		return pi.filterByPrefix(candidates, prefix)
	}

	indices := pi.prefixes[prefix]
	result := make([]SearchItem, len(indices))
	for i, idx := range indices {
		result[i] = pi.items[idx]
	}
	return result
}

func (pi *PrefixIndex) SearchFuzzy(query string) []fuzzy.Match {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if len(query) < minPrefixLength {
		source := searchSourceFromItems(pi.items)
		return fuzzy.FindFrom(query, source)
	}

	searchPrefix := strings.ToLower(query)
	if len(searchPrefix) > maxPrefixLength {
		searchPrefix = searchPrefix[:maxPrefixLength]
	}

	candidateIndices := pi.prefixes[searchPrefix]
	if len(candidateIndices) == 0 {
		return nil
	}

	candidates := make([]SearchItem, len(candidateIndices))
	for i, idx := range candidateIndices {
		candidates[i] = pi.items[idx]
	}

	source := searchSourceFromItems(candidates)
	matches := fuzzy.FindFrom(query, source)

	for i := range matches {
		matches[i].Index = candidateIndices[matches[i].Index]
	}

	return matches
}

func (pi *PrefixIndex) GetItems() []SearchItem {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.getAllItems()
}

func (pi *PrefixIndex) getAllItems() []SearchItem {
	result := make([]SearchItem, len(pi.items))
	copy(result, pi.items)
	return result
}

func (pi *PrefixIndex) filterByPrefix(indices []int, prefix string) []SearchItem {
	result := make([]SearchItem, 0, len(indices))
	for _, idx := range indices {
		item := pi.items[idx]
		if strings.HasPrefix(strings.ToLower(item.Name), prefix) ||
			strings.Contains(strings.ToLower(item.Name), prefix) {
			result = append(result, item)
		}
	}
	return result
}

func (pi *PrefixIndex) Save(path string) error {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	data := prefixIndexData{
		Prefixes:   pi.prefixes,
		Items:      pi.items,
		Version:    pi.version,
		TotalItems: pi.totalItems,
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create prefix index file: %w", err)
	}
	defer file.Close()

	if err := gob.NewEncoder(file).Encode(&data); err != nil {
		return fmt.Errorf("failed to encode prefix index: %w", err)
	}

	return nil
}

func (pi *PrefixIndex) Load(path string) error {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open prefix index file: %w", err)
	}
	defer file.Close()

	var data prefixIndexData
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode prefix index: %w", err)
	}

	if data.Version != prefixIndexVersion {
		return fmt.Errorf("prefix index version mismatch: got %d, expected %d", data.Version, prefixIndexVersion)
	}

	pi.prefixes = data.Prefixes
	pi.items = data.Items
	pi.version = data.Version
	pi.totalItems = data.TotalItems

	return nil
}

func (pi *PrefixIndex) Stats() (prefixCount int, totalItems int, avgBucketSize float64) {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	prefixCount = len(pi.prefixes)
	totalItems = pi.totalItems

	if prefixCount > 0 {
		totalIndices := 0
		for _, indices := range pi.prefixes {
			totalIndices += len(indices)
		}
		avgBucketSize = float64(totalIndices) / float64(prefixCount)
	}

	return
}

type prefixSearchSource struct {
	items []SearchItem
}

func (s prefixSearchSource) String(i int) string {
	return s.items[i].Name + " " + s.items[i].Desc
}

func (s prefixSearchSource) Len() int {
	return len(s.items)
}

func searchSourceFromItems(items []SearchItem) prefixSearchSource {
	return prefixSearchSource{items: items}
}
