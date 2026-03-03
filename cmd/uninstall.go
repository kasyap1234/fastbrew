package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/daemon"
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
		if ran, err := tryRunMutationJob("uninstall", daemon.JobOperationUninstall, args, daemon.JobSubmitOptions{}); ran {
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		removedAny := false
		for _, pkg := range args {
			pkgPath := filepath.Join(client.Cellar, pkg)

			if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
				fmt.Printf("⚠️  %s is not installed\n", pkg)
				continue
			}

			client.Unlink(pkg)

			optLink := filepath.Join(client.Prefix, "opt", pkg)
			if info, err := os.Lstat(optLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
				os.Remove(optLink)
			}

			if err := os.RemoveAll(pkgPath); err != nil {
				fmt.Printf("❌ Error removing %s: %v\n", pkg, err)
				continue
			}

			fmt.Printf("✅ Uninstalled %s\n", pkg)
			removedAny = true
		}

		if removedAny {
			notifyDaemonInvalidation(brew.EventInstalledChanged)
		}
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
