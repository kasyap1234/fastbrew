package brew

import (
	"bufio"
	"bytes"
	"context"
	"fastbrew/internal/retry"
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
	return cmd.Run()
}

// InstallNative performs native installation by resolving deps, downloading bottles, and linking.
// Also handles cask installation via brew install --cask.
func (c *Client) InstallNative(packages []string) error {
	// Split packages into casks and formulae
	var casks, formulae []string
	for _, pkg := range packages {
		isCask, _ := c.IsCask(pkg)
		if isCask {
			casks = append(casks, pkg)
		} else {
			formulae = append(formulae, pkg)
		}
	}

	// Install formulae using native bottle installation
	if len(formulae) > 0 {
		if err := c.installFormulae(formulae); err != nil {
			return err
		}
	}

	// Install casks using brew install --cask
	if len(casks) > 0 {
		fmt.Printf("🍷 Installing casks: %v\n", casks)
		args := append([]string{"install", "--cask"}, casks...)
		cmd := exec.Command("brew", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cask installation failed: %w", err)
		}
		fmt.Println("✅ Casks installed successfully")
	}

	return nil
}

// installFormulae handles formula installation via bottles
func (c *Client) installFormulae(packages []string) error {
	fmt.Println("🔍 Resolving dependencies from API...")

	idx, err := c.LoadIndex()
	if err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	formulaMap := make(map[string]Formula)
	for _, f := range idx.Formulae {
		formulaMap[f.Name] = f
	}

	needed := make(map[string]bool)
	var collectNeeded func(name string)
	collectNeeded = func(name string) {
		if needed[name] || c.isInstalled(name) {
			return
		}
		needed[name] = true

		if f, ok := formulaMap[name]; ok {
			for _, dep := range f.Dependencies {
				collectNeeded(dep)
			}
		}
	}

	for _, pkg := range packages {
		collectNeeded(pkg)
	}

	if len(needed) == 0 {
		fmt.Println("✅ All formulae already installed.")
		return nil
	}

	neededList := make([]string, 0, len(needed))
	for name := range needed {
		neededList = append(neededList, name)
	}

	fmt.Printf("📡 Fetching metadata for %d formulae in parallel...\n", len(neededList))

	const maxWorkers = 10
	type fetchResult struct {
		formula *RemoteFormula
		err     error
	}

	results := make(chan fetchResult, len(neededList))
	fetchSem := make(chan struct{}, maxWorkers)
	var fetchWg sync.WaitGroup

	ctx := context.Background()
	for _, name := range neededList {
		fetchWg.Add(1)
		go func(n string) {
			defer fetchWg.Done()
			fetchSem <- struct{}{}
			defer func() { <-fetchSem }()

			f, err := retry.WithResult(ctx, func() (*RemoteFormula, error) {
				return c.FetchFormula(n)
			})
			results <- fetchResult{formula: f, err: err}
		}(name)
	}

	fetchWg.Wait()
	close(results)

	formulaDetails := make(map[string]*RemoteFormula)
	for res := range results {
		if res.err != nil {
			return fmt.Errorf("failed to fetch formula: %w", res.err)
		}
		formulaDetails[res.formula.Name] = res.formula
	}

	visited := make(map[string]bool)
	var installQueue []*RemoteFormula
	var buildQueue func(name string)
	buildQueue = func(name string) {
		if visited[name] || c.isInstalled(name) {
			return
		}
		visited[name] = true

		f := formulaDetails[name]
		if f == nil {
			return
		}

		for _, dep := range f.Dependencies {
			buildQueue(dep)
		}
		installQueue = append(installQueue, f)
	}

	for _, pkg := range packages {
		buildQueue(pkg)
	}

	fmt.Printf("📦 Found %d formulae to install. Downloading in parallel...\n", len(installQueue))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
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

	fmt.Println("🔗 Linking binaries...")
	if err := c.linkParallel(installQueue); err != nil {
		return err
	}

	return nil
}

