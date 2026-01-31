package cmd

import (
	"encoding/json"
	"fastbrew/internal/config"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify fastbrew configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		fmt.Printf("\nConfig file: %s\n", config.GetConfigPath())
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		key, value := args[0], args[1]

		switch key {
		case "parallel_downloads":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				fmt.Println("Error: parallel_downloads must be a positive integer")
				os.Exit(1)
			}
			cfg.ParallelDownloads = n
		case "show_progress":
			cfg.ShowProgress = value == "true" || value == "1"
		case "auto_cleanup":
			cfg.AutoCleanup = value == "true" || value == "1"
		case "verbose":
			cfg.Verbose = value == "true" || value == "1"
		default:
			fmt.Printf("Unknown config key: %s\n", key)
			fmt.Println("Available keys: parallel_downloads, show_progress, auto_cleanup, verbose")
			os.Exit(1)
		}

		if err := cfg.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Set %s = %s\n", key, value)
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
