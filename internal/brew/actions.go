package brew

import (
	"context"
	"fastbrew/internal/retry"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Fetch downloads the package bottle/source
func (c *Client) Fetch(pkg string) error {
	idx, err := c.LoadIndex()
	if err != nil {
		return err
	}

	isCask := false
	for _, cask := range idx.Casks {
		if cask.Token == pkg {
			isCask = true
			break
		}
	}

	if isCask {
		metadata, err := c.FetchCaskMetadata(pkg)
		if err != nil {
			return err
		}
		installer := NewCaskInstaller(c)
		caskDir, err := installer.getCaskVersionDir(pkg, metadata.Version)
		if err != nil {
			return err
		}
		destPath := filepath.Join(caskDir, filepath.Base(metadata.URL))
		return installer.downloadArtifact(pkg, metadata.URL, destPath, metadata.SHA256, c.ProgressManager)
	}

	f, err := c.FetchFormula(pkg)
	if err != nil {
		return err
	}
	_, _, err = f.GetBottleInfo()
	return err
}

// InstallNative performs native installation by resolving deps, downloading bottles, and linking.
// Also handles cask installation via brew install --cask.
func (c *Client) InstallNative(packages []string) error {
	idx, err := c.LoadIndex()
	if err != nil {
		return err
	}

	caskSet := make(map[string]struct{}, len(idx.Casks))
	for _, cask := range idx.Casks {
		caskSet[cask.Token] = struct{}{}
	}

	// Split packages into casks and formulae
	var casks, formulae []string
	for _, pkg := range packages {
		if _, ok := caskSet[pkg]; ok {
			casks = append(casks, pkg)
		} else {
			formulae = append(formulae, pkg)
		}
	}

	// Install formulae using native bottle installation
	if len(formulae) > 0 {
		if err := c.installFormulaeWithIndex(formulae, idx); err != nil {
			return err
		}
	}

	// Install casks using native installer
	if len(casks) > 0 {
		fmt.Printf("🍷 Installing casks: %v\n", casks)
		installer := NewCaskInstaller(c)
		installer.SetOperation(MutationOperationInstall)
		for _, cask := range casks {
			if err := installer.Install(cask, c.ProgressManager); err != nil {
				return fmt.Errorf("cask installation failed for %s: %w", cask, err)
			}
		}
		fmt.Println("✅ Casks installed successfully")
	}

	c.notifyInvalidation(EventInstalledChanged)
	return nil
}

// installFormulae handles formula installation via bottles
func (c *Client) installFormulaeWithIndex(packages []string, idx *Index) error {
	fmt.Println("🔍 Resolving dependencies from API...")

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
		c.emitMutation(MutationOperationInstall, name, MutationPhaseMetadata, MutationStatusQueued, "metadata queued", 0, 0, "")
		fetchWg.Add(1)
		go func(n string) {
			defer fetchWg.Done()
			fetchSem <- struct{}{}
			defer func() { <-fetchSem }()

			c.emitMutation(MutationOperationInstall, n, MutationPhaseMetadata, MutationStatusRunning, "fetching metadata", 0, 0, "")
			f, err := retry.WithResult(ctx, func() (*RemoteFormula, error) {
				return c.FetchFormula(n)
			})
			if err != nil {
				c.emitMutation(MutationOperationInstall, n, MutationPhaseMetadata, MutationStatusFailed, err.Error(), 0, 0, "")
			} else {
				c.emitMutation(MutationOperationInstall, n, MutationPhaseMetadata, MutationStatusSucceeded, "metadata ready", 0, 0, "")
			}
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
	for _, f := range installQueue {
		c.emitMutation(MutationOperationInstall, f.Name, MutationPhaseDownload, MutationStatusQueued, "download queued", 0, 0, "bytes")
	}

	fmt.Printf("📦 Found %d formulae to install.\n", len(installQueue))

	// Phase 1: Download all bottles in parallel
	fmt.Printf("⬇️  Downloading %d bottle(s) in parallel...\n", len(installQueue))

	type downloadResult struct {
		formula *RemoteFormula
		tarPath string
		err     error
	}

	dlCh := make(chan downloadResult, len(installQueue))
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.getMaxParallel())

	for _, f := range installQueue {
		wg.Add(1)
		go func(frm *RemoteFormula) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c.emitMutation(MutationOperationInstall, frm.Name, MutationPhaseDownload, MutationStatusRunning, "downloading bottle", 0, 0, "bytes")
			tarPath, err := c.DownloadBottle(frm)
			dlCh <- downloadResult{formula: frm, tarPath: tarPath, err: err}
		}(f)
	}
	wg.Wait()
	close(dlCh)

	var downloaded []downloadResult
	var dlErrors []error
	for r := range dlCh {
		if r.err != nil {
			dlErrors = append(dlErrors, fmt.Errorf("failed to download %s: %w", r.formula.Name, r.err))
			fmt.Printf("  ❌ Failed to download %s: %v\n", r.formula.Name, r.err)
			c.emitMutation(MutationOperationInstall, r.formula.Name, MutationPhaseDownload, MutationStatusFailed, r.err.Error(), 0, 0, "bytes")
		} else {
			downloaded = append(downloaded, r)
			fmt.Printf("  ✅ Downloaded %s\n", r.formula.Name)
			c.emitMutation(MutationOperationInstall, r.formula.Name, MutationPhaseDownload, MutationStatusSucceeded, "downloaded bottle", 0, 0, "bytes")
		}
	}

	if len(downloaded) == 0 {
		return fmt.Errorf("%d package(s) failed to download", len(dlErrors))
	}

	// Phase 2: Extract bottles (limited concurrency for disk safety)
	fmt.Printf("📦 Extracting %d bottle(s)...\n", len(downloaded))

	type extractResult struct {
		formula *RemoteFormula
		err     error
	}

	exCh := make(chan extractResult, len(downloaded))
	extractSem := make(chan struct{}, 2) // limit extraction concurrency

	for _, dl := range downloaded {
		wg.Add(1)
		go func(d downloadResult) {
			defer wg.Done()
			extractSem <- struct{}{}
			defer func() { <-extractSem }()
			c.emitMutation(MutationOperationInstall, d.formula.Name, MutationPhaseExtract, MutationStatusRunning, "extracting bottle", 0, 0, "")
			err := c.ExtractAndInstallBottle(d.formula, d.tarPath)
			exCh <- extractResult{formula: d.formula, err: err}
		}(dl)
	}
	wg.Wait()
	close(exCh)

	var installErrors []error
	for r := range exCh {
		if r.err != nil {
			installErrors = append(installErrors, fmt.Errorf("failed to extract %s: %w", r.formula.Name, r.err))
			fmt.Printf("  ❌ Failed to extract %s: %v\n", r.formula.Name, r.err)
			c.emitMutation(MutationOperationInstall, r.formula.Name, MutationPhaseExtract, MutationStatusFailed, r.err.Error(), 0, 0, "")
		} else {
			fmt.Printf("  ✅ Extracted %s\n", r.formula.Name)
			c.emitMutation(MutationOperationInstall, r.formula.Name, MutationPhaseExtract, MutationStatusSucceeded, "extracted bottle", 0, 0, "")
		}
	}

	allErrors := append(dlErrors, installErrors...)
	if len(allErrors) > 0 {
		for _, e := range allErrors {
			fmt.Printf("  ⚠️  %v\n", e)
		}
		if len(downloaded) == len(dlErrors)+len(installErrors) {
			return fmt.Errorf("%d package(s) failed to install", len(allErrors))
		}
	}

	var linkQueue []*RemoteFormula
	var kegOnlyQueue []*RemoteFormula
	for _, f := range installQueue {
		if f.KegOnly {
			kegOnlyQueue = append(kegOnlyQueue, f)
		} else {
			linkQueue = append(linkQueue, f)
		}
	}

	for _, f := range kegOnlyQueue {
		c.emitMutation(MutationOperationInstall, f.Name, MutationPhaseLink, MutationStatusRunning, "linking keg-only package", 0, 0, "")
		optDir := filepath.Join(c.Prefix, "opt")
		optLink := filepath.Join(optDir, f.Name)
		os.MkdirAll(optDir, 0755)
		if existing, err := os.Lstat(optLink); err == nil {
			if existing.Mode()&os.ModeSymlink != 0 {
				os.Remove(optLink)
			}
		}
		cellarPath := filepath.Join(c.Prefix, "Cellar", f.Name, f.Versions.Stable)
		if err := os.Symlink(cellarPath, optLink); err != nil {
			c.emitMutation(MutationOperationInstall, f.Name, MutationPhaseLink, MutationStatusFailed, err.Error(), 0, 0, "")
			continue
		}
		c.emitMutation(MutationOperationInstall, f.Name, MutationPhaseLink, MutationStatusSucceeded, "keg-only link ready", 0, 0, "")
		fmt.Printf("  🔗 %s (keg-only) → opt/%s\n", f.Name, f.Name)
	}

	fmt.Println("🔗 Linking binaries...")
	if err := c.linkParallel(linkQueue, MutationOperationInstall); err != nil {
		return err
	}

	return nil
}

