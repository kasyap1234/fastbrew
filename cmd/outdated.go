package cmd

import (
	"encoding/json"
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	outdatedQuiet bool
	outdatedJSON  bool
)

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "List outdated packages (faster than brew outdated)",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		outdated, err := client.GetOutdated()
		if err != nil {
			fmt.Printf("Error checking for outdated packages: %v\n", err)
			os.Exit(1)
		}

		if len(outdated) == 0 {
			os.Exit(0)
		}

		if outdatedJSON {
			output, err := json.MarshalIndent(outdated, "", "  ")
			if err != nil {
				fmt.Printf("Error encoding JSON: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(output))
		} else if outdatedQuiet {
			for _, pkg := range outdated {
				fmt.Println(pkg.Name)
			}
		} else {
			for _, pkg := range outdated {
				fmt.Printf("%s (%s) < %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
			}
		}

		os.Exit(1)
	},
}

func init() {
	outdatedCmd.Flags().BoolVarP(&outdatedQuiet, "quiet", "q", false, "Only display names of outdated packages")
	outdatedCmd.Flags().BoolVar(&outdatedJSON, "json", false, "Output in JSON format")
	rootCmd.AddCommand(outdatedCmd)
}
