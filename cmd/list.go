package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packages (native fast scan)",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		packages, err := client.ListInstalledNative()
		if err != nil {
			fmt.Printf("Error listing packages: %v\n", err)
			os.Exit(1)
		}

		if len(packages) == 0 {
			fmt.Println("No packages installed.")
			return
		}

		for _, pkg := range packages {
			fmt.Printf("%s %s\n", pkg.Name, pkg.Version)
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
