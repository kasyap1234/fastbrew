package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [package...]",
	Short: "Uninstall packages (native fast removal)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for _, pkg := range args {
			pkgPath := filepath.Join(client.Cellar, pkg)

			// Check if package exists
			if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
				fmt.Printf("⚠️  %s is not installed\n", pkg)
				continue
			}

			// Remove from Cellar
			if err := os.RemoveAll(pkgPath); err != nil {
				fmt.Printf("❌ Error removing %s: %v\n", pkg, err)
				continue
			}

			// Unlink (best effort - ignore errors)
			client.Unlink(pkg)

			fmt.Printf("✅ Uninstalled %s\n", pkg)
		}
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
