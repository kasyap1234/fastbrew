package brew

import (
	"fmt"
	"os"
	"path/filepath"
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
	binDir := filepath.Join(cellarPath, "bin")
	result := &LinkResult{
		Package:  name,
		Binaries: make([]string, 0),
		Success:  true,
	}

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return result, nil
	}

	targetBin := filepath.Join(c.Prefix, "bin")
	if !dryRun {
		if err := os.MkdirAll(targetBin, 0755); err != nil {
			result.Success = false
			result.Errors = append(result.Errors, err)
			return result, err
		}
	}

	entries, err := os.ReadDir(binDir)
	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err)
		return result, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		src := filepath.Join(binDir, entry.Name())
		dst := filepath.Join(targetBin, entry.Name())

		result.Binaries = append(result.Binaries, entry.Name())

		if dryRun {
			continue
		}

		if _, err := os.Lstat(dst); err == nil {
			os.Remove(dst)
		}

		if err := os.Symlink(src, dst); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to link %s: %w", entry.Name(), err))
			result.Success = false
		}
	}

	return result, nil
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
	versions, err := os.ReadDir(pkgDir)
	if err != nil {
		return err
	}

	targetBin := filepath.Join(c.Prefix, "bin")

	for _, vEntry := range versions {
		if !vEntry.IsDir() {
			continue
		}

		binDir := filepath.Join(pkgDir, vEntry.Name(), "bin")
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			continue
		}

		binaries, err := os.ReadDir(binDir)
		if err != nil {
			continue
		}

		for _, bin := range binaries {
			if bin.IsDir() {
				continue
			}

			linkPath := filepath.Join(targetBin, bin.Name())
			if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
				os.Remove(linkPath)
			}
		}
	}

	return nil
}
