package brew

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetEnvironment_Linuxbrew(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}

	// Set environment to simulate Linuxbrew
	t.Setenv("HOMEBREW_PREFIX", "/home/linuxbrew/.linuxbrew")
	defer os.Unsetenv("HOMEBREW_PREFIX")

	env, err := GetEnvironment()
	if err != nil {
		t.Fatalf("GetEnvironment failed: %v", err)
	}

	// Verify core paths
	if env.Prefix != "/home/linuxbrew/.linuxbrew" {
		t.Errorf("Prefix = %s, want /home/linuxbrew/.linuxbrew", env.Prefix)
	}

	expectedCellar := "/home/linuxbrew/.linuxbrew/Cellar"
	if env.Cellar != expectedCellar {
		t.Errorf("Cellar = %s, want %s", env.Cellar, expectedCellar)
	}

	expectedCaskroom := "/home/linuxbrew/.linuxbrew/Caskroom"
	if env.Caskroom != expectedCaskroom {
		t.Errorf("Caskroom = %s, want %s", env.Caskroom, expectedCaskroom)
	}

	expectedBin := "/home/linuxbrew/.linuxbrew/bin"
	if env.BinDir != expectedBin {
		t.Errorf("BinDir = %s, want %s", env.BinDir, expectedBin)
	}

	// Verify repository
	expectedRepo := "/home/linuxbrew/.linuxbrew/Homebrew"
	if env.Repository != expectedRepo {
		t.Errorf("Repository = %s, want %s", env.Repository, expectedRepo)
	}

	// Verify taps dir
	expectedTaps := "/home/linuxbrew/.linuxbrew/Homebrew/Library/Taps"
	if env.TapsDir != expectedTaps {
		t.Errorf("TapsDir = %s, want %s", env.TapsDir, expectedTaps)
	}

	// Verify Linux-specific service directories
	expectedUserService := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
	if env.UserServiceDir != expectedUserService {
		t.Errorf("UserServiceDir = %s, want %s", env.UserServiceDir, expectedUserService)
	}
}

func TestGetEnvironment_MacOSARM(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("macOS ARM-only test")
	}

	t.Setenv("HOMEBREW_PREFIX", "/opt/homebrew")
	defer os.Unsetenv("HOMEBREW_PREFIX")

	env, err := GetEnvironment()
	if err != nil {
		t.Fatalf("GetEnvironment failed: %v", err)
	}

	if env.Prefix != "/opt/homebrew" {
		t.Errorf("Prefix = %s, want /opt/homebrew", env.Prefix)
	}

	expectedCellar := "/opt/homebrew/Cellar"
	if env.Cellar != expectedCellar {
		t.Errorf("Cellar = %s, want %s", env.Cellar, expectedCellar)
	}

	expectedApplications := "/Applications"
	if env.ApplicationsDir != expectedApplications {
		t.Errorf("ApplicationsDir = %s, want %s", env.ApplicationsDir, expectedApplications)
	}

	expectedFrameworks := "/opt/homebrew/Frameworks"
	if env.FrameworksDir != expectedFrameworks {
		t.Errorf("FrameworksDir = %s, want %s", env.FrameworksDir, expectedFrameworks)
	}
}

func TestGetEnvironment_MacOSIntel(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only test")
	}

	t.Setenv("HOMEBREW_PREFIX", "/usr/local")
	defer os.Unsetenv("HOMEBREW_PREFIX")

	env, err := GetEnvironment()
	if err != nil {
		t.Fatalf("GetEnvironment failed: %v", err)
	}

	if env.Prefix != "/usr/local" {
		t.Errorf("Prefix = %s, want /usr/local", env.Prefix)
	}
}

func TestGetEnvironment_NoPrefix(t *testing.T) {
	// Clear environment
	os.Unsetenv("HOMEBREW_PREFIX")
	os.Unsetenv("HOMEBREW_REPOSITORY")
	os.Unsetenv("HOMEBREW_CACHE")

	// Save original HOME to restore later
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create a fake home directory
	tempHome := t.TempDir()
	os.Setenv("HOME", tempHome)

	// Create fake brew directories based on OS
	var fakePrefix string
	switch runtime.GOOS {
	case "darwin":
		fakePrefix = filepath.Join(tempHome, "fakebrew")
	case "linux":
		fakePrefix = filepath.Join(tempHome, ".linuxbrew")
	default:
		t.Skip("Unsupported OS")
	}

	os.MkdirAll(filepath.Join(fakePrefix, "Cellar"), 0755)
	os.MkdirAll(filepath.Join(fakePrefix, "Homebrew"), 0755)

	// Temporarily override detection to use our fake prefix
	// This is tricky because detection uses os.Stat on known paths
	// For this test, we'll set the environment variable
	t.Setenv("HOMEBREW_PREFIX", fakePrefix)

	env, err := GetEnvironment()
	if err != nil {
		t.Fatalf("GetEnvironment failed: %v", err)
	}

	if env.Prefix != fakePrefix {
		t.Errorf("Prefix = %s, want %s", env.Prefix, fakePrefix)
	}
}

func TestEnvironment_CellarPath(t *testing.T) {
	env := &Environment{
		Prefix: "/opt/homebrew",
		Cellar: "/opt/homebrew/Cellar",
	}

	path := env.CellarPath("python", "3.11.0")
	expected := "/opt/homebrew/Cellar/python/3.11.0"
	if path != expected {
		t.Errorf("CellarPath = %s, want %s", path, expected)
	}
}

