package brew

import (
	"fmt"
	"strings"
	"sync"
)

// UpgradeNative performs native upgrades using bottle installation
func (c *Client) UpgradeNative(packages []string) error {
	installed, err := c.ListInstalledNative()
	if err != nil {
		return err
	}

	toCheck := installed
	if len(packages) > 0 {
		toCheck = filterByNames(installed, packages)
	}

	var outdated []*RemoteFormula
	var mu sync.Mutex
	var wg sync.WaitGroup

	fmt.Printf("🔍 Checking %d packages for updates...\n", len(toCheck))

	for _, pkg := range toCheck {
		wg.Add(1)
		go func(p PackageInfo) {
			defer wg.Done()
			remote, err := c.FetchFormula(p.Name)
			if err != nil {
				return // Skip on error
			}

			installedBase := p.Version
			if idx := strings.Index(p.Version, "_"); idx != -1 {
				installedBase = p.Version[:idx]
			}

			if remote.Versions.Stable != installedBase {
				mu.Lock()
				outdated = append(outdated, remote)
				mu.Unlock()
			}
		}(pkg)
	}
	wg.Wait()

	if len(outdated) == 0 {
		fmt.Println("✅ All packages up to date.")
		return nil
	}

	fmt.Printf("📦 Found %d packages to upgrade.\n", len(outdated))

	errChan := make(chan error, len(outdated))
	sem := make(chan struct{}, 10) // Concurrency limit

	fmt.Println("⬇️  Downloading bottles in parallel...")
	for _, f := range outdated {
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
	if err := c.linkParallel(outdated); err != nil {
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
