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

		idx, err := client.LoadIndex()
		if err != nil {
			fmt.Printf("Error loading index: %v\n", err)
			os.Exit(1)
		}

		query := strings.ToLower(args[0])
		fmt.Printf("🔍 Searching for '%s'...\n", query)

		count := 0
		for _, f := range idx.Formulae {
			if strings.Contains(strings.ToLower(f.Name), query) {
				fmt.Printf("🍺 %s: %s\n", f.Name, f.Desc)
				count++
			}
			if count > 20 {
				fmt.Println("... and more formulae")
				break
			}
		}
		
		count = 0
		for _, c := range idx.Casks {
			if strings.Contains(strings.ToLower(c.Token), query) {
				fmt.Printf("🍷 %s: %s\n", c.Token, c.Desc)
				count++
			}
			if count > 20 {
				fmt.Println("... and more casks")
				break
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
