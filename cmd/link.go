package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

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
				result, err := client.LinkDryRun(pkg, "")
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
			result, err := client.Link(pkg, "")
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

func init() {
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)

	linkCmd.Flags().BoolVar(&linkOverwrite, "overwrite", false, "Overwrite existing symlinks")
	linkCmd.Flags().BoolVar(&linkForce, "force", false, "Force link even if formula is keg-only")
	linkCmd.Flags().BoolVarP(&linkDryRun, "dry-run", "n", false, "Show what would be linked without making changes")
}
