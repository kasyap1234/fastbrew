package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ParallelDownloads != 10 {
		t.Errorf("Expected ParallelDownloads=10, got %d", cfg.ParallelDownloads)
	}
	if cfg.ShowProgress != false {
		t.Error("Expected ShowProgress=false")
	}
	if cfg.AutoCleanup != false {
		t.Error("Expected AutoCleanup=false")
	}
	if cfg.Verbose != false {
		t.Error("Expected Verbose=false")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	resetConfigSingleton()

	testCfg := &Config{
		ParallelDownloads: 20,
		ShowProgress:      true,
		AutoCleanup:       true,
		Verbose:           true,
	}

	if err := testCfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".fastbrew", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	resetConfigSingleton()
	loaded := Load()

	if loaded.ParallelDownloads != 20 {
		t.Errorf("Expected ParallelDownloads=20, got %d", loaded.ParallelDownloads)
	}
	if loaded.ShowProgress != true {
		t.Error("Expected ShowProgress=true")
	}
	if loaded.AutoCleanup != true {
		t.Error("Expected AutoCleanup=true")
	}
	if loaded.Verbose != true {
		t.Error("Expected Verbose=true")
	}
}

func TestLoadWithMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	resetConfigSingleton()

	cfg := Load()

	if cfg.ParallelDownloads != 10 {
		t.Errorf("Expected default ParallelDownloads=10, got %d", cfg.ParallelDownloads)
	}
}

func TestGetReturnsSameInstance(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	resetConfigSingleton()

	cfg1 := Get()
	cfg2 := Get()

	if cfg1 != cfg2 {
		t.Error("Get() should return the same instance")
	}
}

func TestGetConfigPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".fastbrew", "config.json")
	actual := GetConfigPath()

	if actual != expected {
		t.Errorf("Expected %q, got %q", expected, actual)
	}
}

func TestLoadWithInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configDir := filepath.Join(tmpDir, ".fastbrew")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.json")
	os.WriteFile(configPath, []byte("invalid json{{{"), 0644)

	resetConfigSingleton()

	cfg := Load()
	if cfg.ParallelDownloads != 10 {
		t.Errorf("Expected default ParallelDownloads on invalid JSON, got %d", cfg.ParallelDownloads)
	}
}

func TestLoadWithPartialJSON(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configDir := filepath.Join(tmpDir, ".fastbrew")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.json")
	os.WriteFile(configPath, []byte(`{"parallel_downloads": 5}`), 0644)

	resetConfigSingleton()

	cfg := Load()
	if cfg.ParallelDownloads != 5 {
		t.Errorf("Expected ParallelDownloads=5, got %d", cfg.ParallelDownloads)
	}
	if cfg.ShowProgress != false {
		t.Error("Expected ShowProgress to remain default (false)")
	}
}

func resetConfigSingleton() {
	cfgOnce = sync.Once{}
	cfg = nil
}
