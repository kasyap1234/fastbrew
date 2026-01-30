package brew

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const FormulaAPIURL = "https://formulae.brew.sh/api/formula"

// RemoteFormula represents the full JSON response from formulae.brew.sh
type RemoteFormula struct {
	Name         string   `json:"name"`
	Desc         string   `json:"desc"`
	Homepage     string   `json:"homepage"`
	Versions     Versions `json:"versions"`
	Bottle       Bottle   `json:"bottle"`
	Dependencies []string `json:"dependencies"`
	KegOnly      bool     `json:"keg_only"`
}

type Versions struct {
	Stable string `json:"stable"`
}

type Bottle struct {
	Stable BottleStable `json:"stable"`
}

type BottleStable struct {
	RootURL string                `json:"root_url"`
	Files   map[string]BottleFile `json:"files"`
}

type BottleFile struct {
	Cellar string `json:"cellar"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// FetchFormula gets metadata for a single package
func (c *Client) FetchFormula(name string) (*RemoteFormula, error) {
	url := fmt.Sprintf("%s/%s.json", FormulaAPIURL, name)

	// Create client with timeout
	httpClient := &http.Client{Timeout: 10 * time.Second}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch formula %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("formula %q not found - try 'fastbrew search %s' to find the correct name (e.g., python@3.12 instead of python)", name, name)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api returned status %d for %s", resp.StatusCode, name)
	}

	var f RemoteFormula
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, fmt.Errorf("failed to parse formula json for %s: %w", name, err)
	}

	return &f, nil
}

// GetBottleInfo returns the URL and SHA256 for the current platform
func (f *RemoteFormula) GetBottleInfo() (string, string, error) {
	platform, err := GetPlatform()
	if err != nil {
		return "", "", err
	}

	file, ok := f.Bottle.Stable.Files[platform]
	if !ok {
		// Fallback to "all" if available (common for scripts/no-arch packages)
		if fileAll, okAll := f.Bottle.Stable.Files["all"]; okAll {
			return fileAll.URL, fileAll.SHA256, nil
		}

		return "", "", fmt.Errorf("no bottle available for platform %s", platform)
	}

	return file.URL, file.SHA256, nil
}
