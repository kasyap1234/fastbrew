package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var leavesCmd = &cobra.Command{
	Use:   "leaves",
	Short: "List installed formulae that are not dependencies of another installed formula",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		installed, err := client.ListInstalledNative()
		if err != nil {
			fmt.Printf("Error listing installed: %v\n", err)
			os.Exit(1)
		}

		if len(installed) == 0 {
			return
		}

		// Map to store if a package is a dependency
		isDep := make(map[string]bool)

		// Load index to check dependencies
		// Note: This might be slow if index is missing, but usually it's cached.
		idx, err := client.LoadIndex()
		if err != nil {
			// Fallback: If we can't load index, we can't determine leaves accurately.
			// But we can try to continue with what we have if some info is available.
			fmt.Printf("Warning: Could not load index for accurate leaves: %v\n", err)
		} else {
			formulaMap := make(map[string]brew.Formula)
			for _, f := range idx.Formulae {
				formulaMap[f.Name] = f
			}

			// Check dependencies of each installed package
			for _, pkg := range installed {
				if f, ok := formulaMap[pkg.Name]; ok {
					for _, dep := range f.Dependencies {
						isDep[dep] = true
					}
				}
			}
		}

		for _, pkg := range installed {
			if !isDep[pkg.Name] {
				fmt.Println(pkg.Name)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(leavesCmd)
}