func (c *Client) linkParallel(installQueue []*RemoteFormula) error {
	const numWorkers = 5

	conflictTracker := NewConflictTracker()

	fmt.Println("  📋 Detecting conflicts...")
	for _, f := range installQueue {
		result, err := c.LinkDryRun(f.Name, f.Versions.Stable)
		if err != nil {
			fmt.Printf("  ⚠️  Error checking %s: %v\n", f.Name, err)
			continue
		}

		for _, binary := range result.Binaries {
			if conflictPkg := conflictTracker.CheckAndTrack(binary, f.Name); conflictPkg != "" {
				continue
			}
		}
	}

	conflictingPackages := conflictTracker.GetConflictingPackages()

	var parallelQueue, sequentialQueue []*RemoteFormula
	for _, f := range installQueue {
		if conflictingPackages[f.Name] {
			sequentialQueue = append(sequentialQueue, f)
		} else {
			parallelQueue = append(parallelQueue, f)
		}
	}

	if len(parallelQueue) > 0 {
		fmt.Printf("  ⚡ Linking %d packages in parallel...\n", len(parallelQueue))

		var wg sync.WaitGroup
		sem := make(chan struct{}, numWorkers)
		errorChan := make(chan error, len(parallelQueue))
		resultChan := make(chan *LinkResult, len(parallelQueue))

		for _, f := range parallelQueue {
			wg.Add(1)
			go func(frm *RemoteFormula) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result, err := c.Link(frm.Name, frm.Versions.Stable)
				if err != nil {
					errorChan <- fmt.Errorf("failed to link %s: %w", frm.Name, err)
					fmt.Printf("  ❌ Failed to link %s: %v\n", frm.Name, err)
				} else {
					resultChan <- result
				}
			}(f)
		}

		wg.Wait()
		close(errorChan)
		close(resultChan)

		successCount := 0
		for result := range resultChan {
			if result.Success {
				successCount++
			}
		}

		if successCount > 0 {
			fmt.Printf("  ✅ Linked %d packages in parallel\n", successCount)
		}
	}

	if len(sequentialQueue) > 0 {
		fmt.Printf("  🔄 Linking %d packages with conflicts sequentially...\n", len(sequentialQueue))

		sequentialTracker := NewConflictTracker()

		for binary, pkg := range conflictTracker.GetAllTrackedBinaries() {
			if !conflictingPackages[pkg] {
				sequentialTracker.CheckAndTrack(binary, pkg)
			}
		}

		for _, f := range sequentialQueue {
			result, err := c.Link(f.Name, f.Versions.Stable)
			if err != nil {
				fmt.Printf("  ❌ Failed to link %s: %v\n", f.Name, err)
				continue
			}

			for _, binary := range result.Binaries {
				if conflictPkg := sequentialTracker.CheckAndTrack(binary, f.Name); conflictPkg != "" {
					fmt.Printf("  ⚠️  Binary '%s' already linked by package '%s', skipping '%s'\n",
						binary, conflictPkg, f.Name)
				}
			}

			if result.Success {
				fmt.Printf("  ✅ Linked %s\n", f.Name)
			}
		}
	}

	conflicts := conflictTracker.GetConflicts()
	if len(conflicts) > 0 {
		fmt.Println("\n⚠️  Binary conflicts detected:")

		// Group conflicts by binary
		conflictsByBinary := make(map[string][]BinaryConflict)
		for _, c := range conflicts {
			conflictsByBinary[c.BinaryName] = append(conflictsByBinary[c.BinaryName], c)
		}

		for binary, conflictList := range conflictsByBinary {
			packages := make(map[string]bool)
			for _, c := range conflictList {
				packages[c.FirstPkg] = true
				packages[c.SecondPkg] = true
			}

			pkgList := make([]string, 0, len(packages))
			for pkg := range packages {
				pkgList = append(pkgList, pkg)
			}

			fmt.Printf("  • Binary '%s' - packages: %s\n", binary, strings.Join(pkgList, ", "))
		}

		fmt.Println("\n💡 To resolve conflicts, run:")
		for binary, conflictList := range conflictsByBinary {
			if len(conflictList) > 0 {
				c := conflictList[0]
				fmt.Printf("  • brew unlink %s && fastbrew link %s  (for binary '%s')\n",
					c.FirstPkg, c.SecondPkg, binary)
			}
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

	formulaMap := make(map[string]Formula)
	for _, f := range idx.Formulae {
		formulaMap[f.Name] = f
	}

	visited := make(map[string]bool)
	var deps []string

	var resolve func(string)
	resolve = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		f, exists := formulaMap[name]
		if !exists {
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
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			_ = exitErr
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
