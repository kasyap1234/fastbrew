package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
					if name != "formula.json.zst" && name != "cask.json.zst" &&
						name != "search.gob.zst" && name != "prefix_index.gob" &&
						!strings.HasSuffix(name, ".fastbrew-resume") {
						os.Remove(filepath.Join(cacheDir, name))
					}
				}
			}
		}

		fmt.Println("🔗 Checking for broken symlinks...")
		linkDirs := []string{"bin", "sbin", "lib", "include", "share", "etc", "opt"}
		brokenCount := 0
		for _, dir := range linkDirs {
			dirPath := filepath.Join(client.Prefix, dir)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				continue
			}
			filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				linfo, lerr := os.Lstat(path)
				if lerr != nil {
					return nil
				}
				if linfo.Mode()&os.ModeSymlink != 0 {
					if _, serr := os.Stat(path); serr != nil {
						fmt.Printf("  🗑️  Removing broken symlink: %s\n", path)
						os.Remove(path)
						brokenCount++
					}
				}
				return nil
			})
		}
		if brokenCount > 0 {
			fmt.Printf("  Removed %d broken symlink(s)\n", brokenCount)
		}

		fmt.Println("✅ Cleanup complete!")
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}
