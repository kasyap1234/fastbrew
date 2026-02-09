package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fastbrew/internal/progress"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var showProgress bool
var installVerbose bool

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

		cfg := config.Get()
		client.Verbose = installVerbose || cfg.Verbose
		client.MaxParallel = cfg.GetParallelDownloads()

		if showProgress {
			client.EnableProgress()
			defer client.DisableProgress()
			go displayProgress(client.ProgressManager)
		}

		fmt.Printf("🚀 FastBrew installing: %v\n", args)
		if err := client.InstallNative(args); err != nil {
			fmt.Printf("Error installing packages: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Done!")
	},
}

func displayProgress(pm *progress.Manager) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		agg := pm.GetAggregateProgress()
		if agg.TotalDownloads == 0 {
			continue
		}

		if agg.OverallPercentage > 0 && agg.OverallPercentage < 100 {
			speedMB := agg.AverageSpeed / (1024 * 1024)
			fmt.Printf("\r  📊 Progress: %.1f%% | Active: %d | Speed: %.2f MB/s    ",
				agg.OverallPercentage, agg.ActiveDownloads, speedMB)
		}

		if pm.IsComplete() || agg.TotalDownloads == agg.CompletedDownloads+agg.FailedDownloads {
			fmt.Println()
			return
		}
	}
}

func init() {
	installCmd.Flags().BoolVarP(&showProgress, "progress", "p", false, "Show download progress")
	installCmd.Flags().BoolVar(&installVerbose, "verbose", false, "Show detailed output (extraction timing, etc.)")
	rootCmd.AddCommand(installCmd)
}
