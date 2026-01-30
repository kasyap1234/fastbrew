package bundle

import (
	"fmt"
	"os/exec"
	"strings"
)

// Dumper collects information about installed packages for Brewfile generation
type Dumper struct {
	brewPath string
}

// NewDumper creates a new Dumper using the system's brew command
func NewDumper() *Dumper {
	return &Dumper{brewPath: "brew"}
}

// DumpOptions configures what to include in the Brewfile
type DumpOptions struct {
	IncludeBrews bool
	IncludeCasks bool
	IncludeTaps  bool
	IncludeMas   bool
	Descriptions bool
}

// DefaultDumpOptions returns options that include everything
func DefaultDumpOptions() DumpOptions {
	return DumpOptions{
		IncludeBrews: true,
		IncludeCasks: true,
		IncludeTaps:  true,
		IncludeMas:   true,
		Descriptions: false,
	}
}

// DumpResult contains all data needed to generate a Brewfile
type DumpResult struct {
	Brews []BrewInfo
	Casks []CaskInfo
	Taps  []TapInfo
	Mas   []MasInfo
}

// BrewInfo represents an installed formula
type BrewInfo struct {
	Name        string
	Version     string
	Description string
	Args        []string
}

// CaskInfo represents an installed cask
type CaskInfo struct {
	Name        string
	Version     string
	Description string
}

// TapInfo represents a tapped repository
type TapInfo struct {
	User string
	Repo string
	Name string
}

// MasInfo represents a Mac App Store application
type MasInfo struct {
	Name string
	ID   string
}

// Dump collects all installed packages
func (d *Dumper) Dump(opts DumpOptions) (*DumpResult, error) {
	result := &DumpResult{}

	if opts.IncludeTaps {
		taps, err := d.DumpTaps()
		if err != nil {
			return nil, fmt.Errorf("failed to dump taps: %w", err)
		}
		result.Taps = taps
	}

	if opts.IncludeBrews {
		brews, err := d.DumpBrews()
		if err != nil {
			return nil, fmt.Errorf("failed to dump brews: %w", err)
		}
		result.Brews = brews
	}

	if opts.IncludeCasks {
		casks, err := d.DumpCasks()
		if err != nil {
			return nil, fmt.Errorf("failed to dump casks: %w", err)
		}
		result.Casks = casks
	}

	if opts.IncludeMas {
		mas, err := d.DumpMas()
		if err != nil {
			// mas might not be installed, that's okay
			result.Mas = []MasInfo{}
		} else {
			result.Mas = mas
		}
	}

	return result, nil
}

// DumpBrews returns installed formulae
func (d *Dumper) DumpBrews() ([]BrewInfo, error) {
	cmd := exec.Command(d.brewPath, "list", "--formula", "--versions")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var brews []BrewInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 1 {
			version := ""
			if len(parts) >= 2 {
				version = parts[1]
			}
			brews = append(brews, BrewInfo{
				Name:    parts[0],
				Version: version,
			})
		}
	}

	return brews, nil
}

// DumpCasks returns installed casks
func (d *Dumper) DumpCasks() ([]CaskInfo, error) {
	cmd := exec.Command(d.brewPath, "list", "--cask", "--versions")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var casks []CaskInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 1 {
			version := ""
			if len(parts) >= 2 {
				version = parts[1]
			}
			casks = append(casks, CaskInfo{
				Name:    parts[0],
				Version: version,
			})
		}
	}

	return casks, nil
}

// DumpTaps returns active taps
func (d *Dumper) DumpTaps() ([]TapInfo, error) {
	cmd := exec.Command(d.brewPath, "tap")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var taps []TapInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "/")
		if len(parts) >= 2 {
			taps = append(taps, TapInfo{
				User: parts[0],
				Repo: parts[1],
				Name: line,
			})
		}
	}

	return taps, nil
}

// DumpMas returns installed Mac App Store apps
func (d *Dumper) DumpMas() ([]MasInfo, error) {
	cmd := exec.Command("mas", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var apps []MasInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			apps = append(apps, MasInfo{
				ID:   parts[0],
				Name: strings.Join(parts[1:], " "),
			})
		}
	}

	return apps, nil
}

// IsMasInstalled checks if mas CLI is available
func (d *Dumper) IsMasInstalled() bool {
	cmd := exec.Command("mas", "--version")
	err := cmd.Run()
	return err == nil
}
