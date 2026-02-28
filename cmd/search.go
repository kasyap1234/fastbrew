package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Instant search for packages",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		query := args[0]

		results, err := client.SearchFuzzyWithIndex(query)
		if err != nil {
			fmt.Printf("Error searching: %v\n", err)
			os.Exit(1)
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
