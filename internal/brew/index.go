package brew

import (
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
	FormulaAPI = "https://formulae.brew.sh/api/formula.json"
	CaskAPI    = "https://formulae.brew.sh/api/cask.json"
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

// LoadIndex ensures JSON files are present (downloads if needed) and parses them.
func (c *Client) LoadIndex() (*Index, error) {
	if err := c.EnsureFreshJSONs(); err != nil {
		return nil, err
	}
	return c.LoadRawIndex()
}

// LoadRawIndex just parses the JSON files from disk
func (c *Client) LoadRawIndex() (*Index, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}
	fPath := filepath.Join(cacheDir, "formula.json")
	cPath := filepath.Join(cacheDir, "cask.json")

	var idx Index
	if err := loadJSON(fPath, &idx.Formulae); err != nil {
		return nil, err
	}
	// Casks are optional
	_ = loadJSON(cPath, &idx.Casks)

	return &idx, nil
}

// ForceRefreshIndex downloads fresh JSON files, regardless of cache age
func (c *Client) ForceRefreshIndex() error {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return err
	}

	fmt.Println("🔄 Refreshing package index...")

	// Download in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := downloadFile(FormulaAPI, filepath.Join(cacheDir, "formula.json")); err != nil {
			errCh <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := downloadFile(CaskAPI, filepath.Join(cacheDir, "cask.json")); err != nil {
			errCh <- err
		}
	}()
	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return <-errCh
	}

	// Rebuild GOB cache key by calling GetSearchIndex
	// We delete the gob first to force rebuild
	os.Remove(filepath.Join(cacheDir, "search.gob"))
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

	fPath := filepath.Join(cacheDir, "formula.json")
	cPath := filepath.Join(cacheDir, "cask.json")

	if shouldUpdate(fPath) {
		fmt.Println("🔄 Updating Formula index...")
		if err := downloadFile(FormulaAPI, fPath); err != nil {
			return err
		}
	}
	if shouldUpdate(cPath) {
		fmt.Println("🔄 Updating Cask index...")
		if err := downloadFile(CaskAPI, cPath); err != nil {
			return err
		}
	}
	return nil
}

// GetSearchIndex returns a simplified index, using a cached GOB file if available and fresh.
func (c *Client) GetSearchIndex() ([]SearchItem, error) {
	if err := c.EnsureFreshJSONs(); err != nil {
		return nil, err
	}

	cacheDir, _ := c.GetCacheDir()
	gobPath := filepath.Join(cacheDir, "search.gob")
	fPath := filepath.Join(cacheDir, "formula.json")

	// Fast path: load from gob
	if isFresh(gobPath, fPath) {
		f, err := os.Open(gobPath)
		if err == nil {
			defer f.Close()
			var items []SearchItem
			if err := gob.NewDecoder(f).Decode(&items); err == nil {
				return items, nil
			}
		}
	}

	// Slow path: parse JSON and build gob
	// We do this if gob is missing or stale

	idx, err := c.LoadRawIndex()
	if err != nil {
		return nil, err
	}

	items := make([]SearchItem, 0, len(idx.Formulae)+len(idx.Casks))
	for _, f := range idx.Formulae {
		items = append(items, SearchItem{Name: f.Name, Desc: f.Desc, IsCask: false})
	}
	for _, c := range idx.Casks {
		items = append(items, SearchItem{Name: c.Token, Desc: c.Desc, IsCask: true})
	}

	// Save GOB
	f, err := os.Create(gobPath)
	if err == nil {
		defer f.Close()
		gob.NewEncoder(f).Encode(items)
	}

	return items, nil
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

func loadJSON(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}
