package cmd

import (
	"fastbrew/internal/tui"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive FastBrew dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		if err := tui.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
