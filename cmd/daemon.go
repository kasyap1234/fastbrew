package cmd

import (
	"errors"
	"fastbrew/internal/config"
	"fastbrew/internal/daemon"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage fastbrewd background process",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start fastbrewd in the background",
	Run: func(cmd *cobra.Command, args []string) {
		if err := startDaemonProcess(false); err != nil {
			fmt.Printf("Error starting daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ fastbrewd started")
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
		if err := client.Shutdown(); err != nil {
			fmt.Printf("Error stopping daemon: %v\n", err)
			os.Exit(1)
		}

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := client.Status(); err != nil {
				fmt.Println("✅ fastbrewd stopped")
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

		fmt.Println("⚠️  stop signal sent, daemon may still be shutting down")
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
		status, err := client.Status()
		if err != nil {
			fmt.Printf("fastbrewd: stopped (%v)\n", err)
			return
		}

		fmt.Println("fastbrewd: running")
		fmt.Printf("pid: %d\n", status.PID)
		fmt.Printf("socket: %s\n", status.SocketPath)
		fmt.Printf("started: %s\n", status.StartedAt.Format(time.RFC3339))
		fmt.Printf("last activity: %s\n", status.LastActivityAt.Format(time.RFC3339))
		fmt.Printf("idle timeout: %ds\n", status.IdleTimeoutSecs)
	},
}

var daemonStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show daemon cache and request statistics",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
		stats, err := client.Stats()
		if err != nil {
			fmt.Printf("Error reading daemon stats: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("uptime_seconds: %d\n", stats.UptimeSeconds)
		fmt.Printf("requests_total: %d\n", stats.RequestsTotal)
		fmt.Printf("cache_hits: %d\n", stats.CacheHits)
		fmt.Printf("cache_misses: %d\n", stats.CacheMisses)
		if stats.LastWarmupAt != nil {
			fmt.Printf("last_warmup_at: %s\n", stats.LastWarmupAt.Format(time.RFC3339))
		}

		fmt.Printf("installed_cached: %t\n", stats.InstalledCached)
		fmt.Printf("outdated_cached: %t\n", stats.OutdatedCached)
		fmt.Printf("leaves_cached: %t\n", stats.LeavesCached)
		fmt.Printf("search_entries: %d\n", stats.SearchEntries)
		fmt.Printf("deps_entries: %d\n", stats.DepsCacheEntries)
		fmt.Printf("tap_entries: %d\n", stats.TapCacheEntries)
		fmt.Printf("services_entries: %d\n", stats.ServicesEntries)
		fmt.Printf("formula_meta_entries: %d\n", stats.FormulaMetaEntries)
		fmt.Printf("cask_meta_entries: %d\n", stats.CaskMetaEntries)
		fmt.Printf("jobs_total: %d\n", stats.JobsTotal)
		fmt.Printf("jobs_running: %d\n", stats.JobsRunning)
		fmt.Printf("jobs_failed: %d\n", stats.JobsFailed)
	},
}

var daemonWarmupCmd = &cobra.Command{
	Use:   "warmup",
	Short: "Warm daemon caches (index/prefix/installed snapshot)",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
		if err := client.Warmup(); err != nil {
			fmt.Printf("Error warming daemon cache: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ daemon warmup complete")
	},
}

var daemonServeCmd = &cobra.Command{
	Use:    "serve",
	Short:  "Run daemon in foreground",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		server, err := daemon.NewServer(daemon.ServerOptions{
			SocketPath:    cfg.GetDaemonSocketPath(),
			IdleTimeout:   cfg.GetDaemonIdleTimeout(),
			BinaryVersion: Version,
			Prewarm:       cfg.Daemon.Prewarm,
		})
		if err != nil {
			fmt.Printf("Error initializing daemon: %v\n", err)
			os.Exit(1)
		}
		if err := server.ServeUntilInterrupted(); err != nil {
			fmt.Printf("Daemon exited with error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonStatsCmd)
	daemonCmd.AddCommand(daemonWarmupCmd)
	daemonCmd.AddCommand(daemonServeCmd)
	rootCmd.AddCommand(daemonCmd)
}

func startDaemonProcess(quiet bool) error {
	cfg := config.Get()
	client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
	if err := client.Ping(); err == nil {
		return nil
	}

	runDir := filepath.Dir(cfg.GetDaemonSocketPath())
	if err := os.MkdirAll(runDir, 0700); err != nil {
		return err
	}

	logPath := filepath.Join(runDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exePath, "daemon", "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		cmd.Dir = cwd
	}
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := client.Ping(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	statusErr := errors.New("daemon did not become ready in time")
	if !quiet {
		return fmt.Errorf("%w; check %s", statusErr, logPath)
	}
	return statusErr
}
