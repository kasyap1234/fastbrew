package brew

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fastbrew/internal/httpclient"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	FormulaAPI      = "https://formulae.brew.sh/api/formula.json"
	CaskAPI         = "https://formulae.brew.sh/api/cask.json"
	minCompressSize = 1024
)

// FormulaVersions matches the Homebrew API's "versions" object structure.
type FormulaVersions struct {
	Stable string `json:"stable"`
}

type Formula struct {
	Name         string          `json:"name"`
	Desc         string          `json:"desc"`
	Homepage     string          `json:"homepage"`
	Versions     FormulaVersions `json:"versions"`
	Installed    []interface{}   `json:"installed"`
	Dependencies []string        `json:"dependencies"`
}

type Cask struct {
	Token    string `json:"token"`
	Desc     string `json:"desc"`
	Homepage string `json:"homepage"`
	Version  string `json:"version"`
}

type Index struct {
	Formulae []Formula
	Casks    []Cask
}

type SearchItem struct {
	Name   string
	Desc   string
	IsCask bool
}

type indexCacheMetadata struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// IsCask checks if a package name is a cask by looking it up in the index
func (c *Client) IsCask(name string) (bool, error) {
	idx, err := c.LoadIndex()
	if err != nil {
		return false, err
	}
	for _, cask := range idx.Casks {
		if cask.Token == name {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) GetCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".fastbrew", "cache")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func (c *Client) cleanupOldUncompressedFiles(cacheDir string) {
	oldFiles := []string{"formula.json", "cask.json", "search.gob"}
	for _, file := range oldFiles {
		path := filepath.Join(cacheDir, file)
		if _, err := os.Stat(path); err == nil {
			zstPath := path + ".zst"
			if _, err := os.Stat(zstPath); err == nil {
				os.Remove(path)
				if c.Verbose {
					fmt.Printf("🧹 Cleaned up old uncompressed file: %s\n", file)
				}
			}
		}
	}
}

func compressFile(data []byte) ([]byte, error) {
	if len(data) < minCompressSize {
		return nil, fmt.Errorf("file too small to compress")
	}
	return compressWithPool(data), nil
}

func decompressFile(data []byte) ([]byte, error) {
	return decompressWithPool(data)
}

func (c *Client) LoadIndex() (*Index, error) {
	c.indexOnce.Do(func() {
		// Re-check inside Do in case another goroutine already loaded it.
		if c.index != nil {
			return
		}
		var formulae []Formula
		var casks []Cask
		var fErr, cErr error

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			formulae, fErr = c.loadFormulaIndexDirect()
		}()
		go func() {
			defer wg.Done()
			casks, cErr = c.loadCaskIndexDirect()
		}()
		wg.Wait()

		if fErr != nil {
			c.indexErr = fErr
			return
		}
		if cErr != nil {
			c.indexErr = cErr
			return
		}

		c.index = &Index{
			Formulae: formulae,
			Casks:    casks,
		}
	})
	if c.indexErr != nil {
		return nil, c.indexErr
	}
	return c.index, nil
}

func (c *Client) LoadFormulaIndex() ([]Formula, error) {
	// If a partial in-memory index exists with no formulae, load them from disk.
	if c.index != nil && len(c.index.Formulae) == 0 {
		formulae, err := c.loadFormulaIndexDirect()
		if err != nil {
			return nil, err
		}
		c.index.Formulae = formulae
		return formulae, nil
	}
	idx, err := c.LoadIndex()
	if err != nil {
		return nil, err
	}
	return idx.Formulae, nil
}

func (c *Client) LoadCaskIndex() ([]Cask, error) {
	// If a partial in-memory index exists with no casks, load them from disk.
	if c.index != nil && len(c.index.Casks) == 0 {
		casks, err := c.loadCaskIndexDirect()
		if err != nil {
			return nil, err
		}
		c.index.Casks = casks
		return casks, nil
	}
	idx, err := c.LoadIndex()
	if err != nil {
		return nil, err
	}
	return idx.Casks, nil
}

// loadFormulaIndexDirect reads and parses the formula index from disk (no caching).
func (c *Client) loadFormulaIndexDirect() ([]Formula, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}

	if err := c.ensureFreshFormulaJSON(); err != nil {
		return nil, err
	}

	fPath := filepath.Join(cacheDir, "formula.json.zst")
	var formulae []Formula
	if err := loadJSON(fPath, &formulae); err != nil {
		return nil, err
	}

	return formulae, nil
}

