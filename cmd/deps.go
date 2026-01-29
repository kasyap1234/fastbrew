package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps [package...]",
	Short: "Show dependencies for packages (fast cached lookup)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		deps, err := client.ResolveDeps(args)
		if err != nil {
			fmt.Printf("Error resolving dependencies: %v\n", err)
			os.Exit(1)
		}

		if len(deps) == 0 {
			fmt.Println("No dependencies found.")
			return
		}

		fmt.Printf("📦 Dependencies: %s\n", strings.Join(deps, ", "))
	},
}

func init() {
	rootCmd.AddCommand(depsCmd)
}
