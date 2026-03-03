package brew

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractTapArchive(tarPath, destPath, prefix string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(destPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create dir: %w", err)
			}
		case tar.TypeReg:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create parent dir: %w", err)
			}

			if !isPathSafe(target, destPath) {
				return fmt.Errorf("path traversal detected: %s", header.Name)
			}

			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()
		}
	}

	return nil
}

func isPathSafe(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

type TapDownloader struct {
	client *Client
}

func NewTapDownloader(c *Client) *TapDownloader {
	return &TapDownloader{client: c}
}

func (d *TapDownloader) Download(url, expectedSHA, name string) (string, error) {
	cacheDir, _ := d.client.GetCacheDir()
	tarPath := filepath.Join(cacheDir, fmt.Sprintf("%s-tap.bottle", name))

	if err := d.client.DownloadWithProgress(url, tarPath, expectedSHA, nil); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	if expectedSHA != "" {
		actualSHA, err := fileSHA256(tarPath)
		if err != nil {
			return "", fmt.Errorf("checksum verification failed: %w", err)
		}
		if actualSHA != expectedSHA {
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA, actualSHA)
		}
	}

	return tarPath, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
