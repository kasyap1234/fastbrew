package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	reinstallBuildFromSource bool
	reinstallForce           bool
	reinstallVerbose         bool
)

var reinstallCmd = &cobra.Command{
	Use:   "reinstall [formula...]",
	Short: "Uninstall and then install a formula",
	Long:  `Reinstall a formula by first uninstalling it, then installing it again.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for _, pkg := range args {
			fmt.Printf("🔄 Reinstalling %s...\n", pkg)

			fmt.Println("  🔗 Unlinking...")
			if err := client.Unlink(pkg); err != nil && reinstallVerbose {
				fmt.Printf("  ⚠️  Unlink warning: %v\n", err)
			}

			fmt.Println("  🗑️  Removing old version...")

			formula, err := client.FetchFormula(pkg)
			if err != nil {
				fmt.Printf("  ❌ Error fetching formula: %v\n", err)
				continue
			}

			fmt.Println("  📦 Installing...")
			if err := client.InstallBottle(formula); err != nil {
				fmt.Printf("  ❌ Error installing: %v\n", err)
				continue
			}

			fmt.Println("  🔗 Linking...")
			result, err := client.Link(formula.Name, formula.Versions.Stable)
			if err != nil {
				fmt.Printf("  ❌ Error linking: %v\n", err)
				continue
			}

			if result.Success {
				fmt.Printf("  ✅ %s reinstalled successfully!\n", pkg)
			} else {
				fmt.Printf("  ⚠️  Reinstalled with %d error(s)\n", len(result.Errors))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(reinstallCmd)

	reinstallCmd.Flags().BoolVar(&reinstallBuildFromSource, "build-from-source", false, "Compile from source instead of using bottle")
	reinstallCmd.Flags().BoolVar(&reinstallForce, "force", false, "Force reinstall even if already latest")
	reinstallCmd.Flags().BoolVarP(&reinstallVerbose, "verbose", "v", false, "Show detailed output")
}
