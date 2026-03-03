package brew

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClient_ListInstalledNative(t *testing.T) {
	// Setup temporary brew prefix
	tempDir := filepath.Join(os.TempDir(), "fastbrew-test")
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	prefix := filepath.Join(tempDir, "usr-local")
	cellar := filepath.Join(prefix, "Cellar")
	caskroom := filepath.Join(prefix, "Caskroom")

	// Create dummy formulae
	formulae := []struct {
		name    string
		version string
	}{
		{"wget", "1.21.1"},
		{"curl", "7.79.1"},
	}

	for _, f := range formulae {
		versionDir := filepath.Join(cellar, f.name, f.version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create dummy casks
	casks := []struct {
		name    string
		version string
	}{
		{"iterm2", "3.4.15"},
	}

	for _, c := range casks {
		versionDir := filepath.Join(caskroom, c.name, c.version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	client := &Client{
		Prefix: prefix,
		Cellar: cellar,
	}

	pkgs, err := client.ListInstalledNative()
	if err != nil {
		t.Fatalf("ListInstalledNative failed: %v", err)
	}

	expectedCount := len(formulae) + len(casks)
	if len(pkgs) != expectedCount {
		t.Errorf("Expected %d packages, got %d", expectedCount, len(pkgs))
	}

	foundFormulae := 0
	foundCasks := 0
	for _, p := range pkgs {
		if p.IsCask {
			foundCasks++
		} else {
			foundFormulae++
		}
	}

	if foundFormulae != len(formulae) {
		t.Errorf("Expected %d formulae, got %d", len(formulae), foundFormulae)
	}
	if foundCasks != len(casks) {
		t.Errorf("Expected %d casks, got %d", len(casks), foundCasks)
	}
}

func TestClient_IsInstalled(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "fastbrew-test-isinstalled")
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	prefix := filepath.Join(tempDir, "usr-local")
	cellar := filepath.Join(prefix, "Cellar")

	client := &Client{
		Prefix: prefix,
		Cellar: cellar,
	}

	// Case 1: Not installed
	if client.isInstalled("wget") {
		t.Error("wget should not be installed")
	}

	// Case 2: Installed formula
	versionDir := filepath.Join(cellar, "wget", "1.21.1")
	os.MkdirAll(versionDir, 0755)

	if !client.isInstalled("wget") {
		t.Error("wget should be installed")
	}

	// Case 3: Installed cask
	caskroom := filepath.Join(prefix, "Caskroom")
	caskVersionDir := filepath.Join(caskroom, "iterm2", "3.4.15")
	os.MkdirAll(caskVersionDir, 0755)

	if !client.isInstalled("iterm2") {
		t.Error("iterm2 should be installed")
	}
}

func TestClient_ResolveDeps(t *testing.T) {
	client := &Client{
		index: &Index{
			Formulae: []Formula{
				{
					Name:         "a",
					Dependencies: []string{"b", "c"},
				},
				{
					Name:         "b",
					Dependencies: []string{"d"},
				},
				{
					Name: "c",
				},
				{
					Name: "d",
				},
			},
		},
	}
	// Manually set indexOnce to loaded state if needed, but here we just set c.index directly
	client.indexOnce.Do(func() {}) // Mark as done

	deps, err := client.ResolveDeps([]string{"a"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	expectedDeps := []string{"d", "b", "c"}
	if len(deps) != len(expectedDeps) {
		t.Errorf("Expected %d dependencies, got %d: %v", len(expectedDeps), len(deps), deps)
	}

	// Order might matter or not depending on implementation, ResolveDeps seems to do post-order-ish
	// a -> [b, c], b -> [d]. Expected: d, b, c
	for i, expected := range expectedDeps {
		if deps[i] != expected {
			t.Errorf("At index %d: expected %s, got %s", i, expected, deps[i])
		}
	}
}

func TestUnique(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	expected := []string{"a", "b", "c"}
	output := unique(input)

	if len(output) != len(expected) {
		t.Fatalf("Expected len %d, got %d", len(expected), len(output))
	}

	for i, v := range expected {
		if output[i] != v {
			t.Errorf("At index %d: expected %s, got %s", i, v, output[i])
		}
	}
}
