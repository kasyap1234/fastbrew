package brew

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fastbrew/internal/httpclient"
	"fastbrew/internal/progress"
	"fastbrew/internal/resume"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallBottle downloads and extracts a bottle for the given formula
func (c *Client) InstallBottle(f *RemoteFormula) error {
	bottleURL, sha256Sum, err := f.GetBottleInfo()
	if err != nil {
		return err
	}

	fmt.Printf("  ⬇️  Downloading bottle for %s...\n", f.Name)

	cacheDir, _ := c.GetCacheDir()
	tarPath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tar.gz", f.Name, f.Versions.Stable))

	var tracker progress.ProgressTracker
	if c.ProgressManager != nil {
		tracker = c.ProgressManager.Register(f.Name, bottleURL)
		defer c.ProgressManager.Unregister(f.Name)
	}

	if err := c.DownloadWithProgress(bottleURL, tarPath, sha256Sum, tracker); err != nil {
		return err
	}

	fmt.Printf("  📦 Extracting %s...\n", f.Name)

	cellarPath := filepath.Join(c.Prefix, "Cellar")

	pkgDir := filepath.Join(cellarPath, f.Name)
	if entries, err := os.ReadDir(pkgDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), f.Versions.Stable) {
				os.RemoveAll(filepath.Join(pkgDir, entry.Name()))
			}
		}
	}

	start := time.Now()
	if err := ExtractTarGz(tarPath, cellarPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}
	if c.Verbose {
		fmt.Printf("  ⏱️  Extracted %s in %s\n", f.Name, time.Since(start).Round(time.Millisecond))
	}

	return nil
}

// DownloadAndVerify downloads the file and checks generic SHA256
func (c *Client) DownloadAndVerify(url, dest, expectedSHA string) error {
	return c.DownloadWithProgress(url, dest, expectedSHA, nil)
}

// DownloadWithProgress downloads a file with optional progress tracking and resume support
func (c *Client) DownloadWithProgress(url, dest, expectedSHA string, tracker progress.ProgressTracker) error {
	if _, err := os.Stat(dest); err == nil {
		if verifyChecksum(dest, expectedSHA) == nil {
			return nil
		}
		os.Remove(dest)
	}

	cacheDir, _ := c.GetCacheDir()
	rm := resume.NewResumeManager(cacheDir)

	var pd *resume.PartialDownload
	var startByte int64

	if rm.Exists(dest) {
		var err error
		pd, err = rm.Load(dest)
		if err == nil && pd.URL == url && resume.CanResume(pd.State) {
			if info, statErr := os.Stat(dest); statErr == nil {
				startByte = info.Size()
			}
		} else {
			rm.Delete(dest)
			os.Remove(dest)
			pd = nil
		}
	}

	var out *os.File
	var err error
	if startByte > 0 {
		out, err = os.OpenFile(dest, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		out, err = os.Create(dest)
	}
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == 401 {
		authHeader := resp.Header.Get("Www-Authenticate")
		if authHeader != "" {
			token, tokenErr := getGHCRToken(authHeader)
			if tokenErr != nil {
				resp.Body.Close()
				return fmt.Errorf("failed to get ghcr token: %w", tokenErr)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp.Body.Close()
			resp, err = httpClient.Do(req)
			if err != nil {
				return err
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 && startByte > 0 {
		out.Close()
		out, err = os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		startByte = 0
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	totalSize := resp.ContentLength + startByte
	if pd == nil {
		pd, _ = rm.Create(url, dest)
	}
	if pd != nil {
		pd.TotalSize = totalSize
		pd.ETag = resp.Header.Get("ETag")
		pd.LastModified = resp.Header.Get("Last-Modified")
		pd.UpdateState(resume.StateInProgress)
		rm.Save(pd)
	}

	if tracker != nil {
		tracker.Start(totalSize)
	}

	buf := make([]byte, 32*1024)
	downloaded := startByte

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				if pd != nil {
					pd.DownloadedBytes = downloaded
					pd.UpdateState(resume.StateFailed)
					rm.Save(pd)
				}
				return writeErr
			}
			downloaded += int64(n)

			if tracker != nil {
				tracker.Update(downloaded)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if pd != nil {
				pd.DownloadedBytes = downloaded
				pd.UpdateState(resume.StateFailed)
				rm.Save(pd)
			}
			return readErr
		}
	}

	out.Close()

	if err := verifyChecksum(dest, expectedSHA); err != nil {
		if pd != nil {
			pd.UpdateState(resume.StateFailed)
			rm.Save(pd)
		}
		os.Remove(dest)
		return fmt.Errorf("checksum mismatch: %w", err)
	}

	if pd != nil {
		pd.DownloadedBytes = downloaded
		pd.UpdateState(resume.StateComplete)
		rm.Delete(dest)
	}

	if tracker != nil {
		tracker.Complete()
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
	resp, err := httpclient.Get().Get(tokenURL)
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
