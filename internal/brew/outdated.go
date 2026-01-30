package brew

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OutdatedPackage represents an outdated package with version info
type OutdatedPackage struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	NewVersion     string `json:"new_version"`
	IsCask         bool   `json:"is_cask"`
}

const CaskAPIURL = "https://formulae.brew.sh/api/cask"

// RemoteCask represents the full JSON response from formulae.brew.sh for casks
type RemoteCask struct {
	Token   string `json:"token"`
	Version string `json:"version"`
}

// FetchCask gets metadata for a single cask
func (c *Client) FetchCask(name string) (*RemoteCask, error) {
	url := fmt.Sprintf("%s/%s.json", CaskAPIURL, name)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cask %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("cask %s not found on API", name)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api returned status %d for cask %s", resp.StatusCode, name)
	}

	var ck RemoteCask
	if err := json.NewDecoder(resp.Body).Decode(&ck); err != nil {
		return nil, fmt.Errorf("failed to parse cask json for %s: %w", name, err)
	}

	return &ck, nil
}

// GetOutdated returns a list of outdated packages (formulae and casks)
func (c *Client) GetOutdated() ([]OutdatedPackage, error) {
	// 1. Get installed packages
	installed, err := c.ListInstalledNative()
	if err != nil {
		return nil, err
	}

	if len(installed) == 0 {
		return []OutdatedPackage{}, nil
	}

	// 2. Check each package for updates in parallel
	var outdated []OutdatedPackage
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, pkg := range installed {
		wg.Add(1)
		go func(p PackageInfo) {
			defer wg.Done()

			// Try to fetch as formula first
			remote, err := c.FetchFormula(p.Name)
			if err == nil {
				// It's a formula - compare versions
				installedBase := stripRevision(p.Version)
				if remote.Versions.Stable != installedBase {
					mu.Lock()
					outdated = append(outdated, OutdatedPackage{
						Name:           p.Name,
						CurrentVersion: p.Version,
						NewVersion:     remote.Versions.Stable,
						IsCask:         false,
					})
					mu.Unlock()
				}
				return
			}

			// Try as cask
			cask, err := c.FetchCask(p.Name)
			if err == nil {
				// It's a cask - compare versions
				installedBase := stripRevision(p.Version)
				if cask.Version != installedBase {
					mu.Lock()
					outdated = append(outdated, OutdatedPackage{
						Name:           p.Name,
						CurrentVersion: p.Version,
						NewVersion:     cask.Version,
						IsCask:         true,
					})
					mu.Unlock()
				}
			}
		}(pkg)
	}

	wg.Wait()
	return outdated, nil
}

// stripRevision removes revision suffixes like "_1" from version strings
func stripRevision(version string) string {
	if idx := strings.Index(version, "_"); idx != -1 {
		return version[:idx]
	}
	return version
}
