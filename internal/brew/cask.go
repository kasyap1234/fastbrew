package brew

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fastbrew/internal/httpclient"
	"fastbrew/internal/progress"
)

type CaskArtifact struct {
	App     []interface{} `json:"app,omitempty"`
	Binary  []interface{} `json:"binary,omitempty"`
	Pkg     []interface{} `json:"pkg,omitempty"`
	MLAMD64 []interface{} `json:"mlamd64,omitempty"`
	Suite   []interface{} `json:"suite,omitempty"`
	Zip     []interface{} `json:"zip,omitempty"`
	Dmg     []interface{} `json:"dmg,omitempty"`
	Ruby    []interface{} `json:"ruby,omitempty"`
	Script  []interface{} `json:"script,omitempty"`
}

type CaskDependsOn struct {
	MacOS interface{} `json:"macos,omitempty"`
}

type CaskUninstall struct {
	Quit        string      `json:"quit,omitempty"`
	LaunchAgent string      `json:"launch_agent,omitempty"`
	LaunchJob   string      `json:"launch_job,omitempty"`
	LoginItem   string      `json:"login_item,omitempty"`
	Kext        string      `json:"kext,omitempty"`
	Directory   string      `json:"directory,omitempty"`
	File        interface{} `json:"file,omitempty"`
	RawKernel   string      `json:"raw_kernel,omitempty"`
	Script      interface{} `json:"script,omitempty"`
	Keystones   interface{} `json:"keystones,omitempty"`
	Plist       interface{} `json:"plist,omitempty"`
}

type CaskMetadata struct {
	Token              string         `json:"token"`
	FullToken          string         `json:"full_token"`
	OldTokens          []string       `json:"old_tokens"`
	Tap                string         `json:"tap"`
	Name               []string       `json:"name"`
	Desc               string         `json:"desc"`
	Homepage           string         `json:"homepage"`
	URL                string         `json:"url"`
	Version            string         `json:"version"`
	SHA256             string         `json:"sha256"`
	Artifacts          []CaskArtifact `json:"artifacts"`
	DependsOn          CaskDependsOn  `json:"depends_on"`
	ConflictsWith      interface{}    `json:"conflicts_with"`
	AutoUpdates        bool           `json:"auto_updates"`
	Deprecated         bool           `json:"deprecated"`
	Disabled           bool           `json:"disabled"`
	BundleShortVersion string         `json:"bundle_short_version,omitempty"`
	BundleVersion      string         `json:"bundle_version,omitempty"`
}

type CaskInstaller struct {
	client    *Client
	metadata  *CaskMetadata
	caskDir   string
	operation string
}

type InstallReceipt struct {
	Token          string    `json:"token"`
	Version        string    `json:"version"`
	InstalledFiles []string  `json:"installed_files"`
	InstallMethod  string    `json:"install_method"`
	SourceArtifact string    `json:"source_artifact"`
	UninstallHints []string  `json:"uninstall_hints"`
	PkgReceiptIDs  []string  `json:"pkg_receipt_ids,omitempty"`
	InstalledAt    time.Time `json:"installed_at"`
}

func NewCaskInstaller(client *Client) *CaskInstaller {
	return &CaskInstaller{
		client:    client,
		operation: MutationOperationInstall,
	}
}

func (ci *CaskInstaller) SetOperation(operation string) {
	if strings.TrimSpace(operation) == "" {
		ci.operation = MutationOperationInstall
		return
	}
	ci.operation = operation
}

func (ci *CaskInstaller) currentOperation() string {
	if strings.TrimSpace(ci.operation) == "" {
		return MutationOperationInstall
	}
	return ci.operation
}

func (c *Client) FetchCaskMetadata(name string) (*CaskMetadata, error) {
	url := fmt.Sprintf("%s/%s.json", CaskAPIURL, name)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for cask %s: %w", name, err)
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cask %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("cask %s not found on API", name)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d for cask %s", resp.StatusCode, name)
	}

	var metadata CaskMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to parse cask JSON for %s: %w", name, err)
	}

	return &metadata, nil
}

