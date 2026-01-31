package brew

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// UpgradeNative performs native upgrades using bottle installation for formulae
// and brew upgrade --cask for casks
func (c *Client) UpgradeNative(packages []string) error {
	outdated, err := c.GetOutdated()
	if err != nil {
		return err
	}

	// Filter by requested packages if specified
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

	// Split into casks and formulae
	var caskOutdated, formulaOutdated []OutdatedPackage
	for _, pkg := range outdated {
		if pkg.IsCask {
			caskOutdated = append(caskOutdated, pkg)
		} else {
			formulaOutdated = append(formulaOutdated, pkg)
		}
	}

	// Upgrade formulae using native bottle installation
	if len(formulaOutdated) > 0 {
		if err := c.upgradeFormulae(formulaOutdated); err != nil {
			return err
		}
	}

	// Upgrade casks using brew upgrade --cask
	if len(caskOutdated) > 0 {
		fmt.Printf("🍷 Upgrading %d cask(s)...\n", len(caskOutdated))
		caskNames := make([]string, len(caskOutdated))
		for i, pkg := range caskOutdated {
			caskNames[i] = pkg.Name
			fmt.Printf("  ⬆️  %s: %s → %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
		}
		args := append([]string{"upgrade", "--cask"}, caskNames...)
		cmd := exec.Command("brew", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cask upgrade failed: %w", err)
		}
		fmt.Println("✅ Casks upgraded successfully")
	}

	return nil
}

// upgradeFormulae handles formula upgrades via bottles
func (c *Client) upgradeFormulae(outdated []OutdatedPackage) error {
	var outdatedFormulae []*RemoteFormula
	var mu sync.Mutex
	var wg sync.WaitGroup

	fmt.Printf("🔍 Fetching formula metadata for %d package(s)...\n", len(outdated))

	for _, pkg := range outdated {
		wg.Add(1)
		go func(p OutdatedPackage) {
			defer wg.Done()
			remote, err := c.FetchFormula(p.Name)
			if err != nil {
				return
			}
			mu.Lock()
			outdatedFormulae = append(outdatedFormulae, remote)
			mu.Unlock()
		}(pkg)
	}
	wg.Wait()

	if len(outdatedFormulae) == 0 {
		return nil
	}

	fmt.Printf("📦 Upgrading %d formulae...\n", len(outdatedFormulae))

	errChan := make(chan error, len(outdatedFormulae))
	sem := make(chan struct{}, 10)

	fmt.Println("⬇️  Downloading bottles in parallel...")
	for _, f := range outdatedFormulae {
		wg.Add(1)
		go func(frm *RemoteFormula) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("  ⬆️  Upgrading %s → %s\n", frm.Name, frm.Versions.Stable)
			if err := c.InstallBottle(frm); err != nil {
				errChan <- fmt.Errorf("failed to install %s: %w", frm.Name, err)
				fmt.Printf("  ❌ Failed %s: %v\n", frm.Name, err)
			} else {
				fmt.Printf("  ✅ Downloaded %s\n", frm.Name)
			}
		}(f)
	}
	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return fmt.Errorf("some upgrades failed, check output")
	}

	fmt.Println("🔗 Linking binaries...")
	if err := c.linkParallel(outdatedFormulae); err != nil {
		return err
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
