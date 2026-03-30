package brew

import (
	"fastbrew/internal/progress"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Client struct {
	Environment     *Environment
	Prefix          string
	Cellar          string
	Verbose         bool
	MaxParallel     int
	ProgressManager *progress.Manager
	index           *Index
	indexErr        error
	indexOnce       sync.Once
	prefixIndex     *PrefixIndex
	prefixIndexOnce sync.Once
	invalidationMu  sync.RWMutex
	onInvalidation  func(event string)
	mutationMu      sync.RWMutex
	onMutation      func(event MutationEvent)
}

const (
	EventInstalledChanged = "installed_changed"
	EventTapChanged       = "tap_changed"
	EventIndexRefreshed   = "index_refreshed"
	EventServiceChanged   = "service_changed"
)

func (c *Client) getMaxParallel() int {
	if c.MaxParallel <= 0 {
		return 4
	}
	return c.MaxParallel
}

func NewClient() (*Client, error) {
	env, err := GetEnvironment()
	if err != nil {
		return nil, err
	}

	return &Client{
		Environment: env,
		Prefix:      env.Prefix,
		Cellar:      env.Cellar,
	}, nil
}

// PackageInfo represents minimal info needed for listing/searching
type PackageInfo struct {
	Name        string `json:"name"`
	Description string `json:"desc"`
	Homepage    string `json:"homepage,omitempty"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version"`
	IsCask      bool   `json:"is_cask"`
}

// ListInstalledNative returns installed packages by scanning Cellar and checking for casks
func (c *Client) ListInstalledNative() ([]PackageInfo, error) {
	var packages []PackageInfo

	// 1. Get formulae from Cellar
	if _, err := os.Stat(c.Environment.Cellar); err == nil {
		entries, err := os.ReadDir(c.Environment.Cellar)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()

			// Get version from subdirectory
			versionsDir := filepath.Join(c.Environment.Cellar, name)
			vEntries, err := os.ReadDir(versionsDir)
			if err != nil {
				continue
			}

			if len(vEntries) == 0 {
				continue
			}

			// Find latest version directory
			latestVer := vEntries[len(vEntries)-1].Name()

			// Skip hidden/system files if any
			if strings.HasPrefix(latestVer, ".") {
				continue
			}

			packages = append(packages, PackageInfo{
				Name:      name,
				Version:   latestVer,
				Installed: true,
				IsCask:    false,
			})
		}
	}

	// 2. Get casks from Caskroom directory
	if _, err := os.Stat(c.Environment.Caskroom); err == nil {
		entries, err := os.ReadDir(c.Environment.Caskroom)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				versionsDir := filepath.Join(c.Environment.Caskroom, name)
				vEntries, err := os.ReadDir(versionsDir)
				if err != nil || len(vEntries) == 0 {
					continue
				}
				latestVer := vEntries[len(vEntries)-1].Name()
				if strings.HasPrefix(latestVer, ".") {
					continue
				}
				packages = append(packages, PackageInfo{
					Name:      name,
					Version:   latestVer,
					Installed: true,
					IsCask:    true,
				})
			}
		}
	}

	return packages, nil
}

// ListInstalled returns a list of installed packages (Legacy wrapper pointing to Native)
func (c *Client) ListInstalled() ([]PackageInfo, error) {
	return c.ListInstalledNative()
}

func (c *Client) EnableProgress() {
	if c.ProgressManager == nil {
		c.ProgressManager = progress.NewManager()
		c.ProgressManager.StartEventRouter()
	}
}

func (c *Client) DisableProgress() {
	if c.ProgressManager != nil {
		c.ProgressManager.Close()
		c.ProgressManager = nil
	}
}

func (c *Client) SetInvalidationHook(fn func(event string)) {
	c.invalidationMu.Lock()
	defer c.invalidationMu.Unlock()
	c.onInvalidation = fn
}

func (c *Client) SetMutationHook(fn func(event MutationEvent)) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()
	c.onMutation = fn
}

func (c *Client) notifyInvalidation(event string) {
	c.invalidationMu.RLock()
	defer c.invalidationMu.RUnlock()
	if c.onInvalidation != nil {
		c.onInvalidation(event)
	}
}

func (c *Client) notifyMutation(event MutationEvent) {
	c.mutationMu.RLock()
	defer c.mutationMu.RUnlock()
	if c.onMutation != nil {
		c.onMutation(event)
	}
}
