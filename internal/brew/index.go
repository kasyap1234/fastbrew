package brew

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
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

type Formula struct {
	Name         string        `json:"name"`
	Desc         string        `json:"desc"`
	Homepage     string        `json:"homepage"`
	Version      string        `json:"version"`
	Installed    []interface{} `json:"installed"`
	Dependencies []string      `json:"dependencies"`
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
	if err := c.EnsureFreshJSONs(); err != nil {
		return nil, err
	}
	return c.LoadRawIndex()
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

func (c *Client) ForceRefreshIndex() error {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return err
	}

	fmt.Println("🔄 Refreshing package index...")

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := c.downloadAndCompress(FormulaAPI, filepath.Join(cacheDir, "formula.json.zst"), "Formula"); err != nil {
			errCh <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := c.downloadAndCompress(CaskAPI, filepath.Join(cacheDir, "cask.json.zst"), "Cask"); err != nil {
			errCh <- err
		}
	}()
	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return <-errCh
	}

	os.Remove(filepath.Join(cacheDir, "search.gob.zst"))
	os.Remove(filepath.Join(cacheDir, "prefix_index.gob"))
	c.prefixIndex = nil
	c.indexOnce = sync.Once{}
	if _, err := c.GetSearchIndex(); err != nil {
		return fmt.Errorf("failed to rebuild search index: %w", err)
	}

	return nil
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
		if err := c.downloadAndCompress(FormulaAPI, fPath, "Formula"); err != nil {
			return err
		}
	}
	if shouldUpdate(cPath) {
		if c.Verbose {
			fmt.Println("🔄 Updating Cask index...")
		}
		if err := c.downloadAndCompress(CaskAPI, cPath, "Cask"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) downloadAndCompress(url, path, label string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	originalSize := len(data)

	compressed, err := compressFile(data)
	if err != nil {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		if c.Verbose {
			fmt.Printf("⚠️  %s index stored uncompressed (%d bytes)\n", label, originalSize)
		}
		return nil
	}

	if err := os.WriteFile(path, compressed, 0644); err != nil {
		return fmt.Errorf("failed to write compressed file: %w", err)
	}

	if c.Verbose {
		ratio := float64(originalSize-len(compressed)) / float64(originalSize) * 100
		fmt.Printf("✅ %s index compressed: %d → %d bytes (%.1f%% reduction)\n",
			label, originalSize, len(compressed), ratio)
	}

	return nil
}

func (c *Client) GetSearchIndex() ([]SearchItem, error) {
	if err := c.EnsureFreshJSONs(); err != nil {
		return nil, err
	}

	cacheDir, _ := c.GetCacheDir()
	gobPath := filepath.Join(cacheDir, "search.gob.zst")
	prefixIndexPath := filepath.Join(cacheDir, "prefix_index.gob")
	fPath := filepath.Join(cacheDir, "formula.json.zst")

	if isFresh(gobPath, fPath) && isFresh(prefixIndexPath, fPath) {
		data, err := os.ReadFile(gobPath)
		if err == nil {
			decompressed, err := decompressFile(data)
			if err == nil {
				var items []SearchItem
				if err := gob.NewDecoder(bytes.NewReader(decompressed)).Decode(&items); err == nil {
					return items, nil
				}
			}
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
	c.indexOnce.Do(func() {
		cacheDir, _ := c.GetCacheDir()
		prefixIndexPath := filepath.Join(cacheDir, "prefix_index.gob")
		fPath := filepath.Join(cacheDir, "formula.json.zst")

		c.prefixIndex = NewPrefixIndex()

		if isFresh(prefixIndexPath, fPath) {
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
	if os.IsNotExist(err) {
		return true
	}
	return time.Since(info.ModTime()) > 24*time.Hour
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
	resp, err := http.Get(url)
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
