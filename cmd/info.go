package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

const infoFetchMaxParallel = 6

type packageInfoResult struct {
	pkg     string
	formula *RemoteFormulaView
	err     error
}

type RemoteFormulaView struct {
	Name         string
	Stable       string
	Desc         string
	Homepage     string
	Dependencies []string
	KegOnly      bool
}

var infoCmd = &cobra.Command{
	Use:   "info [package...]",
	Short: "Display information about packages",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			packages, err := daemonClient.Info(args)
			if err == nil {
				for i, pkg := range packages {
					if i > 0 {
						fmt.Println()
					}
					fmt.Printf("🍺 %s: %s\n", pkg.Name, pkg.Version)
					if pkg.Desc != "" {
						fmt.Printf("%s\n", pkg.Desc)
					}
					if pkg.Homepage != "" {
						fmt.Printf("🌐 %s\n", pkg.Homepage)
					}
					if len(pkg.Dependencies) > 0 {
						fmt.Printf("📦 Dependencies: %s\n", strings.Join(pkg.Dependencies, ", "))
					}
					if pkg.KegOnly {
						fmt.Println("⚠️  Keg-only")
					}
				}
				return
			}
			warnDaemonFallback("info", err)
		} else if daemonErr != nil {
			warnDaemonFallback("info", daemonErr)
		}

		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		results := make([]packageInfoResult, len(args))
		sem := make(chan struct{}, infoFetchMaxParallel)
		var wg sync.WaitGroup

		for i, pkg := range args {
			wg.Add(1)
			go func(idx int, name string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				formula, fetchErr := client.FetchFormula(name)
				if fetchErr != nil {
					results[idx] = packageInfoResult{pkg: name, err: fetchErr}
					return
				}

				results[idx] = packageInfoResult{
					pkg: name,
					formula: &RemoteFormulaView{
						Name:         formula.Name,
						Stable:       formula.Versions.Stable,
						Desc:         formula.Desc,
						Homepage:     formula.Homepage,
						Dependencies: formula.Dependencies,
						KegOnly:      formula.KegOnly,
					},
				}
			}(i, pkg)
		}
		wg.Wait()

		for i, res := range results {
			if i > 0 {
				fmt.Println()
			}

			if res.err != nil {
				fmt.Printf("Error fetching %s: %v\n", res.pkg, res.err)
				continue
			}

			formula := res.formula
			fmt.Printf("🍺 %s: %s\n", formula.Name, formula.Stable)
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
