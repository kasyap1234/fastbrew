package brew

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fastbrew/internal/httpclient"
	"fastbrew/internal/progress"
	"fastbrew/internal/resume"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// DownloadBottle downloads the bottle for a formula and returns the path to the cached tarball.
// It does not print any output.
func (c *Client) DownloadBottle(f *RemoteFormula) (string, error) {
	bottleURL, sha256Sum, err := f.GetBottleInfo()
	if err != nil {
		return "", err
	}

	cacheDir, _ := c.GetCacheDir()
	tarPath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.bottle", f.Name, f.Versions.Stable))

	var tracker progress.ProgressTracker
	if c.ProgressManager != nil {
		tracker = c.ProgressManager.Register(f.Name, bottleURL)
		defer c.ProgressManager.Unregister(f.Name)
	}

	if err := c.DownloadWithProgress(bottleURL, tarPath, sha256Sum, tracker); err != nil {
		return "", err
	}

	return tarPath, nil
}

// ExtractAndInstallBottle extracts a previously downloaded bottle tarball into the Cellar.
// It does not print any output.
func (c *Client) ExtractAndInstallBottle(f *RemoteFormula, tarPath string) error {
	cellarPath := filepath.Join(c.Prefix, "Cellar")

	tmpDir := filepath.Join(cellarPath, fmt.Sprintf(".fastbrew-tmp-%s-%d", f.Name, rand.IntN(1000000)))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := ExtractBottle(tarPath, tmpDir, c.Prefix); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	extractedPkgDir := filepath.Join(tmpDir, f.Name, f.Versions.Stable)
	if _, err := os.Stat(extractedPkgDir); err != nil {
		entries, _ := os.ReadDir(tmpDir)
		if len(entries) == 1 && entries[0].IsDir() {
			subEntries, _ := os.ReadDir(filepath.Join(tmpDir, entries[0].Name()))
			if len(subEntries) == 1 && subEntries[0].IsDir() {
				extractedPkgDir = filepath.Join(tmpDir, entries[0].Name(), subEntries[0].Name())
			}
		}
	}

	finalPkgDir := filepath.Join(cellarPath, f.Name)
	if err := os.MkdirAll(finalPkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create package dir: %w", err)
	}

	finalVersionDir := filepath.Join(finalPkgDir, f.Versions.Stable)
	os.RemoveAll(finalVersionDir)

	if err := os.Rename(extractedPkgDir, finalVersionDir); err != nil {
		return fmt.Errorf("failed to move extracted package into place: %w", err)
	}

	return nil
}

// InstallBottle downloads and extracts a bottle for the given formula (legacy wrapper).
func (c *Client) InstallBottle(f *RemoteFormula) error {
	tarPath, err := c.DownloadBottle(f)
	if err != nil {
		return err
	}
	return c.ExtractAndInstallBottle(f, tarPath)
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

	if resp.StatusCode == 206 {
		contentRange := resp.Header.Get("Content-Range")
		expectedPrefix := fmt.Sprintf("bytes %d-", startByte)
		if contentRange != "" && !strings.HasPrefix(contentRange, expectedPrefix) {
			out.Close()
			out, err = os.Create(dest)
			if err != nil {
				return err
			}
			defer out.Close()
			startByte = 0
		}
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
	authHeader = strings.TrimSpace(authHeader)
	if strings.HasPrefix(authHeader, "Bearer ") {
		authHeader = authHeader[7:]
	}

	params := make(map[string]string)
	for _, part := range strings.Split(authHeader, ",") {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		value := strings.Trim(strings.TrimSpace(part[idx+1:]), "\"")
		params[key] = value
	}

	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("could not find realm in Www-Authenticate")
	}

	service := params["service"]
	scope := params["scope"]

	tokenURL := fmt.Sprintf("%s?service=%s&scope=%s", realm,
		url.QueryEscape(service), url.QueryEscape(scope))
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

// ExtractBottle extracts a bottle archive (gzip or zstd compressed tar) to cellarDir.
// The tarball structure is `name/version/...`, extracted relative to cellarDir.
func ExtractBottle(tarPath, cellarDir, prefixDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	magic, err := br.Peek(4)
	if err != nil {
		return fmt.Errorf("failed to detect compression format: %w", err)
	}

	var decompReader io.Reader
	var decompCloser io.Closer

	if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gzr, err := gzip.NewReader(br)
		if err != nil {
			return err
		}
		decompReader = gzr
		decompCloser = gzr
	} else if len(magic) >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd {
		zr, err := zstd.NewReader(br)
		if err != nil {
			return err
		}
		decompReader = zr
		decompCloser = zr.IOReadCloser()
	} else {
		return fmt.Errorf("unsupported compression format (magic: %x)", magic)
	}
	defer decompCloser.Close()

	tr := tar.NewReader(decompReader)

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
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode)&0777)
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
			if !isSafeSymlink(cellarDir, prefixDir, target, linkTarget) {
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

func isSafeSymlink(cellarDir, prefixDir, target, linkname string) bool {
	var resolved string
	if filepath.IsAbs(linkname) {
		resolved = filepath.Clean(linkname)
	} else {
		resolved = filepath.Clean(filepath.Join(filepath.Dir(target), linkname))
	}
	cleanCellar := filepath.Clean(cellarDir) + string(os.PathSeparator)
	cleanPrefix := filepath.Clean(prefixDir) + string(os.PathSeparator)
	return strings.HasPrefix(resolved, cleanCellar) || strings.HasPrefix(resolved, cleanPrefix)
}
