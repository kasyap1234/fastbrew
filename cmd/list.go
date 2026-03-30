package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var (
	listFormula    bool
	listCask       bool
	listVersions   bool
	listFullName   bool
	listPinned     bool
	listOnePerLine bool
	listReverse    bool
	listByTime     bool
)

var listCmd = &cobra.Command{
	Use:   "list [formula|cask...]",
	Short: "List installed packages (native fast scan)",
	Run: func(cmd *cobra.Command, args []string) {
		var packages []PackageListView

		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			daemonPackages, err := daemonClient.ListInstalled()
			if err == nil {
				packages = make([]PackageListView, len(daemonPackages))
				for i, pkg := range daemonPackages {
					packages[i] = PackageListView{
						Name:    pkg.Name,
						Version: pkg.Version,
						IsCask:  pkg.IsCask,
					}
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
				packages[i] = PackageListView{
					Name:    pkg.Name,
					Version: pkg.Version,
					IsCask:  pkg.IsCask,
				}
			}
		}

		// Filter by type if requested
		if listFormula || listCask {
			var filtered []PackageListView
			for _, pkg := range packages {
				if listFormula && !pkg.IsCask {
					filtered = append(filtered, pkg)
				} else if listCask && pkg.IsCask {
					filtered = append(filtered, pkg)
				}
			}
			packages = filtered
		}

		if len(packages) == 0 {
			fmt.Println("No packages installed.")
			return
		}

		// Sort packages
		sort.Slice(packages, func(i, j int) bool {
			if listReverse {
				return packages[i].Name > packages[j].Name
			}
			return packages[i].Name < packages[j].Name
		})

		// Output format
		for _, pkg := range packages {
			if listVersions {
				fmt.Printf("%s %s\n", pkg.Name, pkg.Version)
			} else if listOnePerLine {
				fmt.Println(pkg.Name)
			} else {
				fmt.Printf("%s %s\n", pkg.Name, pkg.Version)
			}
		}
	},
}

type PackageListView struct {
	Name    string
	Version string
	IsCask  bool
}

func init() {
	listCmd.Flags().BoolVar(&listFormula, "formula", false, "List only formulae")
	listCmd.Flags().BoolVar(&listCask, "cask", false, "List only casks")
	listCmd.Flags().BoolVar(&listVersions, "versions", false, "Show version numbers")
	listCmd.Flags().BoolVar(&listFullName, "full-name", false, "Print formulae with fully-qualified names")
	listCmd.Flags().BoolVar(&listPinned, "pinned", false, "List only pinned formulae")
	listCmd.Flags().BoolVarP(&listOnePerLine, "1", "1", false, "Force one entry per line")
	listCmd.Flags().BoolVarP(&listReverse, "reverse", "r", false, "Reverse order (oldest first)")
	listCmd.Flags().BoolVarP(&listByTime, "time", "t", false, "Sort by time modified")

	// Add ls alias for brew compatibility
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "Alias for list",
		Run:   listCmd.Run,
	}
	lsCmd.Flags().AddFlagSet(listCmd.Flags())

	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(lsCmd)
}
