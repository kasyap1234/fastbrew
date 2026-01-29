package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove old versions of installed formulae and clear cache",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("🧹 Cleaning up old versions...")

		entries, err := os.ReadDir(client.Cellar)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				pkgDir := filepath.Join(client.Cellar, entry.Name())
				versions, err := os.ReadDir(pkgDir)
				if err != nil || len(versions) <= 1 {
					continue
				}

				// Keep latest version (alphabetical last for simplicity in MVP)
				latest := versions[len(versions)-1].Name()
				for i := 0; i < len(versions)-1; i++ {
					v := versions[i].Name()
					if v == latest {
						continue
					}
					fmt.Printf("  🗑️  Removing %s %s...\n", entry.Name(), v)
					os.RemoveAll(filepath.Join(pkgDir, v))
				}
			}
		}

		fmt.Println("🧽 Clearing cache...")
		cacheDir, err := client.GetCacheDir()
		if err == nil {
			// Don't remove formula.json/cask.json/search.gob as they are needed for performance
			// Only remove downloaded tarballs
			cacheEntries, err := os.ReadDir(cacheDir)
			if err == nil {
				for _, ce := range cacheEntries {
					if ce.IsDir() {
						continue
					}
					name := ce.Name()
					if name != "formula.json" && name != "cask.json" && name != "search.gob" {
						os.Remove(filepath.Join(cacheDir, name))
					}
				}
			}
		}

		fmt.Println("✅ Cleanup complete!")
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}
