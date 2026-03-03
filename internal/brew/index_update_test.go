package brew

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndCompressSkipsUnchangedWithETag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client := &Client{}
	cacheDir, err := client.GetCacheDir()
	if err != nil {
		t.Fatalf("GetCacheDir failed: %v", err)
	}
	indexPath := filepath.Join(cacheDir, "formula.json.zst")

	payload := bytes.Repeat([]byte("fastbrew-index-data-"), 100)
	etag := `"v1"`

	requestCount := 0
	sawIfNoneMatch := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Header.Get("If-None-Match") == etag {
			sawIfNoneMatch = true
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", etag)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	changed, err := client.downloadAndCompress(server.URL, indexPath, "Formula")
	if err != nil {
		t.Fatalf("first downloadAndCompress failed: %v", err)
	}
	if !changed {
		t.Fatalf("first download should report changed=true")
	}

	firstStat, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("first stat failed: %v", err)
	}

	changed, err = client.downloadAndCompress(server.URL, indexPath, "Formula")
	if err != nil {
		t.Fatalf("second downloadAndCompress failed: %v", err)
	}
	if changed {
		t.Fatalf("second download should report changed=false")
	}
	if !sawIfNoneMatch {
		t.Fatalf("expected second request to send If-None-Match header")
	}
	if requestCount != 2 {
		t.Fatalf("expected exactly 2 requests, got %d", requestCount)
	}

	secondStat, err := os.Stat(indexPath)
	if err != nil {
		t.Fatalf("second stat failed: %v", err)
	}
	if !secondStat.ModTime().Equal(firstStat.ModTime()) {
		t.Fatalf("cache file was unexpectedly rewritten on unchanged response")
	}
}

func TestSplitIndexCacheLoadsMissingSideWhenPartialInMemoryState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	client := &Client{
		index: &Index{
			Formulae: []Formula{{Name: "existing-formula"}},
		},
	}

	cacheDir, err := client.GetCacheDir()
	if err != nil {
		t.Fatalf("GetCacheDir failed: %v", err)
	}

	formulaPath := filepath.Join(cacheDir, "formula.json.zst")
	caskPath := filepath.Join(cacheDir, "cask.json.zst")

	if err := os.WriteFile(formulaPath, []byte(`[{"name":"existing-formula","version":"1.0.0"}]`), 0644); err != nil {
		t.Fatalf("write formula index failed: %v", err)
	}
	if err := os.WriteFile(caskPath, []byte(`[{"token":"iterm2","version":"3.5.0"}]`), 0644); err != nil {
		t.Fatalf("write cask index failed: %v", err)
	}

	casks, err := client.LoadCaskIndex()
	if err != nil {
		t.Fatalf("LoadCaskIndex failed: %v", err)
	}
	if len(casks) != 1 || casks[0].Token != "iterm2" {
		t.Fatalf("expected cask cache to load from disk, got %+v", casks)
	}

	client2 := &Client{
		index: &Index{
			Casks: []Cask{{Token: "existing-cask"}},
		},
	}
	if err := os.WriteFile(formulaPath, []byte(`[{"name":"jq","version":"1.7.1"}]`), 0644); err != nil {
		t.Fatalf("rewrite formula index failed: %v", err)
	}
	if err := os.WriteFile(caskPath, []byte(`[{"token":"existing-cask","version":"1.0"}]`), 0644); err != nil {
		t.Fatalf("rewrite cask index failed: %v", err)
	}

	formulae, err := client2.LoadFormulaIndex()
	if err != nil {
		t.Fatalf("LoadFormulaIndex failed: %v", err)
	}
	if len(formulae) != 1 || formulae[0].Name != "jq" {
		t.Fatalf("expected formula cache to load from disk, got %+v", formulae)
	}
}
