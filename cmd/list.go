package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packages (native fast scan)",
	Run: func(cmd *cobra.Command, args []string) {
		var packages []PackageListView

		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			daemonPackages, err := daemonClient.ListInstalled()
			if err == nil {
				packages = make([]PackageListView, len(daemonPackages))
				for i, pkg := range daemonPackages {
					packages[i] = PackageListView{Name: pkg.Name, Version: pkg.Version}
				}
			} else {
				warnDaemonFallback("list", err)
			}
		} else if daemonErr != nil {
			warnDaemonFallback("list", daemonErr)
		}

		if packages == nil {
			client, err := newBrewClient()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			localPackages, listErr := client.ListInstalledNative()
			if listErr != nil {
				fmt.Printf("Error listing packages: %v\n", listErr)
				os.Exit(1)
			}
			packages = make([]PackageListView, len(localPackages))
			for i, pkg := range localPackages {
				packages[i] = PackageListView{Name: pkg.Name, Version: pkg.Version}
			}
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

type PackageListView struct {
	Name    string
	Version string
}

func init() {
	rootCmd.AddCommand(listCmd)
}
