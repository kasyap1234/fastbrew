package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var shAuto bool

var shCmd = &cobra.Command{
	Use:   "sh",
	Short: "Print shell environment configuration",
	Long:  `Print the shell commands required to set up Homebrew in your shell environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		shell := os.Getenv("SHELL")
		if shAuto && shell != "" {
			shell = filepath.Base(shell)
		}

		if shell == "" {
			shell = "bash"
		}

		env, err := brew.GetEnvironment()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not determine Homebrew environment: %v\n", err)
			os.Exit(1)
		}

		fmt.Print(env.ShellEnv(shell))
	},
}

func init() {
	rootCmd.AddCommand(shCmd)
	shCmd.Flags().BoolVar(&shAuto, "auto", false, "Auto-detect shell from $SHELL")
}