func (ci *CaskInstaller) getCaskDir() (string, error) {
	if ci.caskDir != "" {
		return ci.caskDir, nil
	}
	dir := filepath.Join(ci.client.Prefix, "Caskroom")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cask directory: %w", err)
	}
	ci.caskDir = dir
	return dir, nil
}

func (ci *CaskInstaller) getCaskVersionDir(name, version string) (string, error) {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(caskDir, name, version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cask version directory: %w", err)
	}
	return dir, nil
}

func (ci *CaskInstaller) downloadArtifact(packageName, url, destPath, expectedSHA256 string, p *progress.Manager) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpClient := httpclient.Get()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()

	var tracker progress.ProgressTracker
	if p != nil {
		tracker = p.Register(packageName, url)
		defer p.Unregister(packageName)
		tracker.Start(resp.ContentLength)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	buf := make([]byte, 1024*1024)
	var downloaded int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
				os.Remove(tmpPath)
				if tracker != nil {
					tracker.Error(writeErr)
				}
				return fmt.Errorf("failed to write downloaded file: %w", writeErr)
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
			os.Remove(tmpPath)
			if tracker != nil {
				tracker.Error(readErr)
			}
			return fmt.Errorf("failed to write downloaded file: %w", readErr)
		}
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && actualSHA256 != expectedSHA256 {
		os.Remove(tmpPath)
		err := fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSHA256, actualSHA256)
		if tracker != nil {
			tracker.Error(err)
		}
		return err
	}

	if err := f.Close(); err != nil {
		if tracker != nil {
			tracker.Error(err)
		}
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		if tracker != nil {
			tracker.Error(err)
		}
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	if tracker != nil {
		tracker.Complete()
	}

	return nil
}

func (ci *CaskInstaller) Install(name string, p *progress.Manager) error {
	operation := ci.currentOperation()
	ci.client.emitMutation(operation, name, MutationPhaseMetadata, MutationStatusRunning, "fetching cask metadata", 0, 0, "")
	metadata, err := ci.client.FetchCaskMetadata(name)
	if err != nil {
		ci.client.emitMutation(operation, name, MutationPhaseMetadata, MutationStatusFailed, err.Error(), 0, 0, "")
		return fmt.Errorf("failed to fetch cask metadata: %w", err)
	}
	ci.client.emitMutation(operation, name, MutationPhaseMetadata, MutationStatusSucceeded, "metadata ready", 0, 0, "")
	ci.metadata = metadata

	if metadata.Deprecated {
		fmt.Printf("⚠️  Cask %s is deprecated\n", name)
	}
	if metadata.Disabled {
		ci.client.emitMutation(operation, name, MutationPhaseInstall, MutationStatusFailed, "cask disabled", 0, 0, "")
		return fmt.Errorf("cask %s is disabled", name)
	}

	versionDir, err := ci.getCaskVersionDir(name, metadata.Version)
	if err != nil {
		return err
	}

	artifactPath := filepath.Join(versionDir, filepath.Base(metadata.URL))
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		fmt.Printf("📥 Downloading %s %s...\n", name, metadata.Version)
		ci.client.emitMutation(operation, name, MutationPhaseDownload, MutationStatusRunning, "downloading artifact", 0, 0, "bytes")
		if err := ci.downloadArtifact(name, metadata.URL, artifactPath, metadata.SHA256, p); err != nil {
			ci.client.emitMutation(operation, name, MutationPhaseDownload, MutationStatusFailed, err.Error(), 0, 0, "bytes")
			return fmt.Errorf("failed to download artifact: %w", err)
		}
		ci.client.emitMutation(operation, name, MutationPhaseDownload, MutationStatusSucceeded, "download complete", 0, 0, "bytes")
	} else {
		fmt.Printf("📦 Using cached artifact for %s %s\n", name, metadata.Version)
		ci.client.emitMutation(operation, name, MutationPhaseDownload, MutationStatusSkipped, "using cached artifact", 0, 0, "bytes")
	}

	ci.client.emitMutation(operation, name, MutationPhaseInstall, MutationStatusRunning, "installing cask artifacts", 0, 0, "")
	if err := ci.installArtifacts(artifactPath, metadata.Artifacts, versionDir); err != nil {
		ci.client.emitMutation(operation, name, MutationPhaseInstall, MutationStatusFailed, err.Error(), 0, 0, "")
		return fmt.Errorf("failed to install artifacts: %w", err)
	}

	if err := ci.writeReceipt(name, metadata); err != nil {
		ci.client.emitMutation(operation, name, MutationPhaseInstall, MutationStatusFailed, err.Error(), 0, 0, "")
		return fmt.Errorf("failed to write receipt: %w", err)
	}
	ci.client.emitMutation(operation, name, MutationPhaseComplete, MutationStatusSucceeded, "cask install complete", 0, 0, "")

	fmt.Printf("✅ %s %s installed successfully!\n", name, metadata.Version)
	ci.client.notifyInvalidation(EventInstalledChanged)
	ci.client.notifyInvalidation(EventServiceChanged)
	return nil
}

