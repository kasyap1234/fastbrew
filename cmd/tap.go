package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var tapFull bool

var tapCmd = &cobra.Command{
	Use:   "tap [user/repo]",
	Short: "Manage Homebrew taps",
	Long: `Tap management commands for Homebrew.
With no arguments, lists all taps.
With a repo argument, adds the tap.`,
	Run: func(cmd *cobra.Command, args []string) {
		tapManager, err := brew.NewTapManager()
		if err != nil {
			fmt.Printf("Error initializing tap manager: %v\n", err)
			os.Exit(1)
		}

		if len(args) == 0 {
			listTaps(tapManager)
		} else {
			addTap(tapManager, args[0], tapFull)
		}
	},
}

var untapCmd = &cobra.Command{
	Use:   "untap [user/repo]",
	Short: "Remove a Homebrew tap",
	Long:  `Removes a previously tapped repository.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tapManager, err := brew.NewTapManager()
		if err != nil {
			fmt.Printf("Error initializing tap manager: %v\n", err)
			os.Exit(1)
		}

		force, _ := cmd.Flags().GetBool("force")
		removeTap(tapManager, args[0], force)
	},
}

var tapInfoCmd = &cobra.Command{
	Use:   "tap-info [user/repo]",
	Short: "Show tap information",
	Long:  `Display detailed information about a tap including formulae and casks.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tapManager, err := brew.NewTapManager()
		if err != nil {
			fmt.Printf("Error initializing tap manager: %v\n", err)
			os.Exit(1)
		}

		installedOnly, _ := cmd.Flags().GetBool("installed")
		showTapInfo(tapManager, args[0], installedOnly)
	},
}

func init() {
	rootCmd.AddCommand(tapCmd)
	rootCmd.AddCommand(untapCmd)
	rootCmd.AddCommand(tapInfoCmd)

	tapCmd.Flags().BoolVar(&tapFull, "full", false, "Perform a full clone instead of a shallow clone")
	untapCmd.Flags().BoolP("force", "f", false, "Untap even if formulae are still installed")
	tapInfoCmd.Flags().BoolP("installed", "i", false, "Show only installed formulae from this tap")
}

func listTaps(tm *brew.TapManager) {
	taps, err := tm.ListTaps()
	if err != nil {
		fmt.Printf("Error listing taps: %v\n", err)
		os.Exit(1)
	}

	if len(taps) == 0 {
		fmt.Println("No taps installed.")
		fmt.Println("Use 'fastbrew tap user/repo' to add a tap.")
		return
	}

	fmt.Printf("Installed taps (%d):\n\n", len(taps))

	for _, tap := range taps {
		fmt.Printf("📦 %s\n", tap.Name)
		if tap.RemoteURL != "" {
			fmt.Printf("   Remote: %s\n", tap.RemoteURL)
		}
		if tap.IsCustom {
			fmt.Printf("   Type: Custom tap\n")
		}
		fmt.Println()
	}
}

func addTap(tm *brew.TapManager, repo string, full bool) {
	repo = normalizeTapRepo(repo)

	fmt.Printf("📦 Tapping %s...\n", repo)
	if full {
		fmt.Println("   (Full clone mode)")
	}

	if err := tm.Tap(repo, full); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully tapped %s\n", repo)
}

func removeTap(tm *brew.TapManager, repo string, force bool) {
	repo = normalizeTapRepo(repo)

	fmt.Printf("📦 Untapping %s...\n", repo)
	if force {
		fmt.Println("   (Force mode: ignoring installed formulae)")
	}

	if err := tm.Untap(repo, force); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully untapped %s\n", repo)
}

func showTapInfo(tm *brew.TapManager, repo string, installedOnly bool) {
	repo = normalizeTapRepo(repo)

	info, err := tm.GetTapInfo(repo, installedOnly)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📦 %s\n", info.Tap.Name)
	fmt.Println(strings.Repeat("=", 40))

	if info.Tap.RemoteURL != "" {
		fmt.Printf("Remote URL: %s\n", info.Tap.RemoteURL)
	}
	if info.Tap.LocalPath != "" {
		fmt.Printf("Local Path: %s\n", info.Tap.LocalPath)
	}
	fmt.Printf("Installed: %s\n", info.Tap.InstalledAt.Format("2006-01-02 15:04:05"))
	if info.Tap.IsCustom {
		fmt.Println("Type: Custom tap")
	}

	fmt.Println()

	if installedOnly {
		fmt.Printf("📋 Installed Formulae (%d):\n", len(info.Installed))
		if len(info.Installed) == 0 {
			fmt.Println("   No formulae from this tap are currently installed.")
		} else {
			for _, formula := range info.Installed {
				fmt.Printf("   • %s\n", formula)
			}
		}
	} else {
		fmt.Printf("📋 Formulae (%d):\n", len(info.Formulae))
		if len(info.Formulae) == 0 {
			fmt.Println("   No formulae in this tap.")
		} else if len(info.Formulae) <= 20 {
			for _, formula := range info.Formulae {
				fmt.Printf("   • %s\n", formula)
			}
		} else {
			for _, formula := range info.Formulae[:20] {
				fmt.Printf("   • %s\n", formula)
			}
			fmt.Printf("   ... and %d more\n", len(info.Formulae)-20)
		}

		if len(info.Casks) > 0 {
			fmt.Println()
			fmt.Printf("🍷 Casks (%d):\n", len(info.Casks))
			if len(info.Casks) <= 10 {
				for _, cask := range info.Casks {
					fmt.Printf("   • %s\n", cask)
				}
			} else {
				for _, cask := range info.Casks[:10] {
					fmt.Printf("   • %s\n", cask)
				}
				fmt.Printf("   ... and %d more\n", len(info.Casks)-10)
			}
		}
	}
}

func normalizeTapRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimSuffix(repo, "/")
	return repo
}
