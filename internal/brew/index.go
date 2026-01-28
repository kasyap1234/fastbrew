package brew

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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



// Custom unmarshal might be needed if version is nested, but for search list, name/desc is key.

type Cask struct {
	Token       string `json:"token"`
	Desc        string `json:"desc"`
	Homepage    string `json:"homepage"`
	Version     string `json:"version"`
}

type Index struct {
	Formulae []Formula
	Casks    []Cask
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

func (c *Client) LoadIndex() (*Index, error) {
	cacheDir, err := c.GetCacheDir()
	if err != nil {
		return nil, err
	}

	fPath := filepath.Join(cacheDir, "formula.json")
	cPath := filepath.Join(cacheDir, "cask.json")

	// Check if we need to update (older than 24h or missing)
	if shouldUpdate(fPath) {
		fmt.Println("🔄 Updating Formula index...") // This might mess up TUI if called inside. Should be separate.
		if err := downloadFile(FormulaAPI, fPath); err != nil {
			return nil, err
		}
	}
	if shouldUpdate(cPath) {
		fmt.Println("🔄 Updating Cask index...")
		if err := downloadFile(CaskAPI, cPath); err != nil {
			return nil, err
		}
	}

	// Load
	var idx Index
	if err := loadJSON(fPath, &idx.Formulae); err != nil {
		return nil, err
	}
	if err := loadJSON(cPath, &idx.Casks); err != nil {
		// Casks might fail if on Linux without Cask support or just optional
		// ignore error or log
	}

	return &idx, nil
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
