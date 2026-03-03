package brew

import (
	"bufio"
	"context"
	"encoding/json"
	"fastbrew/internal/httpclient"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OutdatedPackage struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	NewVersion     string `json:"new_version"`
	IsCask         bool   `json:"is_cask"`
	IsTap          bool   `json:"is_tap"`
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

func (c *Client) GetOutdatedForPackages(packages []string) ([]OutdatedPackage, error) {
	reqMap := make(map[string]bool)
	for _, pkg := range packages {
		reqMap[pkg] = true
	}

	installed, err := c.ListInstalledNative()
	if err != nil {
		return nil, err
	}

	var targetPkgs []PackageInfo
	for _, pkg := range installed {
		if reqMap[pkg.Name] {
			targetPkgs = append(targetPkgs, pkg)
		}
	}

	if len(targetPkgs) == 0 {
		for _, name := range packages {
			targetPkgs = append(targetPkgs, PackageInfo{Name: name, IsCask: false})
		}
	}

	idx, err := c.LoadIndex()
	if err != nil {
		return nil, err
	}

	formulaVersions := make(map[string]string, len(idx.Formulae))
	for _, f := range idx.Formulae {
		formulaVersions[f.Name] = f.FullVersion()
	}

	caskVersions := make(map[string]string, len(idx.Casks))
	for _, cask := range idx.Casks {
		caskVersions[cask.Token] = cask.Version
	}

	var outdated []OutdatedPackage

	for _, pkg := range targetPkgs {
		installedVer := pkg.Version
		if pkg.IsCask {
			if latest, ok := caskVersions[pkg.Name]; ok && isOutdated(installedVer, latest) {
				outdated = append(outdated, OutdatedPackage{
					Name:           pkg.Name,
					CurrentVersion: pkg.Version,
					NewVersion:     latest,
					IsCask:         true,
				})
			} else if !ok {
				cask, err := c.FetchCask(pkg.Name)
				if err == nil && isOutdated(installedVer, cask.Version) {
					outdated = append(outdated, OutdatedPackage{
						Name:           pkg.Name,
						CurrentVersion: pkg.Version,
						NewVersion:     cask.Version,
						IsCask:         true,
					})
				}
			}
			continue
		}

		if latest, ok := formulaVersions[pkg.Name]; ok && isOutdated(installedVer, latest) {
			outdated = append(outdated, OutdatedPackage{
				Name:           pkg.Name,
				CurrentVersion: pkg.Version,
				NewVersion:     latest,
				IsCask:         false,
			})
		} else if !ok {
			// Try tap formulas first
			if tapVer, tapOk := c.GetTapFormulaVersion(pkg.Name); tapOk && isOutdated(installedVer, tapVer) {
				outdated = append(outdated, OutdatedPackage{
					Name:           pkg.Name,
					CurrentVersion: pkg.Version,
					NewVersion:     tapVer,
					IsCask:         false,
					IsTap:          true,
				})
			} else if !tapOk {
				remote, err := c.FetchFormula(pkg.Name)
				if err == nil && isOutdated(installedVer, remote.Versions.Stable) {
					outdated = append(outdated, OutdatedPackage{
						Name:           pkg.Name,
						CurrentVersion: pkg.Version,
						NewVersion:     remote.Versions.Stable,
						IsCask:         false,
					})
				}
			}
		}
	}

	return outdated, nil
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
		formulaVersions[f.Name] = f.FullVersion()
	}

	caskVersions := make(map[string]string, len(idx.Casks))
	for _, cask := range idx.Casks {
		caskVersions[cask.Token] = cask.Version
	}

	// 2. Check each package against the cached index (fast path)
	var outdated []OutdatedPackage
	var unknown []PackageInfo

	for _, pkg := range installed {
		installedVer := pkg.Version
		if pkg.IsCask {
			if latest, ok := caskVersions[pkg.Name]; ok {
				if isOutdated(installedVer, latest) {
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
			if isOutdated(installedVer, latest) {
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
				installedVer := pkg.Version
				if pkg.IsCask {
					cask, err := c.FetchCask(pkg.Name)
					if err == nil && isOutdated(installedVer, cask.Version) {
						results <- OutdatedPackage{
							Name:           pkg.Name,
							CurrentVersion: pkg.Version,
							NewVersion:     cask.Version,
							IsCask:         true,
						}
					}
					continue
				}

				// Try tap formulas first
				if tapVer, tapOk := c.GetTapFormulaVersion(pkg.Name); tapOk {
					if isOutdated(installedVer, tapVer) {
						results <- OutdatedPackage{
							Name:           pkg.Name,
							CurrentVersion: pkg.Version,
							NewVersion:     tapVer,
							IsCask:         false,
							IsTap:          true,
						}
					}
					continue
				}

				remote, err := c.FetchFormula(pkg.Name)
				if err == nil && isOutdated(installedVer, remote.Versions.Stable) {
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
func stripRevision(version string) string {
	if idx := strings.Index(version, "_"); idx != -1 {
		return version[:idx]
	}
	return version
}

// extractRevision extracts the revision number from a version string.
// e.g. "15.2.0_1" returns 1, "15.2.0" returns 0.
func extractRevision(version string) int {
	if idx := strings.LastIndex(version, "_"); idx != -1 {
		rev, err := strconv.Atoi(version[idx+1:])
		if err == nil {
			return rev
		}
	}
	return 0
}

// versionCompare compares two version strings semantically, including revisions.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func versionCompare(v1, v2 string) int {
	base1 := stripRevision(v1)
	base2 := stripRevision(v2)

	parts1 := strings.Split(base1, ".")
	parts2 := strings.Split(base2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1 := 0
		p2 := 0

		if i < len(parts1) {
			p1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			p2, _ = strconv.Atoi(parts2[i])
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}

	// Base versions are equal, compare revisions
	rev1 := extractRevision(v1)
	rev2 := extractRevision(v2)
	if rev1 < rev2 {
		return -1
	}
	if rev1 > rev2 {
		return 1
	}

	return 0
}

// isOutdated checks if installed version is outdated compared to new version
func isOutdated(installed, latest string) bool {
	return versionCompare(installed, latest) < 0
}

// versionLineRegex matches lines like: version "0.46.2"
var versionLineRegex = regexp.MustCompile(`^\s*version\s+"([^"]+)"`)

// GetTapFormulaVersion scans all installed taps for a formula .rb file
// and extracts the version from it.
func (c *Client) GetTapFormulaVersion(name string) (string, bool) {
	detectHomebrewPaths()

	entries, err := os.ReadDir(homebrewTapsDir)
	if err != nil {
		return "", false
	}

	for _, userEntry := range entries {
		if !userEntry.IsDir() {
			continue
		}
		userDir := filepath.Join(homebrewTapsDir, userEntry.Name())
		repoEntries, err := os.ReadDir(userDir)
		if err != nil {
			continue
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}
			tapPath := filepath.Join(userDir, repoEntry.Name())

			// Check Formula/ subdirectory and root for .rb files
			candidatePaths := []string{
				filepath.Join(tapPath, "Formula", name+".rb"),
				filepath.Join(tapPath, name+".rb"),
			}

			for _, rbPath := range candidatePaths {
				if ver, ok := parseRubyFormulaVersion(rbPath); ok {
					return ver, true
				}
			}
		}
	}

	return "", false
}

// parseRubyFormulaVersion reads a .rb formula file and extracts the version string.
func parseRubyFormulaVersion(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := versionLineRegex.FindStringSubmatch(line); len(matches) == 2 {
			return matches[1], true
		}
	}
	return "", false
}
