package brew

import (
	"bufio"
	"bytes"
	"fastbrew/internal/progress"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Client struct {
	Prefix          string
	Cellar          string
	Verbose         bool
	ProgressManager *progress.Manager
	prefixIndex     *PrefixIndex
	indexOnce       sync.Once
}

func NewClient() (*Client, error) {
	// 1. Check Env
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		return &Client{Prefix: p, Cellar: filepath.Join(p, "Cellar")}, nil
	}

	// 2. Check Standard Linux Path (Most likely for this user)
	if _, err := os.Stat("/home/linuxbrew/.linuxbrew"); err == nil {
		return &Client{Prefix: "/home/linuxbrew/.linuxbrew", Cellar: "/home/linuxbrew/.linuxbrew/Cellar"}, nil
	}

	// 3. Check Standard Mac Paths
	if _, err := os.Stat("/opt/homebrew"); err == nil {
		return &Client{Prefix: "/opt/homebrew", Cellar: "/opt/homebrew/Cellar"}, nil
	}
	if _, err := os.Stat("/usr/local/Cellar"); err == nil {
		return &Client{Prefix: "/usr/local", Cellar: "/usr/local/Cellar"}, nil
	}

	// 4. Fallback to slow exec
	cmd := exec.Command("brew", "--prefix")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not find brew prefix: %w", err)
	}
	prefix := strings.TrimSpace(string(out))
	return &Client{Prefix: prefix, Cellar: filepath.Join(prefix, "Cellar")}, nil
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
	if _, err := os.Stat(c.Cellar); err == nil {
		entries, err := os.ReadDir(c.Cellar)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()

			// Get version from subdirectory
			versionsDir := filepath.Join(c.Cellar, name)
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

	// 2. Get casks from brew list --cask
	cmd := exec.Command("brew", "list", "--cask")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			name := strings.TrimSpace(scanner.Text())
			if name != "" {
				packages = append(packages, PackageInfo{
					Name:      name,
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
