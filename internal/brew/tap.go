package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Tap represents a Homebrew tap with metadata
type Tap struct {
	Name        string    `json:"name"`
	RemoteURL   string    `json:"remote_url"`
	LocalPath   string    `json:"local_path"`
	InstalledAt time.Time `json:"installed_at"`
	IsCustom    bool      `json:"is_custom"`
}

// TapInfo represents detailed information about a tap
type TapInfo struct {
	Tap       Tap
	Formulae  []string
	Casks     []string
	Installed []string // Formulae currently installed from this tap
}

// TapManager handles tap registry operations
type TapManager struct {
	registryPath string
	taps         map[string]Tap
	mu           sync.RWMutex
}

// NewTapManager creates a new TapManager with the registry at ~/.fastbrew/taps.json
func NewTapManager() (*TapManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get home directory: %w", err)
	}

	fastbrewDir := filepath.Join(homeDir, ".fastbrew")
	if err := os.MkdirAll(fastbrewDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create fastbrew directory: %w", err)
	}

	registryPath := filepath.Join(fastbrewDir, "taps.json")
	tm := &TapManager{
		registryPath: registryPath,
		taps:         make(map[string]Tap),
	}

	// Load existing registry
	if err := tm.loadRegistry(); err != nil {
		// It's okay if the file doesn't exist yet
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return tm, nil
}

// loadRegistry loads the tap registry from disk
func (tm *TapManager) loadRegistry() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	data, err := os.ReadFile(tm.registryPath)
	if err != nil {
		return err
	}

	var taps []Tap
	if err := json.Unmarshal(data, &taps); err != nil {
		return fmt.Errorf("could not parse taps registry: %w", err)
	}

	for _, tap := range taps {
		tm.taps[tap.Name] = tap
	}

	return nil
}

// saveRegistry saves the tap registry to disk
func (tm *TapManager) saveRegistry() error {
	tm.mu.RLock()
	taps := make([]Tap, 0, len(tm.taps))
	for _, tap := range tm.taps {
		taps = append(taps, tap)
	}
	tm.mu.RUnlock()

	data, err := json.MarshalIndent(taps, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal taps registry: %w", err)
	}

	if err := os.WriteFile(tm.registryPath, data, 0644); err != nil {
		return fmt.Errorf("could not save taps registry: %w", err)
	}

	return nil
}

// ListTaps returns all taps from brew and the registry
func (tm *TapManager) ListTaps() ([]Tap, error) {
	// First, get taps from brew
	brewTaps, err := tm.getBrewTaps()
	if err != nil {
		// If brew tap fails, return what we have in registry
		tm.mu.RLock()
		taps := make([]Tap, 0, len(tm.taps))
		for _, tap := range tm.taps {
			taps = append(taps, tap)
		}
		tm.mu.RUnlock()
		return taps, nil
	}

	// Merge with registry
	tm.mu.Lock()
	for _, tap := range brewTaps {
		tm.taps[tap.Name] = tap
	}
	tm.mu.Unlock()

	// Save merged registry
	if err := tm.saveRegistry(); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: could not save tap registry: %v\n", err)
	}

	return brewTaps, nil
}

// getBrewTaps shells out to `brew tap` and parses the output
func (tm *TapManager) getBrewTaps() ([]Tap, error) {
	cmd := exec.Command("brew", "tap")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run 'brew tap': %w", err)
	}

	var taps []Tap
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Get tap info in parallel
	var wg sync.WaitGroup
	tapChan := make(chan Tap, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			tap := tm.getTapDetails(repo)
			tapChan <- tap
		}(line)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(tapChan)
	}()

	// Collect results
	for tap := range tapChan {
		taps = append(taps, tap)
	}

	return taps, nil
}

// getTapDetails gets details for a tap by running brew commands
func (tm *TapManager) getTapDetails(repo string) Tap {
	tap := Tap{
		Name:        repo,
		InstalledAt: time.Now(),
	}

	// Try to get the remote URL
	cmd := exec.Command("brew", "--repository", repo)
	if output, err := cmd.Output(); err == nil {
		tap.LocalPath = strings.TrimSpace(string(output))
	}

	// Try to get remote URL from git
	if tap.LocalPath != "" {
		cmd = exec.Command("git", "-C", tap.LocalPath, "remote", "get-url", "origin")
		if output, err := cmd.Output(); err == nil {
			tap.RemoteURL = strings.TrimSpace(string(output))
		}
	}

	// Check if it's a custom tap (not homebrew/core or homebrew/cask)
	if !strings.HasPrefix(repo, "homebrew/") {
		tap.IsCustom = true
	}

	return tap
}

