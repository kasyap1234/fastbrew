package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [package...]",
	Short: "Install packages with parallel downloading",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error initializing brew client: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🚀 FastBrew installing: %v\n", args)
		if err := client.InstallParallel(args); err != nil {
			fmt.Printf("Error installing packages: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Done!")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
