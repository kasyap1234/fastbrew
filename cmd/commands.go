package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "Display the list of commands",
	Run: func(cmd *cobra.Command, args []string) {
		// Get all commands
		var names []string
		for _, c := range rootCmd.Commands() {
			if c.IsAvailableCommand() && !c.Hidden {
				names = append(names, c.Name())
				// Also add aliases
				for _, alias := range c.Aliases {
					names = append(names, alias)
				}
			}
		}

		// Sort alphabetically
		sort.Strings(names)

		// Group by category (built-in, aliases, etc.)
		fmt.Println("==> Built-in commands")
		for _, name := range names {
			// Skip aliases for the main list
			if !strings.HasPrefix(name, "--") && len(name) > 2 {
				fmt.Println(name)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(commandsCmd)
}
