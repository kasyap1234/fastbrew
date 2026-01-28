package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"strings"

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

		items, err := client.GetSearchIndex()
		if err != nil {
			fmt.Printf("Error loading index: %v\n", err)
			os.Exit(1)
		}

		query := strings.ToLower(args[0])
		fmt.Printf("🔍 Searching for '%s'...\n", query)

		count := 0
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Name), query) {
				if item.IsCask {
					fmt.Printf("🍷 %s: %s\n", item.Name, item.Desc)
				} else {
					fmt.Printf("🍺 %s: %s\n", item.Name, item.Desc)
				}
				count++
			}
			if count > 40 {
				fmt.Println("... and more results")
				break
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
