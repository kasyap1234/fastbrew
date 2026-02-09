package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	ParallelDownloads int  `json:"parallel_downloads"`
	ShowProgress      bool `json:"show_progress"`
	AutoCleanup       bool `json:"auto_cleanup"`
	Verbose           bool `json:"verbose"`
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
	}
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
