package brew

import (
	"context"
	"encoding/json"
	"fastbrew/internal/httpclient"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

var macOSFallbackOrder = []string{"tahoe", "sequoia", "sonoma", "ventura", "monterey", "big_sur"}

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

	// Use shared HTTP client with request-specific timeout via context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", name, err)
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
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

	if file, ok := f.Bottle.Stable.Files[platform]; ok {
		return file.URL, file.SHA256, nil
	}

	var candidates []string
	if strings.HasPrefix(platform, "arm64_") {
		version := strings.TrimPrefix(platform, "arm64_")
		for i, v := range macOSFallbackOrder {
			if v == version {
				for _, older := range macOSFallbackOrder[i+1:] {
					candidates = append(candidates, "arm64_"+older)
				}
				break
			}
		}
	} else {
		for i, v := range macOSFallbackOrder {
			if v == platform {
				candidates = append(candidates, macOSFallbackOrder[i+1:]...)
				break
			}
		}
	}

	for _, candidate := range candidates {
		if file, ok := f.Bottle.Stable.Files[candidate]; ok {
			return file.URL, file.SHA256, nil
		}
	}

	if file, ok := f.Bottle.Stable.Files["all"]; ok {
		return file.URL, file.SHA256, nil
	}

	available := make([]string, 0, len(f.Bottle.Stable.Files))
	for k := range f.Bottle.Stable.Files {
		available = append(available, k)
	}
	sort.Strings(available)
	return "", "", fmt.Errorf("no bottle available for platform %s (available: %s)", platform, strings.Join(available, ", "))
}
