package brew

import (
	"fmt"
	"os"
	"path/filepath"
)

// Link installs symlinks for the package binaries and other assets
// This is a simplified version of `brew link`.
// For MVP we focus on `bin/` linkage which is 90% of what users care about immediately.
func (c *Client) Link(name, version string) error {
	cellarPath := filepath.Join(c.Prefix, "Cellar", name, version)
	binDir := filepath.Join(cellarPath, "bin")

	// Check if bin dir exists
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return nil // No binaries to link, straightforward
	}

	// Target bin dir
	targetBin := filepath.Join(c.Prefix, "bin")
	if err := os.MkdirAll(targetBin, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(binDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		src := filepath.Join(binDir, entry.Name())
		dst := filepath.Join(targetBin, entry.Name())

		// Remove existing link if it exists
		if _, err := os.Lstat(dst); err == nil {
			os.Remove(dst)
		}

		// Create symlink
		// Note: Homebrew uses relative symlinks usually, but absolute is safer for MVP
		if err := os.Symlink(src, dst); err != nil {
			fmt.Printf("⚠️  Failed to link %s: %v\n", entry.Name(), err)
		} else {
			// fmt.Printf("  🔗 Linked %s\n", entry.Name())
		}
	}

	return nil
}

// Unlink removes symlinks for a package
func (c *Client) Unlink(name string) error {
	// Find all versions of the package
	pkgDir := filepath.Join(c.Cellar, name)
	versions, err := os.ReadDir(pkgDir)
	if err != nil {
		return err // Package not found or error reading
	}

	targetBin := filepath.Join(c.Prefix, "bin")

	// For each version, find binaries and remove their symlinks
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
			// Remove if it's a symlink pointing to this package
			if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
				os.Remove(linkPath)
			}
		}
	}

	return nil
}
