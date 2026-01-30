package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

		prefix := os.Getenv("HOMEBREW_PREFIX")
		if prefix == "" {
			if _, err := os.Stat("/opt/homebrew"); err == nil {
				prefix = "/opt/homebrew"
			} else if _, err := os.Stat("/usr/local"); err == nil {
				prefix = "/usr/local"
			} else if _, err := os.Stat("/home/linuxbrew/.linuxbrew"); err == nil {
				prefix = "/home/linuxbrew/.linuxbrew"
			}
		}

		if prefix == "" {
			out, err := exec.Command("brew", "--prefix").Output()
			if err == nil {
				prefix = strings.TrimSpace(string(out))
			}
		}

		if prefix == "" {
			fmt.Fprintln(os.Stderr, "Error: Could not determine Homebrew prefix")
			os.Exit(1)
		}

		binPath := filepath.Join(prefix, "bin")
		manPath := filepath.Join(prefix, "share/man")

		if shell == "fish" {
			fmt.Printf("set -gx PATH %s $PATH\n", binPath)
			fmt.Printf("set -gx MANPATH %s $MANPATH\n", manPath)
		} else {
			fmt.Printf("export PATH=\"%s:$PATH\"\n", binPath)
			fmt.Printf("export MANPATH=\"%s:$MANPATH\"\n", manPath)
		}
	},
}

func init() {
	rootCmd.AddCommand(shCmd)
	shCmd.Flags().BoolVar(&shAuto, "auto", false, "Auto-detect shell from $SHELL")
}
