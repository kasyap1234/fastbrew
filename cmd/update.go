package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update package index and tap metadata",
	Run: func(cmd *cobra.Command, args []string) {
		// Native update - refresh formula/cask index and tap metadata
		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: failed to initialize client: %v\n", err)
			os.Exit(1)
		}

		// Refresh the index
		fmt.Println("Updating package index...")
		changed, err := client.ForceRefreshIndex()
		if err != nil {
			fmt.Printf("Error: failed to refresh index: %v\n", err)
			os.Exit(1)
		}

		if changed {
			fmt.Println("Package index updated.")
		} else {
			fmt.Println("Package index already up-to-date.")
		}

		// Also update tap metadata if we have a tap manager
		tapManager, err := newTapManager()
		if err == nil && tapManager != nil {
			// List taps to refresh metadata
			_, _ = tapManager.ListTaps()
		}

		fmt.Println("Update complete.")
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
