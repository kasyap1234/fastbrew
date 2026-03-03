package cmd

import (
	"encoding/json"
	"fastbrew/internal/config"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
			cfg.ShowProgress = parseConfigBool(value)
		case "auto_cleanup":
			cfg.AutoCleanup = parseConfigBool(value)
		case "verbose":
			cfg.Verbose = parseConfigBool(value)
		case "daemon.enabled":
			cfg.Daemon.Enabled = parseConfigBool(value)
		case "daemon.auto_start":
			cfg.Daemon.AutoStart = parseConfigBool(value)
		case "daemon.idle_timeout":
			if _, err := time.ParseDuration(value); err != nil {
				fmt.Printf("Error: invalid duration for daemon.idle_timeout: %v\n", err)
				os.Exit(1)
			}
			cfg.Daemon.IdleTimeout = value
		case "daemon.socket_path":
			cfg.Daemon.SocketPath = value
		case "daemon.prewarm":
			cfg.Daemon.Prewarm = parseConfigBool(value)
		default:
			fmt.Printf("Unknown config key: %s\n", key)
			fmt.Println("Available keys: parallel_downloads, show_progress, auto_cleanup, verbose, daemon.enabled, daemon.auto_start, daemon.idle_timeout, daemon.socket_path, daemon.prewarm")
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

func parseConfigBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
