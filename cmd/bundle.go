package cmd

import (
	"fastbrew/internal/bundle"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Manage Brewfile dependencies",
	Long:  `Install from or dump to a Brewfile.`,
}

var bundleInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install dependencies from a Brewfile",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose, _ := cmd.Flags().GetBool("verbose")

		if file == "" {
			file = findBrewfile()
		}

		if file == "" {
			fmt.Println("Error: No Brewfile found. Use --file to specify one.")
			os.Exit(1)
		}

		parser := bundle.SimpleParser()
		brewfile, err := parser.ParseFile(file)
		if err != nil {
			fmt.Printf("Error parsing Brewfile: %v\n", err)
			os.Exit(1)
		}

		if dryRun {
			fmt.Println("Would install:")
			for _, brew := range brewfile.GetBrews() {
				fmt.Printf("  brew: %s\n", brew.Name)
			}
			for _, cask := range brewfile.GetCasks() {
				fmt.Printf("  cask: %s\n", cask.Name)
			}
			for _, tap := range brewfile.GetTaps() {
				fmt.Printf("  tap: %s/%s\n", tap.User, tap.Repo)
			}
			for _, mas := range brewfile.GetMasApps() {
				fmt.Printf("  mas: %s (id: %d)\n", mas.Name, mas.ID)
			}
			return
		}

		if verbose {
			fmt.Printf("Installing from %s...\n", file)
		}

		fmt.Println("Bundle install not yet fully implemented. Parsed successfully.")
		fmt.Printf("Found %d brews, %d casks, %d taps, %d mas apps\n",
			len(brewfile.GetBrews()),
			len(brewfile.GetCasks()),
			len(brewfile.GetTaps()),
			len(brewfile.GetMasApps()),
		)
	},
}

var bundleDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Generate a Brewfile from installed packages",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")
		descriptions, _ := cmd.Flags().GetBool("describe")
		force, _ := cmd.Flags().GetBool("force")

		opts := bundle.DefaultDumpOptions()
		opts.Descriptions = descriptions

		dumper := bundle.NewDumper()
		result, err := dumper.Dump(opts)
		if err != nil {
			fmt.Printf("Error dumping packages: %v\n", err)
			os.Exit(1)
		}

		genOpts := bundle.DefaultGeneratorOptions()
		genOpts.Descriptions = descriptions
		generator := bundle.NewGenerator(genOpts)

		if file == "" || file == "-" {
			err = generator.Generate(os.Stdout, result)
			if err != nil {
				fmt.Printf("Error generating Brewfile: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if _, err := os.Stat(file); err == nil && !force {
			fmt.Printf("File %s already exists. Use --force to overwrite.\n", file)
			os.Exit(1)
		}

		f, err := os.Create(file)
		if err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		err = generator.Generate(f, result)
		if err != nil {
			fmt.Printf("Error generating Brewfile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Brewfile written to %s\n", file)
	},
}

var bundleCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if all dependencies are satisfied",
	Run: func(cmd *cobra.Command, args []string) {
		file, _ := cmd.Flags().GetString("file")

		if file == "" {
			file = findBrewfile()
		}

		if file == "" {
			fmt.Println("Error: No Brewfile found. Use --file to specify one.")
			os.Exit(1)
		}

		parser := bundle.SimpleParser()
		brewfile, err := parser.ParseFile(file)
		if err != nil {
			fmt.Printf("Error parsing Brewfile: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Bundle check not yet fully implemented. Parsed successfully.")
		fmt.Printf("Brewfile contains %d brews, %d casks, %d taps, %d mas apps\n",
			len(brewfile.GetBrews()),
			len(brewfile.GetCasks()),
			len(brewfile.GetTaps()),
			len(brewfile.GetMasApps()),
		)
	},
}

func findBrewfile() string {
	candidates := []string{
		"Brewfile",
		".Brewfile",
	}

	home, err := os.UserHomeDir()
	if err == nil {
		candidates = append(candidates, filepath.Join(home, ".Brewfile"))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func init() {
	bundleInstallCmd.Flags().String("file", "", "Path to Brewfile")
	bundleInstallCmd.Flags().Bool("dry-run", false, "Show what would be installed")
	bundleInstallCmd.Flags().Bool("verbose", false, "Verbose output")

	bundleDumpCmd.Flags().String("file", "", "Output file (default: stdout)")
	bundleDumpCmd.Flags().Bool("describe", false, "Include package descriptions as comments")
	bundleDumpCmd.Flags().Bool("force", false, "Overwrite existing file")

	bundleCheckCmd.Flags().String("file", "", "Path to Brewfile")

	bundleCmd.AddCommand(bundleInstallCmd)
	bundleCmd.AddCommand(bundleDumpCmd)
	bundleCmd.AddCommand(bundleCheckCmd)
	rootCmd.AddCommand(bundleCmd)
}