func (ci *CaskInstaller) installArtifacts(artifactPath string, artifacts []CaskArtifact, versionDir string) error {
	for _, artifact := range artifacts {
		if len(artifact.App) > 0 {
			if err := ci.installApp(artifactPath, artifact.App); err != nil {
				return fmt.Errorf("failed to install app: %w", err)
			}
		}
		if len(artifact.Pkg) > 0 {
			if err := ci.installPkg(artifactPath, artifact.Pkg); err != nil {
				return fmt.Errorf("failed to install pkg: %w", err)
			}
		}
		if len(artifact.Binary) > 0 {
			if err := ci.installBinary(artifactPath, artifact.Binary); err != nil {
				return fmt.Errorf("failed to install binary: %w", err)
			}
		}
		if len(artifact.Dmg) > 0 {
			if err := ci.installDmg(artifactPath, artifact.Dmg); err != nil {
				return fmt.Errorf("failed to install dmg: %w", err)
			}
		}
		if len(artifact.Zip) > 0 {
			if err := ci.installZip(artifactPath, artifact.Zip); err != nil {
				return fmt.Errorf("failed to install zip: %w", err)
			}
		}
	}
	return nil
}

func detectCaskArtifactExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".dmg" || ext == ".zip" || ext == ".pkg" {
		return ext
	}

	f, err := os.Open(path)
	if err != nil {
		return ext
	}
	defer f.Close()

	header := make([]byte, 512)
	n, _ := f.Read(header)
	if n > 4 && string(header[:4]) == "PK\x03\x04" {
		return ".zip"
	}

	// Simple check for DMG magic bytes or just rely on content type
	contentType := http.DetectContentType(header[:n])
	if strings.Contains(contentType, "application/zip") {
		return ".zip"
	}
	if strings.Contains(contentType, "application/x-mach-binary") || strings.Contains(contentType, "application/octet-stream") {
		// DMG files are often octet-stream, but we can try hdiutil later if needed.
		// For now, if it's not a zip and has no extension, fallback to .dmg as a guess or return empty.
	}

	return ext
}

func (ci *CaskInstaller) installApp(artifactPath string, apps []interface{}) error {
	ext := detectCaskArtifactExtension(artifactPath)
	switch ext {
	case ".dmg":
		return ci.mountAndInstallApp(artifactPath, apps)
	case ".zip":
		return ci.extractAndInstallApp(artifactPath, apps)
	default:
		// Fallback to trying dmg if we really don't know, since many casks are DMGs without extensions
		if ext == "" {
			fmt.Printf("⚠️ Unknown artifact format, trying to mount as DMG: %s\n", artifactPath)
			return ci.mountAndInstallApp(artifactPath, apps)
		}
		return fmt.Errorf("unsupported app artifact format: %s", ext)
	}
}

