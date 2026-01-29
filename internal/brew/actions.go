package brew

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Fetch downloads the package bottle/source
func (c *Client) Fetch(pkg string) error {
	cmd := exec.Command("brew", "fetch", pkg)
	// We might want to suppress output or stream it nicely.
	// For now, let's just run it.
	return cmd.Run()
}

// InstallNative performs native installation by resolving deps, downloading bottles, and linking.
func (c *Client) InstallNative(packages []string) error {
	fmt.Println("🔍 Resolving dependencies from API...")

	visited := make(map[string]bool)
	var installQueue []*RemoteFormula

	// Recursive resolver
	var resolve func(name string) error
	resolve = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true

		if c.isInstalled(name) {
			// Skip if installed?
			// Check if we need to check deps of installed packages?
			// For MVP, if installed, assume deps are satisfied or will be handled by brew doctor if broken.
			return nil
		}

		f, err := c.FetchFormula(name)
		if err != nil {
			return err
		}

		for _, dep := range f.Dependencies {
			if err := resolve(dep); err != nil {
				return err
			}
		}

		installQueue = append(installQueue, f)
		return nil
	}

	for _, pkg := range packages {
		if err := resolve(pkg); err != nil {
			return fmt.Errorf("dependency resolution failed for %s: %w", pkg, err)
		}
	}

	if len(installQueue) == 0 {
		fmt.Println("✅ All packages already installed.")
		return nil
	}

	fmt.Printf("📦 Found %d packages to install. Downloading in parallel...\n", len(installQueue))

	// Parallel Download
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Concurrency limit
	errChan := make(chan error, len(installQueue))

	for _, f := range installQueue {
		wg.Add(1)
		go func(frm *RemoteFormula) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := c.InstallBottle(frm); err != nil {
				errChan <- fmt.Errorf("failed to install %s: %w", frm.Name, err)
				fmt.Printf("  ❌ Failed %s: %v\n", frm.Name, err)
			} else {
				fmt.Printf("  ✅ Extracted %s\n", frm.Name)
			}
		}(f)
	}
	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return fmt.Errorf("some installs failed, check output")
	}

	// Sequential Link (safer)
	fmt.Println("🔗 Linking binaries...")
	for _, f := range installQueue {
		if err := c.Link(f.Name, f.Versions.Stable); err != nil {
			fmt.Printf("Error linking %s: %v\n", f.Name, err)
		}
	}

	return nil
}

func (c *Client) isInstalled(name string) bool {
	p := filepath.Join(c.Cellar, name)
	if _, err := os.Stat(p); err == nil {
		return true
	}
	return false
}

// ResolveDeps returns a list of recursive dependencies for the given packages using the cached Index
func (c *Client) ResolveDeps(packages []string) ([]string, error) {
	idx, err := c.LoadIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to load index for dependency resolution: %w", err)
	}

	// Build a map for O(1) lookups
	formulaMap := make(map[string]Formula)
	for _, f := range idx.Formulae {
		formulaMap[f.Name] = f
	}

	visited := make(map[string]bool)
	var deps []string

	// Recursive resolver function
	var resolve func(string)
	resolve = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		f, exists := formulaMap[name]
		if !exists {
			// Might be a cask or system lib, skip for now
			return
		}

		for _, dep := range f.Dependencies {
			resolve(dep)
			deps = append(deps, dep)
		}
	}

	for _, pkg := range packages {
		resolve(pkg)
	}

	return unique(deps), nil
}

// UpgradeParallel identifies outdated packages and fetches them in parallel
func (c *Client) UpgradeParallel(packages []string) error {
	fmt.Println("🔍 Checking for outdated packages...")
	outdated, err := c.getOutdated(packages)
	if err != nil {
		return err
	}

	if len(outdated) == 0 {
		fmt.Println("✅ All packages are up to date.")
		return nil
	}

	fmt.Printf("📦 Found %d packages to upgrade. Fetching in parallel...\n", len(outdated))

	// Use same parallel fetch logic as install
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, pkg := range outdated {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fmt.Printf("  ⬇️  Fetching update for %s...\n", p)
			c.Fetch(p)
		}(pkg)
	}
	wg.Wait()

	fmt.Println("💿 Upgrading...")
	args := append([]string{"upgrade"}, outdated...)
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) getOutdated(packages []string) ([]string, error) {
	args := []string{"outdated", "--quiet"}
	if len(packages) > 0 {
		args = append(args, packages...)
	}
	cmd := exec.Command("brew", args...)
	out, err := cmd.Output()
	if err != nil {
		// brew outdated exits with 1 if there are outdated packages
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// carry on
		} else {
			return nil, err
		}
	}

	var list []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		pkg := strings.TrimSpace(scanner.Text())
		if pkg != "" {
			list = append(list, pkg)
		}
	}
	return list, nil
}

func unique(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
