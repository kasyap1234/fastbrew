package cmd

import (
	"fastbrew/internal/brew"
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

		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error creating client: %v\n", err)
			os.Exit(1)
		}

		taps := brewfile.GetTaps()
		if len(taps) > 0 {
			fmt.Printf("📦 Tapping %d repositories...\n", len(taps))
			tapManager, err := newTapManager()
			if err != nil {
				fmt.Printf("Error initializing tap manager: %v\n", err)
				os.Exit(1)
			}
			for _, tap := range taps {
				tapRepo := fmt.Sprintf("%s/%s", tap.User, tap.Repo)
				fmt.Printf("  tap: %s\n", tapRepo)
				if err := tapManager.Tap(tapRepo, false); err != nil {
					fmt.Printf("  ⚠️  Warning: failed to tap %s: %v\n", tapRepo, err)
				}
			}
		}

		casks := brewfile.GetCasks()
		if len(casks) > 0 {
			fmt.Printf("🍷 Installing %d casks...\n", len(casks))
			installer := brew.NewCaskInstaller(client)
			for _, cask := range casks {
				if verbose {
					fmt.Printf("  Installing cask: %s\n", cask.Name)
				}
				if err := installer.Install(cask.Name, client.ProgressManager); err != nil {
					fmt.Printf("  ⚠️  Error installing cask %s: %v\n", cask.Name, err)
				} else if verbose {
					fmt.Printf("  ✅ %s installed\n", cask.Name)
				}
			}
		}

		brews := brewfile.GetBrews()
		if len(brews) > 0 {
			fmt.Printf("🍺 Installing %d formulae...\n", len(brews))
			formulae := make([]string, len(brews))
			for i, b := range brews {
				formulae[i] = b.Name
			}
			if err := client.InstallNative(formulae); err != nil {
				fmt.Printf("Error installing formulae: %v\n", err)
				os.Exit(1)
			}
		}

		masApps := brewfile.GetMasApps()
		if len(masApps) > 0 {
			fmt.Printf("📱 Found %d Mac App Store apps (not yet supported)\n", len(masApps))
			for _, mas := range masApps {
				fmt.Printf("  mas: %s (id: %d)\n", mas.Name, mas.ID)
			}
		}

		fmt.Println("✅ Bundle install complete!")
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

		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error creating client: %v\n", err)
			os.Exit(1)
		}

		installed, err := client.ListInstalledNative()
		if err != nil {
			fmt.Printf("Error listing installed: %v\n", err)
			os.Exit(1)
		}

		installedMap := make(map[string]bool)
		for _, pkg := range installed {
			installedMap[pkg.Name] = true
		}

		var missing []string

		tapManager, tapErr := newTapManager()
		if tapErr != nil {
			fmt.Printf("Error initializing tap manager: %v\n", tapErr)
			os.Exit(1)
		}

		for _, tap := range brewfile.GetTaps() {
			tapRepo := fmt.Sprintf("%s/%s", tap.User, tap.Repo)
			_, exists := tapManager.GetTap(tapRepo)
			if !exists {
				localPath := filepath.Join("/opt/homebrew/Library/Taps", tap.User, "homebrew-"+tap.Repo)
				if _, err := os.Stat(localPath); os.IsNotExist(err) {
					missing = append(missing, "tap: "+tapRepo)
				}
			}
		}

		for _, cask := range brewfile.GetCasks() {
			if !installedMap[cask.Name] {
				missing = append(missing, "cask: "+cask.Name)
			}
		}

		for _, brew := range brewfile.GetBrews() {
			if !installedMap[brew.Name] {
				missing = append(missing, "brew: "+brew.Name)
			}
		}

		if len(missing) > 0 {
			fmt.Println("❌ The following dependencies are missing:")
			for _, m := range missing {
				fmt.Printf("  %s\n", m)
			}
			os.Exit(1)
		}

		fmt.Println("✅ All dependencies are satisfied")
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