func (ci *CaskInstaller) mountAndInstallApp(dmgPath string, apps []interface{}) error {
	fmt.Printf("🔧 Mounting DMG: %s\n", dmgPath)

	mountPoint, err := ci.mountDmg(dmgPath)
	if err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}
	defer func() {
		if err := ci.detachDmg(mountPoint); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to detach DMG: %v\n", err)
		}
	}()

	installedFiles, err := ci.installFromMountedVolume(mountPoint, apps)
	if err != nil {
		return fmt.Errorf("failed to install from mounted volume: %w", err)
	}

	return ci.writeEnhancedReceipt(installedFiles, "dmg", dmgPath)
}

func (ci *CaskInstaller) mountDmg(dmgPath string) (string, error) {
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-nobrowse", "-readonly", "-plist")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("hdiutil attach failed: %w", err)
	}

	mountPoint, err := findMountPointInPlist(output)
	if err != nil {
		return "", err
	}

	return mountPoint, nil
}

func findMountPointInPlist(data []byte) (string, error) {
	content := string(data)
	lines := strings.Split(content, "\n")
	var inEntities bool
	var foundMountPointKey bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "system-entities") {
			inEntities = true
			continue
		}
		if !inEntities {
			continue
		}
		if strings.HasPrefix(trimmed, "</array>") {
			break
		}

		// In plist format, <key> and <string> are on separate lines:
		//   <key>mount-point</key>
		//   <string>/Volumes/Something</string>
		if foundMountPointKey {
			start := strings.Index(trimmed, "<string>")
			if start != -1 {
				start += 8
				end := strings.Index(trimmed[start:], "</string>")
				if end != -1 {
					return trimmed[start : start+end], nil
				}
			}
			foundMountPointKey = false
		}

		if strings.Contains(trimmed, "<key>mount-point</key>") {
			// Check if the value is on the same line (unlikely but defensive)
			start := strings.Index(trimmed, "<string>")
			if start != -1 {
				start += 8
				end := strings.Index(trimmed[start:], "</string>")
				if end != -1 {
					return trimmed[start : start+end], nil
				}
			}
			// Value is on the next line
			foundMountPointKey = true
		}
	}

	return "", fmt.Errorf("mount point not found in DMG plist")
}

func (ci *CaskInstaller) detachDmg(mountPoint string) error {
	cmd := exec.Command("hdiutil", "detach", mountPoint)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hdiutil detach failed: %w", err)
	}
	return nil
}

func (ci *CaskInstaller) installFromMountedVolume(mountPoint string, apps []interface{}) ([]string, error) {
	var installedFiles []string

	for _, app := range apps {
		appName, ok := app.(string)
		if !ok {
			continue
		}

		srcPath := filepath.Join(mountPoint, appName)
		if _, err := os.Stat(srcPath); err != nil {
			continue
		}

		targetPath := filepath.Join("/Applications", appName)
		files, err := ci.copyAppBundle(srcPath, targetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to copy %s: %w", appName, err)
		}
		installedFiles = append(installedFiles, files...)
	}

	return installedFiles, nil
}

func (ci *CaskInstaller) copyAppBundle(srcPath, targetPath string) ([]string, error) {
	var files []string

	tmpPath := targetPath + ".tmp"
	if err := os.RemoveAll(tmpPath); err != nil {
		return nil, err
	}

	if err := copyDirRecursive(srcPath, tmpPath, &files); err != nil {
		os.RemoveAll(tmpPath)
		return nil, err
	}

	if _, err := os.Stat(targetPath); err == nil {
		if err := os.RemoveAll(targetPath); err != nil {
			os.RemoveAll(tmpPath)
			return nil, err
		}
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return nil, err
	}

	return files, nil
}