// loadCaskIndexDirect reads and parses the cask index from disk (no caching).
func (c *Client) loadCaskIndexDirect() ([]Cask, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}

	if err := c.ensureFreshCaskJSON(); err != nil {
		return nil, err
	}

	cPath := filepath.Join(cacheDir, "cask.json.zst")
	var casks []Cask
	if err := loadJSON(cPath, &casks); err != nil {
		return nil, err
	}

	return casks, nil
}

func (c *Client) ensureFreshFormulaJSON() error {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return err
	}

	fPath := filepath.Join(cacheDir, "formula.json.zst")
	if shouldUpdate(fPath) {
		if c.Verbose {
			fmt.Println("🔄 Updating Formula index...")
		}
		if _, err := c.downloadAndCompress(FormulaAPI, fPath, "Formula"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ensureFreshCaskJSON() error {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return err
	}

	cPath := filepath.Join(cacheDir, "cask.json.zst")
	if shouldUpdate(cPath) {
		if c.Verbose {
			fmt.Println("🔄 Updating Cask index...")
		}
		if _, err := c.downloadAndCompress(CaskAPI, cPath, "Cask"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) LoadIndexLegacy() (*Index, error) {
	c.indexOnce.Do(func() {
		if err := c.EnsureFreshJSONs(); err != nil {
			c.indexErr = err
			return
		}
		c.index, c.indexErr = c.LoadRawIndex()
	})
	if c.indexErr != nil {
		return nil, c.indexErr
	}
	return c.index, nil
}

func (c *Client) LoadRawIndex() (*Index, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}

	c.cleanupOldUncompressedFiles(cacheDir)

	fPath := filepath.Join(cacheDir, "formula.json.zst")
	cPath := filepath.Join(cacheDir, "cask.json.zst")

	var idx Index
	if err := loadJSON(fPath, &idx.Formulae); err != nil {
		return nil, err
	}
	_ = loadJSON(cPath, &idx.Casks)

	return &idx, nil
}

func (c *Client) ForceRefreshIndex() (bool, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return false, err
	}

	fmt.Println("🔄 Refreshing package index...")

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	changedCh := make(chan bool, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		changed, err := c.downloadAndCompress(FormulaAPI, filepath.Join(cacheDir, "formula.json.zst"), "Formula")
		if err != nil {
			errCh <- err
			return
		}
		changedCh <- changed
	}()
	go func() {
		defer wg.Done()
		changed, err := c.downloadAndCompress(CaskAPI, filepath.Join(cacheDir, "cask.json.zst"), "Cask")
		if err != nil {
			errCh <- err
			return
		}
		changedCh <- changed
	}()
	wg.Wait()
	close(errCh)
	close(changedCh)

	if len(errCh) > 0 {
		return false, <-errCh
	}

	anyChanged := false
	for changed := range changedCh {
		if changed {
			anyChanged = true
		}
	}

	if !anyChanged {
		return false, nil
	}

	os.Remove(filepath.Join(cacheDir, "search.gob.zst"))
	os.Remove(filepath.Join(cacheDir, "prefix_index.gob"))

	c.prefixIndex = nil
	c.index = nil
	c.indexErr = nil
	c.indexOnce = sync.Once{}
	c.prefixIndexOnce = sync.Once{}
	c.notifyInvalidation(EventIndexRefreshed)

	return true, nil
}

func (c *Client) EnsureFreshJSONs() error {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return err
	}

	fPath := filepath.Join(cacheDir, "formula.json.zst")
	cPath := filepath.Join(cacheDir, "cask.json.zst")

	if shouldUpdate(fPath) {
		if c.Verbose {
			fmt.Println("🔄 Updating Formula index...")
		}
		if _, err := c.downloadAndCompress(FormulaAPI, fPath, "Formula"); err != nil {
			return err
		}
	}
	if shouldUpdate(cPath) {
		if c.Verbose {
			fmt.Println("🔄 Updating Cask index...")
		}
		if _, err := c.downloadAndCompress(CaskAPI, cPath, "Cask"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) downloadAndCompress(url, path, label string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	metaPath := path + ".meta.json"
	meta := loadIndexCacheMetadata(metaPath)
	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	if meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", meta.LastModified)
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if c.Verbose {
			fmt.Printf("✅ %s index is already up-to-date\n", label)
		}
		meta.ETag = coalesceHeader(resp.Header.Get("ETag"), meta.ETag)
		meta.LastModified = coalesceHeader(resp.Header.Get("Last-Modified"), meta.LastModified)
		if err := saveIndexCacheMetadata(metaPath, meta); err != nil && c.Verbose {
			fmt.Printf("⚠️  Failed to save %s index metadata: %v\n", label, err)
		}
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to fetch %s index: %s", label, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %w", err)
	}

	if existingData, err := readCachedIndexData(path); err == nil && bytes.Equal(existingData, data) {
		if c.Verbose {
			fmt.Printf("✅ %s index unchanged, skipping write\n", label)
		}
		meta.ETag = coalesceHeader(resp.Header.Get("ETag"), meta.ETag)
		meta.LastModified = coalesceHeader(resp.Header.Get("Last-Modified"), meta.LastModified)
		if err := saveIndexCacheMetadata(metaPath, meta); err != nil && c.Verbose {
			fmt.Printf("⚠️  Failed to save %s index metadata: %v\n", label, err)
		}
		return false, nil
	}

	originalSize := len(data)

	compressed, err := compressFile(data)
	if err != nil {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return false, fmt.Errorf("failed to write file: %w", err)
		}
		if c.Verbose {
			fmt.Printf("⚠️  %s index stored uncompressed (%d bytes)\n", label, originalSize)
		}
	} else {
		if err := os.WriteFile(path, compressed, 0644); err != nil {
			return false, fmt.Errorf("failed to write compressed file: %w", err)
		}

		if c.Verbose {
			ratio := float64(originalSize-len(compressed)) / float64(originalSize) * 100
			fmt.Printf("✅ %s index compressed: %d → %d bytes (%.1f%% reduction)\n",
				label, originalSize, len(compressed), ratio)
		}
	}

	meta.ETag = coalesceHeader(resp.Header.Get("ETag"), meta.ETag)
	meta.LastModified = coalesceHeader(resp.Header.Get("Last-Modified"), meta.LastModified)
	if err := saveIndexCacheMetadata(metaPath, meta); err != nil && c.Verbose {
		fmt.Printf("⚠️  Failed to save %s index metadata: %v\n", label, err)
	}

	return true, nil
}