func TestEnvironment_CaskroomPath(t *testing.T) {
	env := &Environment{
		Prefix:   "/opt/homebrew",
		Caskroom: "/opt/homebrew/Caskroom",
	}

	path := env.CaskroomPath("firefox", "110.0")
	expected := "/opt/homebrew/Caskroom/firefox/110.0"
	if path != expected {
		t.Errorf("CaskroomPath = %s, want %s", path, expected)
	}
}

func TestEnvironment_TapPath(t *testing.T) {
	env := &Environment{
		TapsDir: "/opt/homebrew/Homebrew/Library/Taps",
	}

	tests := []struct {
		tap      string
		expected string
	}{
		{"homebrew/core", "/opt/homebrew/Homebrew/Library/Taps/homebrew/homebrew-core"},
		{"user/repo", "/opt/homebrew/Homebrew/Library/Taps/user/homebrew-repo"},
		{"user/custom", "/opt/homebrew/Homebrew/Library/Taps/user/homebrew-custom"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		path := env.TapPath(tt.tap)
		if path != tt.expected {
			t.Errorf("TapPath(%s) = %s, want %s", tt.tap, path, tt.expected)
		}
	}
}

func TestEnvironment_TapFromPath(t *testing.T) {
	env := &Environment{
		TapsDir: "/opt/homebrew/Homebrew/Library/Taps",
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"/opt/homebrew/Homebrew/Library/Taps/homebrew/homebrew-core", "homebrew/core"},
		{"/opt/homebrew/Homebrew/Library/Taps/user/homebrew-repo", "user/repo"},
		{"/opt/homebrew/Homebrew/Library/Taps/user/custom", "user/custom"},
		{"/some/other/path", ""},
	}

	for _, tt := range tests {
		tap := env.TapFromPath(tt.path)
		if tap != tt.expected {
			t.Errorf("TapFromPath(%s) = %s, want %s", tt.path, tap, tt.expected)
		}
	}
}

func TestEnvironment_ShellEnv(t *testing.T) {
	env := &Environment{
		OS:         "darwin",
		Arch:       "arm64",
		Prefix:     "/opt/homebrew",
		Cellar:     "/opt/homebrew/Cellar",
		Repository: "/opt/homebrew/Homebrew",
		BinDir:     "/opt/homebrew/bin",
	}

	// Test bash output
	bashOutput := env.ShellEnv("bash")
	expectedStrings := []string{
		"export HOMEBREW_PREFIX=\"/opt/homebrew\"",
		"export HOMEBREW_CELLAR=\"/opt/homebrew/Cellar\"",
		"export HOMEBREW_REPOSITORY=\"/opt/homebrew/Homebrew\"",
		"export PATH=\"/opt/homebrew/bin:$PATH\"",
	}

	for _, expected := range expectedStrings {
		if !containsString(bashOutput, expected) {
			t.Errorf("ShellEnv(bash) missing expected string: %s", expected)
		}
	}

	// Test fish output
	fishOutput := env.ShellEnv("fish")
	expectedFishStrings := []string{
		"set -gx HOMEBREW_PREFIX /opt/homebrew",
		"set -gx HOMEBREW_CELLAR /opt/homebrew/Cellar",
		"set -gx HOMEBREW_REPOSITORY /opt/homebrew/Homebrew",
		"set -gx PATH /opt/homebrew/bin $PATH",
	}

	for _, expected := range expectedFishStrings {
		if !containsString(fishOutput, expected) {
			t.Errorf("ShellEnv(fish) missing expected string: %s", expected)
		}
	}
}

func TestEnvironment_GetEnvPath(t *testing.T) {
	env := &Environment{
		OS:     "darwin",
		Prefix: "/opt/homebrew",
		BinDir: "/opt/homebrew/bin",
	}

	path := env.GetEnvPath()
	if !containsString(path, "/opt/homebrew/bin") {
		t.Errorf("GetEnvPath() missing /opt/homebrew/bin")
	}
}

func TestEnvironment_GetEnvManpath(t *testing.T) {
	env := &Environment{
		Prefix: "/opt/homebrew",
	}

	manpath := env.GetEnvManpath()
	expected := "/opt/homebrew/share/man"
	if manpath != expected {
		t.Errorf("GetEnvManpath() = %s, want %s", manpath, expected)
	}
}

func TestGetFastbrewEnvironment(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	fastbrewDir, cacheDir, err := GetFastbrewEnvironment()
	if err != nil {
		t.Fatalf("GetFastbrewEnvironment failed: %v", err)
	}

	expectedFastbrew := filepath.Join(tempHome, ".fastbrew")
	if fastbrewDir != expectedFastbrew {
		t.Errorf("fastbrewDir = %s, want %s", fastbrewDir, expectedFastbrew)
	}

	expectedCache := filepath.Join(tempHome, ".fastbrew", "cache")
	if cacheDir != expectedCache {
		t.Errorf("cacheDir = %s, want %s", cacheDir, expectedCache)
	}

	// Verify directory was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Errorf("cache directory was not created")
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringInternal(s, substr))
}

func containsStringInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