func copyDirRecursive(src, dst string, files *[]string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath, files); err != nil {
				return err
			}
		} else {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return err
			}
			defer srcFile.Close()

			dstFile, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			defer dstFile.Close()

			if _, err := io.Copy(dstFile, srcFile); err != nil {
				return err
			}

			if err := dstFile.Chmod(info.Mode()); err != nil {
				return err
			}

			*files = append(*files, dstPath)
		}
	}

	return nil
}

func (ci *CaskInstaller) extractAndInstallApp(zipPath string, apps []interface{}) error {
	fmt.Printf("🔧 Extracting and installing app from ZIP: %s\n", zipPath)

	tmpDir, err := os.MkdirTemp("", "fastbrew-cask-zip-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := ci.extractZip(zipPath, tmpDir); err != nil {
		return fmt.Errorf("failed to extract zip: %w", err)
	}

	installedFiles, err := ci.installFromExtractedDir(tmpDir, apps)
	if err != nil {
		return fmt.Errorf("failed to install from extracted directory: %w", err)
	}

	return ci.writeEnhancedReceipt(installedFiles, "zip", zipPath)
}

func (ci *CaskInstaller) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		inFile, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		if _, err := io.Copy(outFile, inFile); err != nil {
			inFile.Close()
			outFile.Close()
			return err
		}

		inFile.Close()
		outFile.Close()
	}

	return nil
}

func (ci *CaskInstaller) installFromExtractedDir(tmpDir string, apps []interface{}) ([]string, error) {
	var installedFiles []string

	for _, app := range apps {
		appName, ok := app.(string)
		if !ok {
			continue
		}

		srcPath := filepath.Join(tmpDir, appName)
		if _, err := os.Stat(srcPath); err != nil {
			continue
		}

		targetPath := filepath.Join("/Applications", appName)
		files, err := ci.copyAppBundle(srcPath, targetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to copy %s: %w", appName, err)
		}
		installedFiles = append(installedFiles, files...)
	}

	return installedFiles, nil
}

func (ci *CaskInstaller) installPkg(artifactPath string, pkgs []interface{}) error {
	fmt.Printf("🔧 Installing PKG: %s\n", artifactPath)

	var installedFiles []string
	var pkgIDs []string

	for _, pkg := range pkgs {
		pkgName, ok := pkg.(string)
		if !ok {
			continue
		}

		pkgPath := artifactPath
		if !strings.HasSuffix(pkgPath, ".pkg") && !strings.HasPrefix(pkgPath, "/") {
			pkgPath = filepath.Join(filepath.Dir(artifactPath), pkgName)
		}

		if _, err := os.Stat(pkgPath); err != nil {
			continue
		}

		files, pkgID, err := ci.installSinglePkg(pkgPath)
		if err != nil {
			return fmt.Errorf("failed to install pkg %s: %w", pkgName, err)
		}

		installedFiles = append(installedFiles, files...)
		pkgIDs = append(pkgIDs, pkgID)
	}

	return ci.writeEnhancedReceiptWithPkgIDs(installedFiles, "pkg", artifactPath, pkgIDs)
}

func (ci *CaskInstaller) installSinglePkg(pkgPath string) ([]string, string, error) {
	cmd := exec.Command("installer", "-pkg", pkgPath, "-target", "/", "-plist")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "needs to be run as root") || strings.Contains(stderr, "No such file or directory") {
				return nil, "", fmt.Errorf("installer requires administrator privileges (run with sudo): %w", err)
			}
			return nil, "", fmt.Errorf("installer failed: %w", err)
		}
		return nil, "", fmt.Errorf("installer failed: %w", err)
	}

	var files []string
	var pkgID string

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "file-path") {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				path := strings.TrimSpace(parts[1])
				files = append(files, path)
			}
		}
		if strings.Contains(line, "package-id") {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				pkgID = strings.TrimSpace(parts[1])
			}
		}
	}

	if pkgID == "" {
		pkgID = filepath.Base(pkgPath)
	}

	return files, pkgID, nil
}