func (c *Client) GetSearchIndex() ([]SearchItem, error) {
	if err := c.EnsureFreshJSONs(); err != nil {
		return nil, err
	}

	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}
	gobPath := filepath.Join(cacheDir, "search.gob.zst")
	prefixIndexPath := filepath.Join(cacheDir, "prefix_index.gob")
	fPath := filepath.Join(cacheDir, "formula.json.zst")
	cPath := filepath.Join(cacheDir, "cask.json.zst")

	if isFreshAgainst(gobPath, fPath, cPath) {
		if items, loadErr := loadSearchItemsFromGob(gobPath); loadErr == nil {
			if !isFreshAgainst(prefixIndexPath, fPath, cPath) {
				prefixIdx := NewPrefixIndex()
				if buildErr := prefixIdx.BuildIndex(items); buildErr == nil {
					if saveErr := prefixIdx.Save(prefixIndexPath); saveErr != nil && c.Verbose {
						fmt.Printf("⚠️  Failed to save prefix index: %v\n", saveErr)
					}
				}
			}
			return items, nil
		}
	}

	idx, err := c.LoadRawIndex()
	if err != nil {
		return nil, err
	}

	items := make([]SearchItem, 0, len(idx.Formulae)+len(idx.Casks))
	for _, f := range idx.Formulae {
		items = append(items, SearchItem{Name: f.Name, Desc: f.Desc, IsCask: false})
	}
	for _, cask := range idx.Casks {
		items = append(items, SearchItem{Name: cask.Token, Desc: cask.Desc, IsCask: true})
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(items); err == nil {
		gobData := buf.Bytes()
		if compressed, err := compressFile(gobData); err == nil {
			if err := os.WriteFile(gobPath, compressed, 0644); err == nil && c.Verbose {
				ratio := float64(len(gobData)-len(compressed)) / float64(len(gobData)) * 100
				fmt.Printf("✅ Search index compressed: %d → %d bytes (%.1f%% reduction)\n",
					len(gobData), len(compressed), ratio)
			}
		} else {
			os.WriteFile(gobPath, gobData, 0644)
		}
	}

	prefixIdx := NewPrefixIndex()
	if err := prefixIdx.BuildIndex(items); err == nil {
		if err := prefixIdx.Save(prefixIndexPath); err == nil && c.Verbose {
			prefixCount, totalItems, avgBucket := prefixIdx.Stats()
			fmt.Printf("✅ Prefix index built: %d prefixes, %d items, avg bucket %.1f\n",
				prefixCount, totalItems, avgBucket)
		}
	}

	return items, nil
}

