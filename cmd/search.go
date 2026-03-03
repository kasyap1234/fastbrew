package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type SearchResultView struct {
	Name   string
	Desc   string
	IsCask bool
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Instant search for packages",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		var results []SearchResultView

		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			daemonResults, err := daemonClient.Search(query)
			if err == nil {
				results = make([]SearchResultView, len(daemonResults))
				for i, item := range daemonResults {
					results[i] = SearchResultView{Name: item.Name, Desc: item.Desc, IsCask: item.IsCask}
				}
			} else {
				warnDaemonFallback("search", err)
			}
		} else if daemonErr != nil {
			warnDaemonFallback("search", daemonErr)
		}

		if results == nil {
			client, err := newBrewClient()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			localResults, searchErr := client.SearchFuzzyWithIndex(query)
			if searchErr != nil {
				fmt.Printf("Error searching: %v\n", searchErr)
				os.Exit(1)
			}
			results = make([]SearchResultView, len(localResults))
			for i, item := range localResults {
				results[i] = SearchResultView{Name: item.Name, Desc: item.Desc, IsCask: item.IsCask}
			}
		}

		fmt.Printf("🔍 Searching for '%s'...\n", query)

		if len(results) == 0 {
			fmt.Println("No matches found.")
			return
		}

		limit := 40
		if len(results) < limit {
			limit = len(results)
		}

		for i := 0; i < limit; i++ {
			item := results[i]
			emoji := "🍺"
			if item.IsCask {
				emoji = "🍷"
			}
			fmt.Printf("%s %s: %s\n", emoji, item.Name, item.Desc)
		}

		if len(results) > 40 {
			fmt.Println("... and more results")
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