func (ci *CaskInstaller) installBinary(artifactPath string, binaries []interface{}) error {
	fmt.Printf("🔧 Installing binary: %s\n", artifactPath)

	var installedFiles []string

	for _, binary := range binaries {
		binaryName, ok := binary.(string)
		if !ok {
			continue
		}

		binaryPath := artifactPath
		if !strings.HasSuffix(binaryPath, binaryName) {
			binaryPath = filepath.Join(filepath.Dir(artifactPath), binaryName)
		}

		if _, err := os.Stat(binaryPath); err != nil {
			continue
		}

		targetPath := filepath.Join("/usr/local/bin", binaryName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create bin directory: %w", err)
		}

		if err := copyFile(binaryPath, targetPath); err != nil {
			return fmt.Errorf("failed to copy binary: %w", err)
		}

		if err := os.Chmod(targetPath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}

		installedFiles = append(installedFiles, targetPath)
	}

	return ci.writeEnhancedReceipt(installedFiles, "binary", artifactPath)
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (ci *CaskInstaller) installDmg(artifactPath string, dmgs []interface{}) error {
	return ci.installApp(artifactPath, dmgs)
}

func (ci *CaskInstaller) installZip(artifactPath string, zips []interface{}) error {
	return ci.installApp(artifactPath, zips)
}

func (ci *CaskInstaller) writeEnhancedReceipt(installedFiles []string, method, artifact string) error {
	receipt := InstallReceipt{
		Token:          ci.metadata.Token,
		Version:        ci.metadata.Version,
		InstalledFiles: installedFiles,
		InstallMethod:  method,
		SourceArtifact: artifact,
		InstalledAt:    time.Now(),
	}

	return ci.saveEnhancedReceipt(receipt)
}

func (ci *CaskInstaller) writeEnhancedReceiptWithPkgIDs(installedFiles []string, method, artifact string, pkgIDs []string) error {
	receipt := InstallReceipt{
		Token:          ci.metadata.Token,
		Version:        ci.metadata.Version,
		InstalledFiles: installedFiles,
		InstallMethod:  method,
		SourceArtifact: artifact,
		PkgReceiptIDs:  pkgIDs,
		InstalledAt:    time.Now(),
	}

	return ci.saveEnhancedReceipt(receipt)
}

func (ci *CaskInstaller) saveEnhancedReceipt(receipt InstallReceipt) error {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return err
	}

	receiptPath := filepath.Join(caskDir, ci.metadata.Token, ".receipt.json")
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal receipt: %w", err)
	}

	return os.WriteFile(receiptPath, data, 0644)
}

func (ci *CaskInstaller) writeReceipt(name string, metadata *CaskMetadata) error {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return err
	}

	receiptPath := filepath.Join(caskDir, name, ".receipt.json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal receipt: %w", err)
	}

	return os.WriteFile(receiptPath, data, 0644)
}

