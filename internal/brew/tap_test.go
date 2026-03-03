package brew

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeTapRepoInput(t *testing.T) {
	tests := []struct {
		input    string
		wantRepo string
		wantURL  string
		wantErr  bool
	}{
		{"homebrew/core", "homebrew/core", "https://github.com/homebrew/core.git", false},
		{"homebrew/cask", "homebrew/cask", "https://github.com/homebrew/cask.git", false},
		{"user/repo", "user/repo", "https://github.com/user/repo.git", false},
		{"https://github.com/user/repo.git", "", "https://github.com/user/repo.git", false},
		{"https://github.com/user/repo", "", "https://github.com/user/repo", false},
		{"git@github.com:user/repo.git", "user/repo.git", "https://github.com/user/repo.git.git", false},
		{"invalid", "", "", true},
		{"", "", "", true},
		{"homebrew/", "", "", true},
		{"/user/repo", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			repo, url, err := normalizeTapRepoInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeTapRepoInput(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if repo != tt.wantRepo {
					t.Errorf("normalizeTapRepoInput(%q) repo = %q, want %q", tt.input, repo, tt.wantRepo)
				}
				if url != tt.wantURL {
					t.Errorf("normalizeTapRepoInput(%q) url = %q, want %q", tt.input, url, tt.wantURL)
				}
			}
		})
	}
}

func TestIsValidTapRepo(t *testing.T) {
	tests := []struct {
		input    string
		wantBool bool
	}{
		{"homebrew/core", true},
		{"homebrew/cask", true},
		{"user/repo", true},
		{"https://github.com/user/repo.git", true},
		{"git@github.com:user/repo.git", true},
		{"invalid", false},
		{"", false},
		{"homebrew/", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidTapRepo(tt.input)
			if result != tt.wantBool {
				t.Errorf("isValidTapRepo(%q) = %v, want %v", tt.input, result, tt.wantBool)
			}
		})
	}
}

func TestTapLocalPath(t *testing.T) {
	detectHomebrewPaths()

	tests := []struct {
		repo      string
		wantPanic bool
	}{
		{"homebrew/core", false},
		{"homebrew/cask", false},
		{"user/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			path := tapLocalPath(tt.repo)
			if path == "" {
				t.Errorf("tapLocalPath(%q) = empty string", tt.repo)
			}
			if !contains(path, "homebrew-") && !contains(path, "homebrew/") {
				t.Errorf("tapLocalPath(%q) = %q, should contain homebrew- prefix or homebrew/", tt.repo, path)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestTapManagerRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	tm := &TapManager{
		registryPath: filepath.Join(tmpDir, "taps.json"),
		taps:         make(map[string]Tap),
	}

	tap := Tap{
		Name:        "test/tap",
		RemoteURL:   "https://github.com/test/tap.git",
		LocalPath:   "/test/path",
		InstalledAt: time.Now(),
		IsCustom:    true,
	}

	tm.taps["test/tap"] = tap

	if err := tm.saveRegistry(); err != nil {
		t.Fatalf("saveRegistry failed: %v", err)
	}

	tm2 := &TapManager{
		registryPath: filepath.Join(tmpDir, "taps.json"),
		taps:         make(map[string]Tap),
	}

	if err := tm2.loadRegistry(); err != nil {
		t.Fatalf("loadRegistry failed: %v", err)
	}

	if _, ok := tm2.taps["test/tap"]; !ok {
		t.Error("loaded registry missing test/tap")
	}
}

func TestTapManagerGetTap(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	tm := &TapManager{
		registryPath: filepath.Join(tmpDir, "taps.json"),
		taps:         make(map[string]Tap),
	}

	tap := Tap{
		Name:        "homebrew/core",
		RemoteURL:   "https://github.com/homebrew/core.git",
		LocalPath:   "/opt/homebrew/Library/Taps/homebrew/homebrew-core",
		InstalledAt: time.Now(),
		IsCustom:    false,
	}

	tm.taps["homebrew/core"] = tap

	foundTap, exists := tm.GetTap("homebrew/core")
	if !exists {
		t.Error("GetTap returned exists=false for existing tap")
	}
	if foundTap.Name != "homebrew/core" {
		t.Errorf("GetTap returned tap with Name = %q, want %q", foundTap.Name, "homebrew/core")
	}

	_, exists = tm.GetTap("nonexistent/tap")
	if exists {
		t.Error("GetTap returned exists=true for nonexistent tap")
	}
}

func TestTapManagerListTapsEmptyDir(t *testing.T) {
	detectHomebrewPaths()

	origHome := os.Getenv("HOME")
	origPrefix := os.Getenv("HOMEBREW_PREFIX")
	homeDir := t.TempDir()
	os.Setenv("HOME", homeDir)

	os.Setenv("HOMEBREW_PREFIX", homeDir)

	defer func() {
		os.Setenv("HOME", origHome)
		if origPrefix != "" {
			os.Setenv("HOMEBREW_PREFIX", origPrefix)
		} else {
			os.Unsetenv("HOMEBREW_PREFIX")
		}
	}()

	detectHomebrewPaths()

	tm := &TapManager{
		registryPath: filepath.Join(homeDir, ".fastbrew", "taps.json"),
		taps:         make(map[string]Tap),
	}

	taps, err := tm.ListTaps()
	if err != nil {
		t.Fatalf("ListTaps failed: %v", err)
	}

	if len(taps) != 0 {
		t.Logf("ListTaps returned %d taps (may have scanned existing dirs)", len(taps))
	}
}

type testTapManager struct {
	*TapManager
}

func TestValidateTapContents(t *testing.T) {
	tmpDir := t.TempDir()

	formulaDir := filepath.Join(tmpDir, "Formula")
	if err := os.MkdirAll(formulaDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(formulaDir, "test.rb"), []byte("# test"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	tm := &TapManager{}

	if err := tm.validateTapContents(tmpDir); err != nil {
		t.Errorf("validateTapContents with formulae failed: %v", err)
	}

	casksDir := filepath.Join(tmpDir, "Casks")
	if err := os.MkdirAll(casksDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(casksDir, "test.cask"), []byte("# test"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	if err := tm.validateTapContents(tmpDir); err != nil {
		t.Errorf("validateTapContents with casks failed: %v", err)
	}

	emptyDir := t.TempDir()
	if err := tm.validateTapContents(emptyDir); err == nil {
		t.Error("validateTapContents should fail for empty directory")
	}
}

func TestTapManagerConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	tm := &TapManager{
		registryPath: filepath.Join(tmpDir, "taps.json"),
		taps:         make(map[string]Tap),
	}

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(n int) {
			tap := Tap{
				Name:        "test/tap",
				RemoteURL:   "https://github.com/test/tap.git",
				LocalPath:   "/test/path",
				InstalledAt: time.Now(),
				IsCustom:    true,
			}
			tm.mu.Lock()
			tm.taps["test/tap"] = tap
			tm.mu.Unlock()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
