package cmd

import (
	"fastbrew/internal/tui"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	// Custom handling for unknown commands to fallback to brew
	if len(os.Args) > 1 {
		cmd, _, _ := rootCmd.Find(os.Args[1:])
		if cmd == rootCmd && os.Args[1] != "help" && os.Args[1] != "-h" && os.Args[1] != "--help" {
			// Not a known command, pass to brew
			handleFallback(os.Args[1:])
			return
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func handleFallback(args []string) {
	fmt.Printf("⏩ Passing to brew: brew %s\n", strings.Join(args, " "))
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func init() {
	// Flags and configuration can go here
}