func (c *Client) GetPrefixIndex() (*PrefixIndex, error) {
	var err error
	c.prefixIndexOnce.Do(func() {
		cacheDir, cacheErr := c.GetCacheDir()
		if cacheErr != nil {
			err = cacheErr
			return
		}
		prefixIndexPath := filepath.Join(cacheDir, "prefix_index.gob")
		fPath := filepath.Join(cacheDir, "formula.json.zst")
		cPath := filepath.Join(cacheDir, "cask.json.zst")

		c.prefixIndex = NewPrefixIndex()

		if isFreshAgainst(prefixIndexPath, fPath, cPath) {
			if loadErr := c.prefixIndex.Load(prefixIndexPath); loadErr == nil {
				if c.Verbose {
					prefixCount, totalItems, avgBucket := c.prefixIndex.Stats()
					fmt.Printf("✅ Prefix index loaded: %d prefixes, %d items, avg bucket %.1f\n",
						prefixCount, totalItems, avgBucket)
				}
				return
			}
		}

		items, getErr := c.GetSearchIndex()
		if getErr != nil {
			err = getErr
			return
		}

		if buildErr := c.prefixIndex.BuildIndex(items); buildErr != nil {
			err = buildErr
			return
		}

		if saveErr := c.prefixIndex.Save(prefixIndexPath); saveErr != nil && c.Verbose {
			fmt.Printf("⚠️  Failed to save prefix index: %v\n", saveErr)
		}
	})

	if err != nil {
		return nil, err
	}

	return c.prefixIndex, nil
}

func (c *Client) SearchFuzzyWithIndex(query string) ([]SearchItem, error) {
	prefixIdx, err := c.GetPrefixIndex()
	if err != nil {
		return nil, err
	}

	matches := prefixIdx.SearchFuzzy(query)
	if matches == nil {
		return []SearchItem{}, nil
	}

	items := prefixIdx.GetItems()
	result := make([]SearchItem, len(matches))
	for i, match := range matches {
		result[i] = items[match.Index]
	}

	return result, nil
}

func isFresh(target, source string) bool {
	tInfo, err := os.Stat(target)
	if err != nil {
		return false
	}
	sInfo, err := os.Stat(source)
	if err != nil {
		return false
	}
	return tInfo.ModTime().After(sInfo.ModTime())
}

func shouldUpdate(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > 24*time.Hour
}

func isFreshAgainst(target string, sources ...string) bool {
	for _, source := range sources {
		if !isFresh(target, source) {
			return false
		}
	}
	return true
}

func loadSearchItemsFromGob(path string) ([]SearchItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	decompressed, err := decompressFile(data)
	if err != nil {
		decompressed = data
	}

	var items []SearchItem
	if err := gob.NewDecoder(bytes.NewReader(decompressed)).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func readCachedIndexData(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decompressed, err := decompressFile(data)
	if err == nil {
		return decompressed, nil
	}
	return data, nil
}

func loadIndexCacheMetadata(path string) indexCacheMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return indexCacheMetadata{}
	}

	var meta indexCacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return indexCacheMetadata{}
	}
	return meta
}

func saveIndexCacheMetadata(path string, meta indexCacheMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func coalesceHeader(newValue, fallback string) string {
	if newValue != "" {
		return newValue
	}
	return fallback
}

func loadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	decompressed, err := decompressFile(data)
	if err != nil {
		decompressed = data
	}

	return json.Unmarshal(decompressed, v)
}

func downloadFile(url, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
