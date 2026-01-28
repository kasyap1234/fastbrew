package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Homebrew and FastBrew index in parallel",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("🔄 Updating Homebrew and FastBrew index...")

		var wg sync.WaitGroup
		wg.Add(2)

		// 1. Brew update
		go func() {
			defer wg.Done()
			fmt.Println("  ⬇️  Updating Homebrew core...")
			brewCmd := exec.Command("brew", "update")
			brewCmd.Run() // We don't necessarily want to block TUI with this output
			fmt.Println("  ✅ Homebrew core updated.")
		}()

		// 2. FastBrew index update
		go func() {
			defer wg.Done()
			fmt.Println("  ⬇️  Refreshing FastBrew JSON index...")
			// Logic to force refresh index
			cacheDir, _ := client.GetCacheDir()
			os.Remove(fmt.Sprintf("%s/formula.json", cacheDir))
			os.Remove(fmt.Sprintf("%s/cask.json", cacheDir))
			_, err := client.LoadIndex()
			if err != nil {
				fmt.Printf("  ❌ Failed to refresh index: %v\n", err)
			} else {
				fmt.Println("  ✅ FastBrew index refreshed.")
			}
		}()

		wg.Wait()
		fmt.Println("🚀 System up to date!")
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
