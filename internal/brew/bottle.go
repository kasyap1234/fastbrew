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

	// Bottles contain: name/version/... at root
	// So we extract directly to Cellar (NOT Cellar/name/version)
	cellarPath := filepath.Join(c.Prefix, "Cellar")

	// Remove all existing version directories for this package to prevent permission errors
	// Bottles may have revision suffixes like 20190702_1 that don't match API version
	pkgDir := filepath.Join(cellarPath, f.Name)
	if entries, err := os.ReadDir(pkgDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), f.Versions.Stable) {
				os.RemoveAll(filepath.Join(pkgDir, entry.Name()))
			}
		}
	}

	if err := ExtractTarGz(tarPath, cellarPath); err != nil {
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
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", target, err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close file %s: %w", target, err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create directory for symlink %s: %w", target, err)
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing %s: %w", target, err)
			}
			linkTarget := header.Linkname
			if !isSafeSymlink(cellarDir, target, linkTarget) {
				return fmt.Errorf("unsafe symlink target %q for %s", linkTarget, header.Name)
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", target, err)
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create directory for hard link %s: %w", target, err)
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing %s: %w", target, err)
			}
			linkTarget := filepath.Join(cellarDir, header.Linkname)
			if !strings.HasPrefix(linkTarget, filepath.Clean(cellarDir)+string(os.PathSeparator)) {
				return fmt.Errorf("illegal hard link target %q for %s", header.Linkname, header.Name)
			}
			if err := os.Link(linkTarget, target); err != nil {
				return fmt.Errorf("failed to create hard link %s: %w", target, err)
			}
		case tar.TypeChar, tar.TypeBlock:
			fmt.Printf("Warning: skipping device file %s\n", header.Name)
		default:
			if header.Typeflag != 0 {
				fmt.Printf("Warning: skipping unsupported file type %d for %s\n", header.Typeflag, header.Name)
			}
		}
	}
	return nil
}

func isSafeSymlink(cellarDir, target, linkname string) bool {
	if filepath.IsAbs(linkname) {
		resolved := filepath.Join(filepath.Dir(target), linkname)
		return strings.HasPrefix(resolved, filepath.Clean(cellarDir)+string(os.PathSeparator))
	}
	return true
}
