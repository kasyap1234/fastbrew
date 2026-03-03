package cmd

import (
	"fastbrew/internal/tui"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fastbrew",
	Short: "A lightning-fast wrapper for Homebrew",
	Long: `FastBrew is a high-performance interface for Homebrew, written in Go.
It features parallel execution, a modern TUI, and zero-latency search.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := tui.Start(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Flags and configuration can go here
}
