package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [package...]",
	Short: "Display information about packages",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for i, pkg := range args {
			if i > 0 {
				fmt.Println()
			}

			formula, err := client.FetchFormula(pkg)
			if err != nil {
				fmt.Printf("Error fetching %s: %v\n", pkg, err)
				continue
			}

			fmt.Printf("🍺 %s: %s\n", formula.Name, formula.Versions.Stable)
			if formula.Desc != "" {
				fmt.Printf("%s\n", formula.Desc)
			}
			if formula.Homepage != "" {
				fmt.Printf("🌐 %s\n", formula.Homepage)
			}
			if len(formula.Dependencies) > 0 {
				fmt.Printf("📦 Dependencies: %s\n", strings.Join(formula.Dependencies, ", "))
			}
			if formula.KegOnly {
				fmt.Println("⚠️  Keg-only")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
