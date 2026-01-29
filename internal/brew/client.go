package brew

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	Prefix string
	Cellar string
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
}

// ListInstalledNative returns installed packages by scanning Cellar
func (c *Client) ListInstalledNative() ([]PackageInfo, error) {
	if _, err := os.Stat(c.Cellar); os.IsNotExist(err) {
		return []PackageInfo{}, nil // Cellar doesn't exist yet, empty list
	}

	entries, err := os.ReadDir(c.Cellar)
	if err != nil {
		return nil, err
	}

	var packages []PackageInfo
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
		})
	}
	return packages, nil
}

// ListInstalled returns a list of installed packages (Legacy wrapper pointing to Native)
func (c *Client) ListInstalled() ([]PackageInfo, error) {
	return c.ListInstalledNative()
}
