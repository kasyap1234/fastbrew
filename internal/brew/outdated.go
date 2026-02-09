package brew

import (
	"context"
	"encoding/json"
	"fastbrew/internal/httpclient"
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for cask %s: %w", name, err)
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
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

	idx, err := c.LoadIndex()
	if err != nil {
		return nil, err
	}

	formulaVersions := make(map[string]string, len(idx.Formulae))
	for _, f := range idx.Formulae {
		formulaVersions[f.Name] = f.Version
	}

	caskVersions := make(map[string]string, len(idx.Casks))
	for _, cask := range idx.Casks {
		caskVersions[cask.Token] = cask.Version
	}

	// 2. Check each package against the cached index (fast path)
	var outdated []OutdatedPackage
	var unknown []PackageInfo

	for _, pkg := range installed {
		installedBase := stripRevision(pkg.Version)
		if pkg.IsCask {
			if latest, ok := caskVersions[pkg.Name]; ok {
				if latest != installedBase {
					outdated = append(outdated, OutdatedPackage{
						Name:           pkg.Name,
						CurrentVersion: pkg.Version,
						NewVersion:     latest,
						IsCask:         true,
					})
				}
				continue
			}
			unknown = append(unknown, pkg)
			continue
		}

		if latest, ok := formulaVersions[pkg.Name]; ok {
			if latest != installedBase {
				outdated = append(outdated, OutdatedPackage{
					Name:           pkg.Name,
					CurrentVersion: pkg.Version,
					NewVersion:     latest,
					IsCask:         false,
				})
			}
			continue
		}

		unknown = append(unknown, pkg)
	}

	// 3. Fallback to remote lookups for packages not in the cached index
	if len(unknown) > 0 {
		const maxWorkers = 8
		jobs := make(chan PackageInfo)
		results := make(chan OutdatedPackage, len(unknown))
		var wg sync.WaitGroup

		worker := func() {
			defer wg.Done()
			for pkg := range jobs {
				installedBase := stripRevision(pkg.Version)
				if pkg.IsCask {
					cask, err := c.FetchCask(pkg.Name)
					if err == nil && cask.Version != installedBase {
						results <- OutdatedPackage{
							Name:           pkg.Name,
							CurrentVersion: pkg.Version,
							NewVersion:     cask.Version,
							IsCask:         true,
						}
					}
					continue
				}

				remote, err := c.FetchFormula(pkg.Name)
				if err == nil && remote.Versions.Stable != installedBase {
					results <- OutdatedPackage{
						Name:           pkg.Name,
						CurrentVersion: pkg.Version,
						NewVersion:     remote.Versions.Stable,
						IsCask:         false,
					}
				}
			}
		}

		wg.Add(maxWorkers)
		for i := 0; i < maxWorkers; i++ {
			go worker()
		}

		for _, pkg := range unknown {
			jobs <- pkg
		}
		close(jobs)
		wg.Wait()
		close(results)

		for pkg := range results {
			outdated = append(outdated, pkg)
		}
	}

	return outdated, nil
}

// stripRevision removes revision suffixes like "_1" from version strings.
// NOTE: Version comparison uses simple string equality after stripping revisions,
// which can produce false positives (e.g., "1.0" vs "1.0.0" are treated as different).
func stripRevision(version string) string {
	if idx := strings.Index(version, "_"); idx != -1 {
		return version[:idx]
	}
	return version
}
