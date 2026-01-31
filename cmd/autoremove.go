package cmd

import (
	"bufio"
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var autoremoveDryRun bool

var autoremoveCmd = &cobra.Command{
	Use:   "autoremove",
	Short: "Remove orphaned dependencies that are no longer needed",
	Long: `Identifies and removes packages that were installed as dependencies
but are no longer required by any installed formula.

A package is considered orphaned if:
- It is not a "leaf" package (i.e., it appears as a dependency of something)
- No currently installed leaf package depends on it (directly or transitively)

Use --dry-run to preview what would be removed without actually removing anything.`,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		orphans, err := findOrphanedPackages(client)
		if err != nil {
			fmt.Printf("Error finding orphaned packages: %v\n", err)
			os.Exit(1)
		}

		if len(orphans) == 0 {
			fmt.Println("✅ No orphaned packages to remove.")
			return
		}

		fmt.Printf("🔍 Found %d orphaned package(s):\n", len(orphans))
		for _, pkg := range orphans {
			fmt.Printf("  • %s\n", pkg)
		}

		if autoremoveDryRun {
			fmt.Println("\n💡 Dry run - no packages were removed.")
			fmt.Println("   Run without --dry-run to remove these packages.")
			return
		}

		// Prompt for confirmation
		fmt.Printf("\n❓ Remove %d orphaned package(s)? [y/N]: ", len(orphans))
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return
		}

		// Remove orphans
		removed := 0
		for _, pkg := range orphans {
			pkgPath := filepath.Join(client.Cellar, pkg)

			if err := os.RemoveAll(pkgPath); err != nil {
				fmt.Printf("❌ Error removing %s: %v\n", pkg, err)
				continue
			}

			// Unlink (best effort)
			client.Unlink(pkg)

			fmt.Printf("✅ Removed %s\n", pkg)
			removed++
		}

		fmt.Printf("\n🧹 Removed %d orphaned package(s).\n", removed)
	},
}

// findOrphanedPackages identifies packages that were installed as dependencies
// but are no longer needed by any installed leaf package.
func findOrphanedPackages(client *brew.Client) ([]string, error) {
	installed, err := client.ListInstalledNative()
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}

	if len(installed) == 0 {
		return nil, nil
	}

	idx, err := client.LoadIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	// Build formula lookup map
	formulaMap := make(map[string]brew.Formula)
	for _, f := range idx.Formulae {
		formulaMap[f.Name] = f
	}

	// Build set of installed package names (formulae only, not casks)
	installedSet := make(map[string]bool)
	for _, pkg := range installed {
		if !pkg.IsCask {
			installedSet[pkg.Name] = true
		}
	}

	// Step 1: Find all packages that ARE dependencies of some installed package
	// These are packages that appear in the dependency list of at least one installed package
	isDependencyOf := make(map[string][]string) // pkg -> list of packages that depend on it
	for _, pkg := range installed {
		if pkg.IsCask {
			continue
		}
		if f, ok := formulaMap[pkg.Name]; ok {
			for _, dep := range f.Dependencies {
				if installedSet[dep] {
					isDependencyOf[dep] = append(isDependencyOf[dep], pkg.Name)
				}
			}
		}
	}

	// Step 2: Find leaves - packages that are NOT dependencies of any other installed package
	leaves := make(map[string]bool)
	for _, pkg := range installed {
		if pkg.IsCask {
			continue
		}
		if len(isDependencyOf[pkg.Name]) == 0 {
			leaves[pkg.Name] = true
		}
	}

	// Step 3: Compute transitive dependencies of all leaves
	// These are packages that are actually needed
	needed := make(map[string]bool)
	var markNeeded func(name string)
	markNeeded = func(name string) {
		if needed[name] {
			return
		}
		needed[name] = true

		if f, ok := formulaMap[name]; ok {
			for _, dep := range f.Dependencies {
				if installedSet[dep] {
					markNeeded(dep)
				}
			}
		}
	}

	// Mark all leaves and their dependencies as needed
	for leaf := range leaves {
		markNeeded(leaf)
	}

	// Step 4: Find orphans - installed packages that are not needed
	var orphans []string
	for _, pkg := range installed {
		if pkg.IsCask {
			continue
		}
		if !needed[pkg.Name] {
			orphans = append(orphans, pkg.Name)
		}
	}

	return orphans, nil
}

func init() {
	autoremoveCmd.Flags().BoolVar(&autoremoveDryRun, "dry-run", false, "Show what would be removed without actually removing")
	rootCmd.AddCommand(autoremoveCmd)
}
