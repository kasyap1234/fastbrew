package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	showPrefix      bool
	showCellar      bool
	showCache       bool
	showEnv         bool
	showRepository  bool
	showVersionFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "fastbrew",
	Short: "A lightning-fast interface for Homebrew",
	Long: `FastBrew is a high-performance native implementation of Homebrew.
It provides parallel execution, instant search, and optimized package management.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Handle --* flags like brew does
		if showPrefix {
			env, err := brew.GetEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(env.Prefix)
			return
		}
		if showCellar {
			env, err := brew.GetEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(env.Cellar)
			return
		}
		if showCache {
			env, err := brew.GetEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(env.Cache)
			return
		}
		if showEnv {
			env, err := brew.GetEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			// Print environment variables in brew format
			fmt.Printf("HOMEBREW_PREFIX=%s\n", env.HomebrewPrefix)
			fmt.Printf("HOMEBREW_CELLAR=%s\n", env.HomebrewCellar)
			fmt.Printf("HOMEBREW_REPOSITORY=%s\n", env.HomebrewRepository)
			fmt.Printf("HOMEBREW_CACHE=%s\n", env.HomebrewCache)
			return
		}
		if showRepository {
			env, err := brew.GetEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(env.Repository)
			return
		}
		if showVersionFlag {
			// Just show FastBrew version - don't call brew
			fmt.Println(Version)
			return
		}

		// When no args provided, show help like brew does
		if len(args) == 0 {
			cmd.Help()
			return
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&showPrefix, "prefix", false, "Display Homebrew's install path")
	rootCmd.Flags().BoolVar(&showCellar, "cellar", false, "Display Homebrew's Cellar path")
	rootCmd.Flags().BoolVar(&showCache, "cache", false, "Display Homebrew's download cache")
	rootCmd.Flags().BoolVar(&showEnv, "env", false, "Display Homebrew's build environment")
	rootCmd.Flags().BoolVar(&showRepository, "repository", false, "Display Homebrew's repository path")
	rootCmd.Flags().BoolVar(&showVersionFlag, "version", false, "Display version")
}
