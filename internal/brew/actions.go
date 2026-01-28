package brew

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

// InstallParallel performs parallel fetch followed by install
func (c *Client) InstallParallel(packages []string) error {
	// 1. Resolve all dependencies (optional optimization, but brew install handles it)
	// For MVP, we'll just fetch the requested packages in parallel. 
	// To be truly effective, we should `brew deps` first.

	fmt.Println("🔍 Resolving dependencies...")
	deps, err := c.ResolveDeps(packages)
	if err != nil {
		return fmt.Errorf("failed to resolve deps: %w", err)
	}
	
	// Add original packages to the list to fetch
	allToFetch := append(deps, packages...)
	allToFetch = unique(allToFetch)

	fmt.Printf("📦 Fetching %d packages in parallel...\n", len(allToFetch))

	// 2. Parallel Fetch
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit to 5 concurrent downloads
	errChan := make(chan error, len(allToFetch))

	for _, pkg := range allToFetch {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("  ⬇️  Fetching %s...\n", p)
			if err := c.Fetch(p); err != nil {
				errChan <- fmt.Errorf("failed to fetch %s: %w", p, err)
				fmt.Printf("  ❌ Failed %s\n", p)
			} else {
				fmt.Printf("  ✅ Fetched %s\n", p)
			}
		}(pkg)
	}
	wg.Wait()
	close(errChan)

	// Check if any critical fetch failed? 
	// Actually, `brew install` might still work if fetch failed (maybe it builds from source), 
	// so we proceed but warn.
	if len(errChan) > 0 {
		fmt.Println("⚠️  Some downloads failed, falling back to standard install behavior.")
	}

	// 3. Install
	fmt.Println("💿 Installing...")
	args := append([]string{"install"}, packages...)
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
