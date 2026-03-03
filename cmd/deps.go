package cmd

import (
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
		var deps []string
		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			daemonDeps, err := daemonClient.Deps(args)
			if err == nil {
				deps = daemonDeps
			} else {
				warnDaemonFallback("deps", err)
			}
		} else if daemonErr != nil {
			warnDaemonFallback("deps", daemonErr)
		}

		if deps == nil {
			client, err := newBrewClient()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			localDeps, depsErr := client.ResolveDeps(args)
			if depsErr != nil {
				fmt.Printf("Error resolving dependencies: %v\n", depsErr)
				os.Exit(1)
			}
			deps = localDeps
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
