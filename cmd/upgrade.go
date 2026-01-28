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

		if err := client.UpgradeParallel(args); err != nil {
			fmt.Printf("Error upgrading: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Upgrade complete!")
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}
