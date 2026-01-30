package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system for potential problems",
	Long:  `Run comprehensive diagnostics on your Homebrew installation to identify issues and suggest fixes.`,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		doctor := brew.NewDoctor(client, verbose)
		results := doctor.RunDiagnostics()
		doctor.PrintResults(results)

		exitCode := doctor.GetExitCode(results)
		os.Exit(exitCode)
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed diagnostic output")
}
