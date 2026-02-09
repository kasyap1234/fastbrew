package brew

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type LinkResult struct {
	Package  string
	Binaries []string
	Errors   []error
	Success  bool
}

type BinaryConflict struct {
	BinaryName string
	FirstPkg   string
	SecondPkg  string
}

func (c *Client) Link(name, version string) (*LinkResult, error) {
	return c.linkInternal(name, version, false)
}

func (c *Client) LinkDryRun(name, version string) (*LinkResult, error) {
	return c.linkInternal(name, version, true)
}

func (c *Client) linkInternal(name, version string, dryRun bool) (*LinkResult, error) {
	cellarPath := filepath.Join(c.Prefix, "Cellar", name, version)
	result := &LinkResult{
		Package:  name,
		Binaries: make([]string, 0),
		Success:  true,
	}

	optDir := filepath.Join(c.Prefix, "opt")
	optLink := filepath.Join(optDir, name)
	if !dryRun {
		os.MkdirAll(optDir, 0755)
		if existing, err := os.Lstat(optLink); err == nil {
			if existing.Mode()&os.ModeSymlink != 0 {
				os.Remove(optLink)
			}
		}
		if err := os.Symlink(cellarPath, optLink); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create opt link: %w", err))
			result.Success = false
		}
	}

	linkDirs := []string{"bin", "sbin", "lib", "include", "share", "etc"}
	if runtime.GOOS == "darwin" {
		linkDirs = append(linkDirs, "Frameworks")
	}

	for _, dir := range linkDirs {
		srcDir := filepath.Join(cellarPath, dir)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}
		targetDir := filepath.Join(c.Prefix, dir)
		if !dryRun {
			os.MkdirAll(targetDir, 0755)
		}
		c.linkDir(srcDir, targetDir, cellarPath, result, dryRun)
	}

	return result, nil
}

func (c *Client) linkDir(srcDir, targetDir, cellarPath string, result *LinkResult, dryRun bool) {
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == srcDir {
			return nil
		}

		rel, _ := filepath.Rel(srcDir, path)
		dst := filepath.Join(targetDir, rel)

		if info.IsDir() {
			if !dryRun {
				os.MkdirAll(dst, 0755)
			}
			return nil
		}

		result.Binaries = append(result.Binaries, rel)

		if dryRun {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create dir for %s: %w", rel, err))
			return nil
		}

		if _, err := os.Lstat(dst); err == nil {
			os.Remove(dst)
		}

		if err := os.Symlink(path, dst); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to link %s: %w", rel, err))
			result.Success = false
		}

		return nil
	})
}

type ConflictTracker struct {
	mu        sync.RWMutex
	binaries  map[string]string
	conflicts []BinaryConflict
}

func NewConflictTracker() *ConflictTracker {
	return &ConflictTracker{
		binaries:  make(map[string]string),
		conflicts: make([]BinaryConflict, 0),
	}
}

func (ct *ConflictTracker) CheckAndTrack(binary, pkg string) string {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if existingPkg, exists := ct.binaries[binary]; exists {
		if existingPkg != pkg {
			ct.conflicts = append(ct.conflicts, BinaryConflict{
				BinaryName: binary,
				FirstPkg:   existingPkg,
				SecondPkg:  pkg,
			})
		}
		return existingPkg
	}

	ct.binaries[binary] = pkg
	return ""
}

func (ct *ConflictTracker) GetConflicts() []BinaryConflict {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make([]BinaryConflict, len(ct.conflicts))
	copy(result, ct.conflicts)
	return result
}

func (ct *ConflictTracker) HasConflicts() bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	return len(ct.conflicts) > 0
}

func (ct *ConflictTracker) GetConflictingPackages() map[string]bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make(map[string]bool)
	for _, c := range ct.conflicts {
		result[c.FirstPkg] = true
		result[c.SecondPkg] = true
	}
	return result
}

func (ct *ConflictTracker) GetAllTrackedBinaries() map[string]string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range ct.binaries {
		result[k] = v
	}
	return result
}

func (c *Client) Unlink(name string) error {
	pkgDir := filepath.Join(c.Cellar, name)
	cellarPrefix := filepath.Join(c.Cellar, name) + string(filepath.Separator)

	versions, err := os.ReadDir(pkgDir)
	if err != nil {
		return err
	}

	optLink := filepath.Join(c.Prefix, "opt", name)
	if info, err := os.Lstat(optLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
		os.Remove(optLink)
	}

	linkDirs := []string{"bin", "sbin", "lib", "include", "share", "etc"}
	if runtime.GOOS == "darwin" {
		linkDirs = append(linkDirs, "Frameworks")
	}

	for _, vEntry := range versions {
		if !vEntry.IsDir() {
			continue
		}

		for _, dir := range linkDirs {
			srcDir := filepath.Join(pkgDir, vEntry.Name(), dir)
			if _, err := os.Stat(srcDir); os.IsNotExist(err) {
				continue
			}
			targetDir := filepath.Join(c.Prefix, dir)
			c.unlinkDir(srcDir, targetDir, cellarPrefix)
		}
	}

	return nil
}

func (c *Client) unlinkDir(srcDir, targetDir, cellarPrefix string) {
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(srcDir, path)
		linkPath := filepath.Join(targetDir, rel)

		linfo, err := os.Lstat(linkPath)
		if err != nil {
			return nil
		}
		if linfo.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		target, err := os.Readlink(linkPath)
		if err != nil {
			return nil
		}
		if !strings.HasPrefix(target, cellarPrefix) {
			return nil
		}

		os.Remove(linkPath)
		return nil
	})
}
