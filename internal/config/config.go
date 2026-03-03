package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DaemonConfig struct {
	Enabled     bool   `json:"enabled"`
	AutoStart   bool   `json:"auto_start"`
	IdleTimeout string `json:"idle_timeout"`
	SocketPath  string `json:"socket_path"`
	Prewarm     bool   `json:"prewarm"`
}

type Config struct {
	ParallelDownloads int          `json:"parallel_downloads"`
	ShowProgress      bool         `json:"show_progress"`
	AutoCleanup       bool         `json:"auto_cleanup"`
	Verbose           bool         `json:"verbose"`
	Daemon            DaemonConfig `json:"daemon"`
}

var (
	cfg     *Config
	cfgOnce sync.Once
)

func DefaultConfig() *Config {
	return &Config{
		ParallelDownloads: 10,
		ShowProgress:      false,
		AutoCleanup:       false,
		Verbose:           false,
		Daemon: DaemonConfig{
			Enabled:     false,
			AutoStart:   true,
			IdleTimeout: "15m",
			SocketPath:  DefaultDaemonSocketPath(),
			Prewarm:     true,
		},
	}
}

func DefaultDaemonSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fastbrew", "run", "daemon.sock")
}

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fastbrew", "config.json")
}

func Load() *Config {
	cfgOnce.Do(func() {
		cfg = DefaultConfig()
		path := GetConfigPath()

		data, err := os.ReadFile(path)
		if err != nil {
			return
		}

		json.Unmarshal(data, cfg)
	})
	return cfg
}

func (c *Config) Save() error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func Get() *Config {
	return Load()
}

func (c *Config) GetParallelDownloads() int {
	if c.ParallelDownloads <= 0 {
		return 4
	}
	if c.ParallelDownloads > 20 {
		return 20
	}
	return c.ParallelDownloads
}

func (c *Config) GetDaemonSocketPath() string {
	if c.Daemon.SocketPath == "" {
		return DefaultDaemonSocketPath()
	}
	return c.Daemon.SocketPath
}

func (c *Config) GetDaemonIdleTimeout() time.Duration {
	d, err := time.ParseDuration(c.Daemon.IdleTimeout)
	if err != nil || d <= 0 {
		return 15 * time.Minute
	}
	return d
}
