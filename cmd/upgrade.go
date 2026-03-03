package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fastbrew/internal/daemon"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [package...]",
	Short: "Upgrade packages with parallel fetching",
	Run: func(cmd *cobra.Command, args []string) {
		pinned, _ := loadPinnedPackages()
		pinnedList := make([]string, 0, len(pinned))
		for name := range pinned {
			pinnedList = append(pinnedList, name)
		}

		if ran, err := tryRunMutationJob("upgrade", daemon.JobOperationUpgrade, args, daemon.JobSubmitOptions{Pinned: pinnedList}); ran {
			if err != nil {
				fmt.Printf("Error upgrading: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Upgrade complete!")
			return
		}

		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		cfg := config.Get()
		client.MaxParallel = cfg.GetParallelDownloads()

		var outdated []brew.OutdatedPackage

		if len(args) > 0 {
			outdated, err = client.GetOutdatedForPackages(args)
			if err != nil {
				fmt.Printf("Error checking outdated: %v\n", err)
				os.Exit(1)
			}
		} else {
			outdated, err = client.GetOutdated()
			if err != nil {
				fmt.Printf("Error checking outdated: %v\n", err)
				os.Exit(1)
			}
		}

		if len(pinned) > 0 {
			var filtered []brew.OutdatedPackage
			for _, pkg := range outdated {
				if pinned[pkg.Name] {
					fmt.Printf("⏭️  Skipping pinned package: %s\n", pkg.Name)
					continue
				}
				filtered = append(filtered, pkg)
			}
			outdated = filtered
		}

		if len(outdated) == 0 {
			fmt.Println("✅ All packages up to date or pinned.")
			return
		}

		if err := client.UpgradeNative(nil, outdated); err != nil {
			fmt.Printf("Error upgrading: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Upgrade complete!")
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}
