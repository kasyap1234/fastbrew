package brew

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sahilm/fuzzy"
)

func TestPrefixIndex_BuildIndex(t *testing.T) {
	pi := NewPrefixIndex()

	items := []SearchItem{
		{Name: "python", Desc: "Python programming language", IsCask: false},
		{Name: "python3", Desc: "Python 3 programming language", IsCask: false},
		{Name: "pyenv", Desc: "Python version manager", IsCask: false},
		{Name: "pytorch", Desc: "Machine learning framework", IsCask: false},
		{Name: "node", Desc: "Node.js runtime", IsCask: false},
		{Name: "nodejs", Desc: "Node.js runtime", IsCask: false},
		{Name: "npm", Desc: "Node package manager", IsCask: false},
	}

	err := pi.BuildIndex(items)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	prefixCount, totalItems, avgBucket := pi.Stats()
	if totalItems != 7 {
		t.Errorf("Expected 7 items, got %d", totalItems)
	}
	if prefixCount == 0 {
		t.Error("Expected non-zero prefix count")
	}
	if avgBucket == 0 {
		t.Error("Expected non-zero average bucket size")
	}
}

func TestPrefixIndex_SearchPrefix(t *testing.T) {
	pi := NewPrefixIndex()

	items := []SearchItem{
		{Name: "python", Desc: "Python programming language", IsCask: false},
		{Name: "python3", Desc: "Python 3 programming language", IsCask: false},
		{Name: "pyenv", Desc: "Python version manager", IsCask: false},
		{Name: "pytorch", Desc: "Machine learning framework", IsCask: false},
		{Name: "node", Desc: "Node.js runtime", IsCask: false},
	}

	if err := pi.BuildIndex(items); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	tests := []struct {
		prefix   string
		expected int
	}{
		{"py", 4},
		{"pyt", 3},
		{"no", 1},
		{"xyz", 0},
		{"a", 5},
	}

	for _, tc := range tests {
		results := pi.SearchPrefix(tc.prefix)
		if len(results) != tc.expected {
			t.Errorf("SearchPrefix(%q): expected %d results, got %d", tc.prefix, tc.expected, len(results))
		}
	}
}

func TestPrefixIndex_SearchFuzzy(t *testing.T) {
	pi := NewPrefixIndex()

	items := []SearchItem{
		{Name: "python", Desc: "Python programming language", IsCask: false},
		{Name: "python3", Desc: "Python 3 programming language", IsCask: false},
		{Name: "pyenv", Desc: "Python version manager", IsCask: false},
		{Name: "node", Desc: "Node.js runtime", IsCask: false},
		{Name: "nodejs", Desc: "Node.js runtime", IsCask: false},
	}

	if err := pi.BuildIndex(items); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	tests := []struct {
		query         string
		minResults    int
		maxResults    int
		expectedFirst string
	}{
		{"python", 1, 2, "python"},
		{"py", 3, 4, ""},
		{"node", 1, 2, "node"},
		{"xyz", 0, 0, ""},
	}

	for _, tc := range tests {
		matches := pi.SearchFuzzy(tc.query)
		if len(matches) < tc.minResults || len(matches) > tc.maxResults {
			t.Errorf("SearchFuzzy(%q): expected %d-%d results, got %d", tc.query, tc.minResults, tc.maxResults, len(matches))
		}
		if tc.expectedFirst != "" && len(matches) > 0 {
			first := items[matches[0].Index].Name
			if first != tc.expectedFirst {
				t.Errorf("SearchFuzzy(%q): expected first result %q, got %q", tc.query, tc.expectedFirst, first)
			}
		}
	}
}

func TestPrefixIndex_SaveAndLoad(t *testing.T) {
	pi := NewPrefixIndex()

	items := []SearchItem{
		{Name: "python", Desc: "Python programming language", IsCask: false},
		{Name: "node", Desc: "Node.js runtime", IsCask: false},
		{Name: "go", Desc: "Go programming language", IsCask: false},
	}

	if err := pi.BuildIndex(items); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_prefix_index.gob")

	if err := pi.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	pi2 := NewPrefixIndex()
	if err := pi2.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	results := pi2.SearchPrefix("py")
	if len(results) != 1 || results[0].Name != "python" {
		t.Errorf("After load, SearchPrefix(py): expected [python], got %v", results)
	}

	matches := pi2.SearchFuzzy("python")
	if len(matches) != 1 || items[matches[0].Index].Name != "python" {
		t.Errorf("After load, SearchFuzzy(python): expected 1 result for python, got %v", matches)
	}
}

func TestPrefixIndex_ConcurrentAccess(t *testing.T) {
	pi := NewPrefixIndex()

	items := []SearchItem{
		{Name: "python", Desc: "Python programming language", IsCask: false},
		{Name: "node", Desc: "Node.js runtime", IsCask: false},
	}

	if err := pi.BuildIndex(items); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			pi.SearchPrefix("py")
			pi.SearchFuzzy("python")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			pi.SearchPrefix("no")
			pi.SearchFuzzy("node")
		}
		done <- true
	}()

	<-done
	<-done
}

func BenchmarkPrefixIndex_SearchFuzzy(b *testing.B) {
	pi := NewPrefixIndex()

	items := make([]SearchItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = SearchItem{
			Name:   fmt.Sprintf("package-%c%c", 'a'+i%26, 'a'+i/26%26),
			Desc:   fmt.Sprintf("Description for package %d", i),
			IsCask: i%2 == 0,
		}
	}

	if err := pi.BuildIndex(items); err != nil {
		b.Fatalf("BuildIndex failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pi.SearchFuzzy("pack")
	}
}

func BenchmarkPrefixIndex_SearchPrefix(b *testing.B) {
	pi := NewPrefixIndex()

	items := make([]SearchItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = SearchItem{
			Name:   fmt.Sprintf("package-%c%c", 'a'+i%26, 'a'+i/26%26),
			Desc:   fmt.Sprintf("Description for package %d", i),
			IsCask: i%2 == 0,
		}
	}

	if err := pi.BuildIndex(items); err != nil {
		b.Fatalf("BuildIndex failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pi.SearchPrefix("pa")
	}
}

func BenchmarkLinearSearch(b *testing.B) {
	items := make([]SearchItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = SearchItem{
			Name:   fmt.Sprintf("package-%c%c", 'a'+i%26, 'a'+i/26%26),
			Desc:   fmt.Sprintf("Description for package %d", i),
			IsCask: i%2 == 0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := make([]SearchItem, 0)
		for _, item := range items {
			if len(item.Name) >= 2 && item.Name[:2] == "pa" {
				results = append(results, item)
			}
		}
		_ = results
	}
}

func BenchmarkFullFuzzySearch(b *testing.B) {
	items := make([]SearchItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = SearchItem{
			Name:   fmt.Sprintf("package-%c%c", 'a'+i%26, 'a'+i/26%26),
			Desc:   fmt.Sprintf("Description for package %d", i),
			IsCask: i%2 == 0,
		}
	}

	source := searchSourceFromItems(items)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fuzzy.FindFrom("pack", source)
	}
}
