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

		var filtered []string
		if len(args) > 0 {
			for _, pkg := range args {
				if pinned[pkg] {
					fmt.Printf("⏭️  Skipping pinned package: %s\n", pkg)
					continue
				}
				filtered = append(filtered, pkg)
			}
			if len(filtered) == 0 {
				fmt.Println("All specified packages are pinned.")
				return
			}
		}

		if len(args) == 0 && len(pinned) > 0 {
			outdated, err := client.GetOutdated()
			if err != nil {
				fmt.Printf("Error checking outdated: %v\n", err)
				os.Exit(1)
			}
			for _, pkg := range outdated {
				if pinned[pkg.Name] {
					fmt.Printf("⏭️  Skipping pinned package: %s\n", pkg.Name)
					continue
				}
				filtered = append(filtered, pkg.Name)
			}
			if len(filtered) == 0 {
				fmt.Println("✅ All packages up to date or pinned.")
				return
			}
		}

		if err := client.UpgradeNative(filtered); err != nil {
			fmt.Printf("Error upgrading: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Upgrade complete!")
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}
