package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Homebrew and FastBrew index in parallel",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("🔄 Updating FastBrew index...")
		changed, err := client.ForceRefreshIndex()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if changed {
			fmt.Println("✅ Index updated!")
			return
		}
		fmt.Println("Already up-to-date.")
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
