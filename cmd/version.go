package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "0.3.2"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of FastBrew",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("FastBrew version %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