func (ci *CaskInstaller) Uninstall(name string) error {
	operation := ci.currentOperation()
	ci.client.emitMutation(operation, name, MutationPhaseUninstall, MutationStatusRunning, "uninstalling cask", 0, 0, "")

	receipt, err := ci.loadEnhancedReceipt(name)
	if err != nil {
		if legacyErr := ci.legacyUninstall(name); legacyErr != nil {
			ci.client.emitMutation(operation, name, MutationPhaseUninstall, MutationStatusFailed, legacyErr.Error(), 0, 0, "")
			return legacyErr
		}
		ci.client.emitMutation(operation, name, MutationPhaseComplete, MutationStatusSucceeded, "cask uninstall complete", 0, 0, "")
		return nil
	}

	if receipt.PkgReceiptIDs != nil && len(receipt.PkgReceiptIDs) > 0 {
		for _, pkgID := range receipt.PkgReceiptIDs {
			cmd := exec.Command("pkgutil", "--forget", pkgID)
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to forget pkg %s: %v\n", pkgID, err)
			}
		}
	}

	for _, file := range receipt.InstalledFiles {
		if err := os.RemoveAll(file); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", file, err)
		}
	}

	caskDir, err := ci.getCaskDir()
	if err != nil {
		ci.client.emitMutation(operation, name, MutationPhaseUninstall, MutationStatusFailed, err.Error(), 0, 0, "")
		return err
	}

	versionDirs, err := filepath.Glob(filepath.Join(caskDir, name, "*"))
	if err != nil {
		ci.client.emitMutation(operation, name, MutationPhaseUninstall, MutationStatusFailed, err.Error(), 0, 0, "")
		return fmt.Errorf("failed to list cask versions: %w", err)
	}

	for _, dir := range versionDirs {
		if err := os.RemoveAll(dir); err != nil {
			ci.client.emitMutation(operation, name, MutationPhaseUninstall, MutationStatusFailed, err.Error(), 0, 0, "")
			return fmt.Errorf("failed to remove %s: %w", dir, err)
		}
	}

	ci.client.emitMutation(operation, name, MutationPhaseComplete, MutationStatusSucceeded, "cask uninstall complete", 0, 0, "")
	fmt.Printf("✅ %s uninstalled successfully!\n", name)
	ci.client.notifyInvalidation(EventInstalledChanged)
	ci.client.notifyInvalidation(EventServiceChanged)
	return nil
}

func (ci *CaskInstaller) loadEnhancedReceipt(name string) (*InstallReceipt, error) {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return nil, err
	}

	receiptPath := filepath.Join(caskDir, name, ".receipt.json")
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		return nil, err
	}

	var receipt InstallReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}

	if receipt.Token == "" {
		return nil, fmt.Errorf("not an enhanced receipt")
	}

	return &receipt, nil
}

func (ci *CaskInstaller) legacyUninstall(name string) error {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return err
	}

	versionDirs, err := filepath.Glob(filepath.Join(caskDir, name, "*"))
	if err != nil {
		return fmt.Errorf("failed to list cask versions: %w", err)
	}

	for _, dir := range versionDirs {
		if filepath.Base(dir) == ".receipt.json" {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove %s: %w", dir, err)
		}
	}

	fmt.Printf("✅ %s uninstalled successfully!\n", name)
	ci.client.notifyInvalidation(EventInstalledChanged)
	ci.client.notifyInvalidation(EventServiceChanged)
	return nil
}

func (ci *CaskInstaller) IsInstalled(name string) (bool, string, error) {
	caskDir, err := ci.getCaskDir()
	if err != nil {
		return false, "", err
	}

	entries, err := os.ReadDir(caskDir)
	if err != nil {
		return false, "", nil
	}

	for _, entry := range entries {
		if entry.Name() == name {
			versionDirs, err := filepath.Glob(filepath.Join(caskDir, name, "*"))
			if err != nil || len(versionDirs) == 0 {
				return false, "", nil
			}
			latest := ""
			for _, vdir := range versionDirs {
				v := filepath.Base(vdir)
				if v != ".receipt.json" && v > latest {
					latest = v
				}
			}
			return true, latest, nil
		}
	}

	return false, "", nil
}

var (
	caskMetadataCache  = make(map[string]*CaskMetadata)
	caskMetadataMutex  sync.RWMutex
	caskMetadataExpiry = time.Hour
)

func (c *Client) GetCaskMetadata(name string) (*CaskMetadata, error) {
	caskMetadataMutex.RLock()
	if cached, ok := caskMetadataCache[name]; ok {
		caskMetadataMutex.RUnlock()
		return cached, nil
	}
	caskMetadataMutex.RUnlock()

	metadata, err := c.FetchCaskMetadata(name)
	if err != nil {
		return nil, err
	}

	caskMetadataMutex.Lock()
	caskMetadataCache[name] = metadata
	caskMetadataMutex.Unlock()

	return metadata, nil
}

func (ci *CaskInstaller) Cleanup() error {
	return nil
}
