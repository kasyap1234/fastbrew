package cmd

import (
	"fastbrew/internal/brew"
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

		pinned, _ := loadPinnedPackages()
		if len(pinned) > 0 && len(args) == 0 {
			fmt.Printf("ℹ️  Skipping %d pinned package(s)\n", len(pinned))
		}

		var filtered []string
		for _, pkg := range args {
			if pinned[pkg] {
				fmt.Printf("⏭️  Skipping pinned package: %s\n", pkg)
				continue
			}
			filtered = append(filtered, pkg)
		}

		if len(args) > 0 && len(filtered) == 0 {
			fmt.Println("All specified packages are pinned.")
			return
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
