package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	linkOverwrite bool
	linkForce     bool
	linkDryRun    bool
)

var linkCmd = &cobra.Command{
	Use:   "link [formula...]",
	Short: "Symlink a formula's installed files into the prefix",
	Long:  `Link a formula's installed files into the Homebrew prefix, making them available in PATH.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for _, pkg := range args {
			if linkDryRun {
				fmt.Printf("Would link %s...\n", pkg)
				version, verErr := findInstalledVersion(client, pkg)
				if verErr != nil {
					fmt.Printf("  Error: %v\n", verErr)
					continue
				}
				result, err := client.LinkDryRun(pkg, version)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					continue
				}
				for _, binary := range result.Binaries {
					fmt.Printf("  -> %s\n", binary)
				}
				continue
			}

			fmt.Printf("🔗 Linking %s...\n", pkg)

			version, verErr := findInstalledVersion(client, pkg)
			if verErr != nil {
				fmt.Printf("  ❌ Error: %v\n", verErr)
				continue
			}

			result, err := client.Link(pkg, version)
			if err != nil {
				fmt.Printf("  ❌ Error: %v\n", err)
				continue
			}

			if len(result.Binaries) == 0 {
				fmt.Printf("  ℹ️  No binaries to link\n")
			} else {
				fmt.Printf("  ✅ Linked %d binary(ies)\n", len(result.Binaries))
				for _, binary := range result.Binaries {
					fmt.Printf("     • %s\n", binary)
				}
			}
		}
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink [formula...]",
	Short: "Remove symlinks for a formula from the prefix",
	Long:  `Unlink a formula's symlinks from the Homebrew prefix, removing them from PATH.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for _, pkg := range args {
			fmt.Printf("🔗 Unlinking %s...\n", pkg)
			if err := client.Unlink(pkg); err != nil {
				fmt.Printf("  ❌ Error: %v\n", err)
				continue
			}
			fmt.Printf("  ✅ Unlinked\n")
		}
	},
}

func findInstalledVersion(client *brew.Client, pkg string) (string, error) {
	pkgDir := filepath.Join(client.Cellar, pkg)
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return "", fmt.Errorf("%s is not installed", pkg)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("%s has no installed versions", pkg)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].IsDir() && !strings.HasPrefix(entries[i].Name(), ".") {
			return entries[i].Name(), nil
		}
	}
	return "", fmt.Errorf("%s has no installed versions", pkg)
}

func init() {
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)

	linkCmd.Flags().BoolVar(&linkOverwrite, "overwrite", false, "Overwrite existing symlinks")
	linkCmd.Flags().BoolVar(&linkForce, "force", false, "Force link even if formula is keg-only")
	linkCmd.Flags().BoolVarP(&linkDryRun, "dry-run", "n", false, "Show what would be linked without making changes")
}
