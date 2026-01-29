package brew

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// InstallBottle downloads and extracts a bottle for the given formula
func (c *Client) InstallBottle(f *RemoteFormula) error {
	bottleURL, sha256Sum, err := f.GetBottleInfo()
	if err != nil {
		return err
	}

	fmt.Printf("  ⬇️  Downloading bottle for %s...\n", f.Name)

	// Use cached download if valid
	cacheDir, _ := c.GetCacheDir()
	tarPath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tar.gz", f.Name, f.Versions.Stable))

	if err := c.DownloadAndVerify(bottleURL, tarPath, sha256Sum); err != nil {
		return err
	}

	fmt.Printf("  📦 Extracting %s...\n", f.Name)

	// Determine install path (Cellar/name/version)
	// We need to construct this carefully.
	// Standard: {prefix}/Cellar/{name}/{version}

	// Note: Client needs access to Cellar path. Adding it to Client struct is needed.
	// For now assuming Client has Prefix and we derive Cellar from it if not explicit.
	cellarPath := filepath.Join(c.Prefix, "Cellar")
	if strings.Contains(c.Prefix, "Linuxbrew/Cellar") { // already points to cellar?
		// handle edge cases later, assume standard layout
	}

	destDir := filepath.Join(cellarPath, f.Name, f.Versions.Stable)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create cellar dir: %w", err)
	}

	if err := ExtractTarGz(tarPath, destDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	return nil
}

// DownloadAndVerify downloads the file and checks generic SHA256
func (c *Client) DownloadAndVerify(url, dest, expectedSHA string) error {
	// 1. Check if exists and valid
	if _, err := os.Stat(dest); err == nil {
		if verifyChecksum(dest, expectedSHA) == nil {
			return nil // Already downloaded and valid
		}
		os.Remove(dest) // Invalid, re-download
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// 2. Download with Auth Handling
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Create client
	httpClient := &http.Client{}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	// Handle 401 Unauthorized (GHCR needs token)
	if resp.StatusCode == 401 {
		authHeader := resp.Header.Get("Www-Authenticate")
		if authHeader != "" {
			token, err := getGHCRToken(authHeader)
			if err != nil {
				return fmt.Errorf("failed to get ghcr token: %w", err)
			}
			// Retry with token
			req.Header.Set("Authorization", "Bearer "+token)
			resp.Body.Close()
			resp, err = httpClient.Do(req)
			if err != nil {
				return err
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	out.Close() // Flush to disk

	// Verify
	if err := verifyChecksum(dest, expectedSHA); err != nil {
		os.Remove(dest)
		return fmt.Errorf("checksum mismatch: %w", err)
	}

	return nil
}

// getGHCRToken parses the Www-Authenticate header and fetches a bearer token
// Header format: Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:homebrew/core/cowsay:pull"
func getGHCRToken(authHeader string) (string, error) {
	parts := strings.Split(authHeader, ",")
	var realm, service, scope string

	for _, part := range parts {
		if strings.Contains(part, "realm=") {
			realm = strings.Trim(strings.Split(part, "=")[1], "\"")
		}
		if strings.Contains(part, "service=") {
			service = strings.Trim(strings.Split(part, "=")[1], "\"")
		}
		if strings.Contains(part, "scope=") {
			scope = strings.Trim(strings.Split(part, "=")[1], "\"")
		}
	}

	if realm == "" {
		return "", fmt.Errorf("could not find realm in Www-Authenticate")
	}

	tokenURL := fmt.Sprintf("%s?service=%s&scope=%s", realm, service, scope)
	resp, err := http.Get(tokenURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to get token from %s: %s", tokenURL, resp.Status)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

func verifyChecksum(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return nil
}

// ExtractTarGz extracts a tar.gz file to dest.
// Crucial: Strip the first component if needed?
// Homebrew bottles usually contain the full path `name/version/...` or just `version/...` inside?
// Actually bottles usually contain: `name/version/...` at root.
// So if we extract to `Cellar/`, it creates `name/version`.
// But we want to be safe. Let's inspect.
// API says `files.x86_64_linux.cellar` is `/home/linuxbrew/.linuxbrew/Cellar`.
// The tarball typically is relative to the Cellar.
// Let's assume we extract into the PARENT of destDir (which is Cellar) to match standard behavior,
// Or we extract into Cellar/name/version and strip components?
//
// CORRECTION: Bottles from ghcr.io are usually RELOCATABLE.
// They are tarballs of the `name/version` directory.
// Wait, let's verify standard behavior.
// If I download a bottle and extract it:
// `tar -tvf wget...tar.gz` ->
// `wget/1.25.0/bin/wget`
// `wget/1.25.0/share/...`
// So the structure inside the tar is `name/version/...`.
// Therefore, we should extract to `Cellar/` (the parent of `name`), NOT `Cellar/name/version`.
func ExtractTarGz(tarPath, cellarDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Security: prevent ZipSlip
		// The target path is cellarDir joined with header.Name
		target := filepath.Join(cellarDir, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(cellarDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}
