package brew

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Client struct {
	Prefix string
}

func NewClient() (*Client, error) {
	// Try to find brew prefix
	cmd := exec.Command("brew", "--prefix")
	out, err := cmd.Output()
	if err != nil {
		// Fallback for standard linux path if brew command fails/not in path
		if _, err := os.Stat("/home/linuxbrew/.linuxbrew"); err == nil {
			return &Client{Prefix: "/home/linuxbrew/.linuxbrew"}, nil
		}
		return nil, fmt.Errorf("could not find brew prefix: %w", err)
	}
	return &Client{Prefix: strings.TrimSpace(string(out))}, nil
}

// PackageInfo represents minimal info needed for listing/searching
type PackageInfo struct {
	Name        string `json:"name"`
	Description string `json:"desc"`
	Homepage    string `json:"homepage,omitempty"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version"`
}

// ListInstalled returns a list of installed packages
func (c *Client) ListInstalled() ([]PackageInfo, error) {
	cmd := exec.Command("brew", "info", "--installed", "--json=v2")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var result struct {
		Formulae []struct {
			Name        string `json:"name"`
			Desc        string `json:"desc"`
			Homepage    string `json:"homepage"`
			Versions    struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
		Casks []struct {
			Token       string `json:"token"`
			Desc        string `json:"desc"`
			Homepage    string `json:"homepage"`
			Version     string `json:"version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, err
	}

	var packages []PackageInfo
	for _, f := range result.Formulae {
		packages = append(packages, PackageInfo{
			Name:        f.Name,
			Description: f.Desc,
			Homepage:    f.Homepage,
			Installed:   true,
			Version:     f.Versions.Stable,
		})
	}
	for _, c := range result.Casks {
		packages = append(packages, PackageInfo{
			Name:        c.Token,
			Description: c.Desc,
			Homepage:    c.Homepage,
			Installed:   true,
			Version:     c.Version,
		})
	}

	return packages, nil
}
