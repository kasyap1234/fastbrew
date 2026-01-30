package brew

import (
	"fmt"
	"os"
)

type CacheCorruptionChecker struct {
	cacheDir string
	verbose  bool
}

func NewCacheCorruptionChecker(cacheDir string, verbose bool) *CacheCorruptionChecker {
	return &CacheCorruptionChecker{
		cacheDir: cacheDir,
		verbose:  verbose,
	}
}

type CorruptionReport struct {
	CorruptedFiles []string
	FixedFiles     []string
	Errors         []error
}

func (c *CacheCorruptionChecker) CheckAndRepair() (*CorruptionReport, error) {
	report := &CorruptionReport{}

	validator := NewCacheValidator(c.cacheDir)
	statuses, err := validator.ValidateAll()
	if err != nil {
		return nil, err
	}

	for _, status := range statuses {
		if !status.Valid {
			report.CorruptedFiles = append(report.CorruptedFiles, status.Path)
			if c.verbose {
				fmt.Printf("Corrupted cache detected: %s (%v)\n", status.Path, status.Error)
			}

			if err := os.Remove(status.Path); err != nil {
				report.Errors = append(report.Errors, fmt.Errorf("failed to remove %s: %w", status.Path, err))
			} else {
				report.FixedFiles = append(report.FixedFiles, status.Path)
				if c.verbose {
					fmt.Printf("Removed corrupted cache: %s\n", status.Path)
				}
			}
		}
	}

	return report, nil
}

func (c *CacheCorruptionChecker) IsTruncated(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}

	if info.Size() == 0 {
		return true
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}

	if len(data) < 100 {
		return true
	}

	return false
}
