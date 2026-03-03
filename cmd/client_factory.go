package cmd

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fastbrew/internal/daemon"
	"fmt"
	"os"
	"sync"
)

var daemonWarmupOnce sync.Once

func newBrewClient() (*brew.Client, error) {
	client, err := brew.NewClient()
	if err != nil {
		return nil, err
	}

	cfg := config.Get()
	client.MaxParallel = cfg.GetParallelDownloads()
	if cfg.Verbose {
		client.Verbose = true
	}
	client.SetInvalidationHook(notifyDaemonInvalidation)

	return client, nil
}

func newTapManager() (*brew.TapManager, error) {
	manager, err := brew.NewTapManager()
	if err != nil {
		return nil, err
	}
	manager.SetInvalidationHook(notifyDaemonInvalidation)
	return manager, nil
}

func getDaemonClientForRead() (*daemon.Client, error) {
	cfg := config.Get()
	if !cfg.Daemon.Enabled {
		return nil, nil
	}

	client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
	if err := client.Ping(); err == nil {
		return client, nil
	}

	if cfg.Daemon.AutoStart {
		if err := startDaemonProcess(true); err == nil {
			if pingErr := client.Ping(); pingErr == nil {
				if cfg.Daemon.Prewarm {
					daemonWarmupOnce.Do(func() {
						go func() {
							_ = client.Warmup()
						}()
					})
				}
				return client, nil
			}
		}
	}

	return nil, fmt.Errorf("%w (socket: %s)", daemon.ErrUnavailable, cfg.GetDaemonSocketPath())
}

func warnDaemonFallback(commandName string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "⚠️  daemon fallback for %s: %v\n", commandName, err)
}

func notifyDaemonInvalidation(event string) {
	cfg := config.Get()
	if !cfg.Daemon.Enabled {
		return
	}

	client := daemon.NewClient(cfg.GetDaemonSocketPath(), Version)
	_ = client.Invalidate(event)
}
