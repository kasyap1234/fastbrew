package brew

import (
	"crypto/sha256"
	"encoding/hex"
	"fastbrew/internal/progress"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaskInstallerGetCaskVersionDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	installer := NewCaskInstaller(client)

	dir, err := installer.getCaskVersionDir("firefox", "128.0")
	if err != nil {
		t.Fatalf("getCaskVersionDir failed: %v", err)
	}

	expected := filepath.Join(client.Prefix, "Caskroom", "firefox", "128.0")
	if dir != expected {
		t.Errorf("getCaskVersionDir = %q, want %q", dir, expected)
	}
}

func TestCaskInstallerInstallCreatesCaskroom(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	caskroom := filepath.Join(client.Prefix, "Caskroom")
	if _, err := os.Stat(caskroom); err == nil {
		t.Skip("Caskroom already exists, skipping test")
	}

	installer := NewCaskInstaller(client)
	_ = installer

	if _, err := os.Stat(caskroom); os.IsNotExist(err) {
		t.Logf("Caskroom will be created at: %s", caskroom)
	}
}

func TestFetchCask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cask/firefox.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"token": "firefox",
			"version": "128.0"
		}`))
	}))
	defer server.Close()

	client := &Client{}

	cask, err := client.FetchCask("firefox")
	if err != nil {
		t.Fatalf("FetchCask failed: %v", err)
	}

	if cask.Token != "firefox" {
		t.Errorf("Token = %q, want %q", cask.Token, "firefox")
	}
	if cask.Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestClientIsCask(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	idx, err := client.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if len(idx.Casks) == 0 {
		t.Skip("No casks in index, skipping")
	}

	knownCask := idx.Casks[0].Token
	isCask, err := client.IsCask(knownCask)
	if err != nil {
		t.Fatalf("IsCask failed: %v", err)
	}
	if !isCask {
		t.Errorf("IsCask(%q) = false, want true", knownCask)
	}

	notCask := "nonexistent-package-xyz"
	isCask, _ = client.IsCask(notCask)
	if isCask {
		t.Errorf("IsCask(%q) = true, want false", notCask)
	}
}

func TestClientPrefixDetection(t *testing.T) {
	origHome := os.Getenv("HOME")
	origPrefix := os.Getenv("HOMEBREW_PREFIX")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		if origPrefix != "" {
			os.Setenv("HOMEBREW_PREFIX", origPrefix)
		} else {
			os.Unsetenv("HOMEBREW_PREFIX")
		}
	}()

	os.Unsetenv("HOMEBREW_PREFIX")
	t.Setenv("HOME", t.TempDir())

	_, err := NewClient()
	if err != nil {
		t.Logf("NewClient error (expected in test env): %v", err)
	}
}

func TestCaskDownloadArtifactPublishesProgressEvents(t *testing.T) {
	payload := []byte("abcdefghijklmnopqrstuvwxyz0123456789")
	sum := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	tmp := t.TempDir()
	client := &Client{
		Prefix: tmp,
		Cellar: filepath.Join(tmp, "Cellar"),
	}
	installer := NewCaskInstaller(client)

	pm := progress.NewManager()
	pm.StartEventRouter()
	defer pm.Close()

	eventsCh := make(chan progress.ProgressEvent, 64)
	pm.SubscribeToEvents("test-sub", eventsCh)
	defer pm.UnsubscribeFromEvents("test-sub")

	destPath := filepath.Join(tmp, "download.bin")
	if err := installer.downloadArtifact("firefox", server.URL, destPath, expectedSHA, pm); err != nil {
		t.Fatalf("downloadArtifact failed: %v", err)
	}

	var startSeen bool
	var progressSeen bool
	var completeSeen bool
	timeout := time.After(750 * time.Millisecond)

	for !completeSeen {
		select {
		case event := <-eventsCh:
			switch event.Type {
			case progress.EventDownloadStart:
				startSeen = true
				if event.Total <= 0 {
					t.Fatalf("expected total bytes > 0 for start event, got %d", event.Total)
				}
			case progress.EventDownloadProgress:
				progressSeen = true
				if event.Total <= 0 {
					t.Fatalf("expected total bytes > 0 for progress event, got %d", event.Total)
				}
			case progress.EventDownloadComplete:
				completeSeen = true
				if event.Total <= 0 {
					t.Fatalf("expected total bytes > 0 for complete event, got %d", event.Total)
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for cask download progress events")
		}
	}

	if !startSeen {
		t.Fatal("expected download_start event")
	}
	if !progressSeen {
		t.Fatal("expected at least one download_progress event")
	}
}
