package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [package...]",
	Short: "Upgrade packages with parallel fetching",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		cfg := config.Get()
		client.MaxParallel = cfg.GetParallelDownloads()

		pinned, _ := loadPinnedPackages()

		// Get all outdated packages once
		outdated, err := client.GetOutdated()
		if err != nil {
			fmt.Printf("Error checking outdated: %v\n", err)
			os.Exit(1)
		}

		// Filter by requested packages if specified
		if len(args) > 0 {
			reqMap := make(map[string]bool)
			for _, pkg := range args {
				reqMap[pkg] = true
			}
			var filtered []brew.OutdatedPackage
			for _, pkg := range outdated {
				if reqMap[pkg.Name] {
					filtered = append(filtered, pkg)
				}
			}
			outdated = filtered
		}

		// Filter out pinned packages
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