// Tap adds a tap using brew tap
func (tm *TapManager) Tap(repo string, full bool) error {
	// Validate repo format
	if !isValidTapRepo(repo) {
		return fmt.Errorf("invalid tap repo format: %s (expected user/repo or full URL)", repo)
	}

	// Build brew tap command
	args := []string{"tap"}
	if full {
		args = append(args, "--full")
	}
	args = append(args, repo)

	// Execute brew tap
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to tap %s: %w", repo, err)
	}

	// Get tap details and add to registry
	tap := tm.getTapDetails(repo)
	tap.InstalledAt = time.Now()

	tm.mu.Lock()
	tm.taps[repo] = tap
	tm.mu.Unlock()

	// Save registry
	if err := tm.saveRegistry(); err != nil {
		return fmt.Errorf("tap added but failed to save registry: %w", err)
	}

	return nil
}

// Untap removes a tap using brew untap
func (tm *TapManager) Untap(repo string, force bool) error {
	// Build brew untap command
	args := []string{"untap"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, repo)

	// Execute brew untap
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to untap %s: %w", repo, err)
	}

	// Remove from registry
	tm.mu.Lock()
	delete(tm.taps, repo)
	tm.mu.Unlock()

	// Save registry
	if err := tm.saveRegistry(); err != nil {
		return fmt.Errorf("untap succeeded but failed to save registry: %w", err)
	}

	return nil
}

// GetTapInfo returns detailed information about a tap
func (tm *TapManager) GetTapInfo(repo string, installedOnly bool) (*TapInfo, error) {
	// Validate repo
	if !isValidTapRepo(repo) {
		return nil, fmt.Errorf("invalid tap repo format: %s", repo)
	}

	// Get tap details
	tap := tm.getTapDetails(repo)

	// Check if tap exists
	if tap.LocalPath == "" {
		return nil, fmt.Errorf("tap %s not found", repo)
	}

	info := &TapInfo{
		Tap: tap,
	}

	// Get formulae from tap
	formulaeDir := filepath.Join(tap.LocalPath, "Formula")
	if entries, err := os.ReadDir(formulaeDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
				name := strings.TrimSuffix(entry.Name(), ".rb")
				info.Formulae = append(info.Formulae, name)
			}
		}
	}

	// Also check root directory for formula files
	if entries, err := os.ReadDir(tap.LocalPath); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
				name := strings.TrimSuffix(entry.Name(), ".rb")
				// Avoid duplicates
				found := false
				for _, f := range info.Formulae {
					if f == name {
						found = true
						break
					}
				}
				if !found {
					info.Formulae = append(info.Formulae, name)
				}
			}
		}
	}

	// Get casks from tap
	casksDir := filepath.Join(tap.LocalPath, "Casks")
	if entries, err := os.ReadDir(casksDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
				name := strings.TrimSuffix(entry.Name(), ".rb")
				info.Casks = append(info.Casks, name)
			}
		}
	}

	// If installedOnly flag, filter to show only installed formulae
	if installedOnly {
		// Get list of installed packages
		client, err := NewClient()
		if err == nil {
			installed, err := client.ListInstalledNative()
			if err == nil {
				installedMap := make(map[string]bool)
				for _, pkg := range installed {
					installedMap[pkg.Name] = true
				}

				// Filter formulae to only installed ones
				var installedFromTap []string
				for _, formula := range info.Formulae {
					if installedMap[formula] {
						installedFromTap = append(installedFromTap, formula)
					}
				}
				info.Installed = installedFromTap
			}
		}
	}

	return info, nil
}

// isValidTapRepo checks if the repo string is a valid tap reference
func isValidTapRepo(repo string) bool {
	// Short form: user/repo
	if strings.Count(repo, "/") == 1 && !strings.Contains(repo, ":") {
		parts := strings.Split(repo, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return true
		}
	}

	// Full URL form
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return true
	}

	return false
}

// GetTap returns a single tap from the registry
func (tm *TapManager) GetTap(repo string) (Tap, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tap, exists := tm.taps[repo]
	return tap, exists
}