// installFormulae handles formula installation via bottles
func (c *Client) installFormulae(packages []string) error {
	idx, err := c.LoadIndex()
	if err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}
	return c.installFormulaeWithIndex(packages, idx)
}

func (c *Client) linkParallel(installQueue []*RemoteFormula, operation string) error {
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
		fmt.Printf("  🔗 Linking %d packages...\n", len(parallelQueue))

		for _, f := range parallelQueue {
			c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusRunning, "linking package", 0, 0, "")
			result, err := c.Link(f.Name, f.Versions.Stable)
			if err != nil {
				fmt.Printf("  ❌ Failed to link %s: %v\n", f.Name, err)
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusFailed, err.Error(), 0, 0, "")
				continue
			}
			if result.Success {
				fmt.Printf("  ✅ Linked %s\n", f.Name)
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusSucceeded, "linked successfully", 0, 0, "")
			} else {
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusFailed, "link completed with errors", 0, 0, "")
			}
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
			c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusRunning, "linking package", 0, 0, "")
			result, err := c.Link(f.Name, f.Versions.Stable)
			if err != nil {
				fmt.Printf("  ❌ Failed to link %s: %v\n", f.Name, err)
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusFailed, err.Error(), 0, 0, "")
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
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusSucceeded, "linked successfully", 0, 0, "")
			} else {
				c.emitMutation(operation, f.Name, MutationPhaseLink, MutationStatusFailed, "link completed with errors", 0, 0, "")
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

// UpgradeParallel identifies outdated packages and upgrades them natively
func (c *Client) UpgradeParallel(packages []string) error {
	fmt.Println("🔍 Checking for outdated packages...")
	outdated, err := c.GetOutdated()
	if err != nil {
		return err
	}

	if len(outdated) == 0 {
		fmt.Println("✅ All packages are up to date.")
		return nil
	}

	var outdatedNames []string
	for _, pkg := range outdated {
		outdatedNames = append(outdatedNames, pkg.Name)
	}

	fmt.Printf("📦 Found %d packages to upgrade. Fetching in parallel...\n", len(outdatedNames))

	var wg sync.WaitGroup
	sem := make(chan struct{}, c.getMaxParallel())
	fetchErrChan := make(chan error, len(outdatedNames))
	for _, pkg := range outdatedNames {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fmt.Printf("  ⬇️  Fetching update for %s...\n", p)
			if err := c.Fetch(p); err != nil {
				fetchErrChan <- fmt.Errorf("failed to fetch %s: %w", p, err)
			}
		}(pkg)
	}
	wg.Wait()
	close(fetchErrChan)

	for err := range fetchErrChan {
		return err
	}

	return c.UpgradeNative(nil, outdated)
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
