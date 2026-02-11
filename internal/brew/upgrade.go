package brew

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
)

// UpgradeNative performs native upgrades using bottle installation for formulae
// and brew upgrade --cask for casks
func (c *Client) UpgradeNative(packages []string) error {
	outdated, err := c.GetOutdated()
	if err != nil {
		return err
	}

	if len(packages) > 0 {
		reqMap := make(map[string]bool)
		for _, name := range packages {
			reqMap[name] = true
		}
		var filtered []OutdatedPackage
		for _, pkg := range outdated {
			if reqMap[pkg.Name] {
				filtered = append(filtered, pkg)
			}
		}
		outdated = filtered
	}

	if len(outdated) == 0 {
		fmt.Println("✅ All packages up to date.")
		return nil
	}

	var caskOutdated, formulaOutdated []OutdatedPackage
	for _, pkg := range outdated {
		if pkg.IsCask {
			caskOutdated = append(caskOutdated, pkg)
		} else {
			formulaOutdated = append(formulaOutdated, pkg)
		}
	}

	if len(formulaOutdated) > 0 {
		if err := c.upgradeFormulae(formulaOutdated); err != nil {
			return err
		}
	}

	if len(caskOutdated) > 0 {
		fmt.Printf("\n🍷 Upgrading %d cask(s)...\n", len(caskOutdated))
		caskNames := make([]string, len(caskOutdated))
		for i, pkg := range caskOutdated {
			caskNames[i] = pkg.Name
			fmt.Printf("  %s %s → %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
		}
		args := append([]string{"upgrade", "--cask"}, caskNames...)
		cmd := exec.Command("brew", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cask upgrade failed: %w", err)
		}
	}

	return nil
}

type downloadResult struct {
	formula *RemoteFormula
	tarPath string
	err     error
}

// upgradeFormulae handles formula upgrades via bottles with clean phased output
func (c *Client) upgradeFormulae(outdated []OutdatedPackage) error {
	// Phase 1: Fetch metadata
	fmt.Printf("🔍 Fetching formula metadata for %d package(s)...\n", len(outdated))

	type metaResult struct {
		pkg    OutdatedPackage
		remote *RemoteFormula
		err    error
	}

	metaCh := make(chan metaResult, len(outdated))
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.getMaxParallel())

	for _, pkg := range outdated {
		wg.Add(1)
		go func(p OutdatedPackage) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			remote, err := c.FetchFormula(p.Name)
			metaCh <- metaResult{pkg: p, remote: remote, err: err}
		}(pkg)
	}
	wg.Wait()
	close(metaCh)

	nameToOutdated := make(map[string]OutdatedPackage, len(outdated))
	for _, pkg := range outdated {
		nameToOutdated[pkg.Name] = pkg
	}

	var formulae []*RemoteFormula
	var metaErrors []string
	for r := range metaCh {
		if r.err != nil {
			metaErrors = append(metaErrors, fmt.Sprintf("%s: %v", r.pkg.Name, r.err))
		} else {
			formulae = append(formulae, r.remote)
		}
	}

	sort.Slice(formulae, func(i, j int) bool {
		return formulae[i].Name < formulae[j].Name
	})

	if len(metaErrors) > 0 {
		for _, e := range metaErrors {
			fmt.Printf("  ⚠️  %s\n", e)
		}
	}

	if len(formulae) == 0 {
		return nil
	}

	// Print upgrade plan
	fmt.Printf("\n📦 %d formula(e) to upgrade:\n", len(formulae))
	for _, f := range formulae {
		if pkg, ok := nameToOutdated[f.Name]; ok {
			fmt.Printf("  %s %s → %s\n", f.Name, pkg.CurrentVersion, f.Versions.Stable)
		} else {
			fmt.Printf("  %s → %s\n", f.Name, f.Versions.Stable)
		}
	}

	// Phase 2: Download all bottles in parallel
	fmt.Printf("\n⬇️  Downloading %d bottle(s)...\n", len(formulae))

	dlCh := make(chan downloadResult, len(formulae))

	for _, f := range formulae {
		wg.Add(1)
		go func(frm *RemoteFormula) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tarPath, err := c.DownloadBottle(frm)
			dlCh <- downloadResult{formula: frm, tarPath: tarPath, err: err}
		}(f)
	}
	wg.Wait()
	close(dlCh)

	var downloaded []downloadResult
	var dlErrors []downloadResult
	for r := range dlCh {
		if r.err != nil {
			dlErrors = append(dlErrors, r)
		} else {
			downloaded = append(downloaded, r)
		}
	}

	sort.Slice(downloaded, func(i, j int) bool {
		return downloaded[i].formula.Name < downloaded[j].formula.Name
	})

	if len(dlErrors) > 0 {
		for _, r := range dlErrors {
			fmt.Printf("  ❌ %s: %v\n", r.formula.Name, r.err)
		}
	}

	fmt.Printf("  ✅ %d downloaded", len(downloaded))
	if len(dlErrors) > 0 {
		fmt.Printf(", %d failed", len(dlErrors))
	}
	fmt.Println()

	if len(downloaded) == 0 {
		return fmt.Errorf("%d package(s) failed to download", len(dlErrors))
	}

	// Phase 3: Extract all bottles in parallel
	fmt.Printf("\n📦 Extracting %d bottle(s)...\n", len(downloaded))

	type extractResult struct {
		formula *RemoteFormula
		err     error
	}

	exCh := make(chan extractResult, len(downloaded))

	for _, dl := range downloaded {
		wg.Add(1)
		go func(d downloadResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := c.ExtractAndInstallBottle(d.formula, d.tarPath)
			exCh <- extractResult{formula: d.formula, err: err}
		}(dl)
	}
	wg.Wait()
	close(exCh)

	var extracted []*RemoteFormula
	var exErrors []extractResult
	for r := range exCh {
		if r.err != nil {
			exErrors = append(exErrors, r)
		} else {
			extracted = append(extracted, r.formula)
		}
	}

	if len(exErrors) > 0 {
		for _, r := range exErrors {
			fmt.Printf("  ❌ %s: %v\n", r.formula.Name, r.err)
		}
	}

	fmt.Printf("  ✅ %d extracted", len(extracted))
	if len(exErrors) > 0 {
		fmt.Printf(", %d failed", len(exErrors))
	}
	fmt.Println()

	if len(extracted) == 0 {
		totalFailed := len(dlErrors) + len(exErrors)
		return fmt.Errorf("%d package(s) failed to upgrade", totalFailed)
	}

	// Phase 4: Link
	fmt.Println("\n🔗 Linking binaries...")
	if err := c.linkParallel(extracted); err != nil {
		return err
	}

	totalFailed := len(dlErrors) + len(exErrors)
	if totalFailed > 0 {
		return fmt.Errorf("%d package(s) failed to upgrade", totalFailed)
	}

	return nil
}

func filterByNames(installed []PackageInfo, requested []string) []PackageInfo {
	var filtered []PackageInfo
	reqMap := make(map[string]bool)
	for _, name := range requested {
		reqMap[name] = true
	}

	for _, pkg := range installed {
		if reqMap[pkg.Name] {
			filtered = append(filtered, pkg)
		}
	}
	return filtered
}
