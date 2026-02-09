package brew

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CheckStatus string

const (
	StatusOK      CheckStatus = "OK"
	StatusWarning CheckStatus = "WARNING"
	StatusError   CheckStatus = "ERROR"
	StatusInfo    CheckStatus = "INFO"
)

type CheckResult struct {
	Name       string
	Status     CheckStatus
	Message    string
	Suggestion string
	Details    []string
}

type Doctor struct {
	client  *Client
	verbose bool
	cache   map[string]interface{}
}

func NewDoctor(client *Client, verbose bool) *Doctor {
	return &Doctor{
		client:  client,
		verbose: verbose,
		cache:   make(map[string]interface{}),
	}
}

func (d *Doctor) RunDiagnostics() []CheckResult {
	var wg sync.WaitGroup
	results := make([]CheckResult, 9)
	var mu sync.Mutex

	type checkFunc struct {
		index int
		name  string
		fn    func() CheckResult
	}

	checks := []checkFunc{
		{0, "Homebrew installation", d.checkHomebrewInstallation},
		{1, "Cellar permissions", d.checkCellarPermissions},
		{2, "Broken symlinks", d.checkBrokenSymlinks},
		{3, "Outdated index", d.checkOutdatedIndex},
		{4, "Disk space", d.checkDiskSpace},
		{5, "Duplicate binaries", d.checkDuplicateBinaries},
		{6, "Unlinked keg-only", d.checkUnlinkedKegOnly},
		{7, "PATH configuration", d.checkPathConfiguration},
		{8, "Cache integrity", d.checkCacheIntegrity},
	}

	for _, check := range checks {
		wg.Add(1)
		go func(cf checkFunc) {
			defer wg.Done()
			result := cf.fn()
			mu.Lock()
			results[cf.index] = result
			mu.Unlock()
		}(check)
	}

	wg.Wait()
	return results
}

func (d *Doctor) checkHomebrewInstallation() CheckResult {
	if _, err := os.Stat(d.client.Prefix); os.IsNotExist(err) {
		return CheckResult{
			Name:       "Homebrew installation",
			Status:     StatusError,
			Message:    "Homebrew installation not found",
			Suggestion: "Install Homebrew from https://brew.sh",
		}
	}

	if _, err := os.Stat(d.client.Cellar); os.IsNotExist(err) {
		return CheckResult{
			Name:       "Homebrew installation",
			Status:     StatusWarning,
			Message:    "Cellar directory does not exist",
			Suggestion: "Run: brew install something to initialize Cellar",
		}
	}

	return CheckResult{
		Name:    "Homebrew installation",
		Status:  StatusOK,
		Message: fmt.Sprintf("Found at %s", d.client.Prefix),
	}
}

func (d *Doctor) checkCellarPermissions() CheckResult {
	info, err := os.Stat(d.client.Cellar)
	if err != nil {
		return CheckResult{
			Name:    "Cellar permissions",
			Status:  StatusOK,
			Message: "Cellar not initialized yet",
		}
	}

	mode := info.Mode().Perm()
	if mode&0200 == 0 {
		return CheckResult{
			Name:       "Cellar permissions",
			Status:     StatusError,
			Message:    "Cellar not writable",
			Suggestion: fmt.Sprintf("Run: sudo chown -R $(whoami) %s", d.client.Cellar),
		}
	}

	return CheckResult{
		Name:    "Cellar permissions",
		Status:  StatusOK,
		Message: "Writable",
	}
}

func (d *Doctor) checkBrokenSymlinks() CheckResult {
	binDir := filepath.Join(d.client.Prefix, "bin")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Broken symlinks",
			Status:  StatusOK,
			Message: "No bin directory to check",
		}
	}

	entries, err := os.ReadDir(binDir)
	if err != nil {
		return CheckResult{
			Name:    "Broken symlinks",
			Status:  StatusOK,
			Message: "Cannot read bin directory",
		}
	}

	var broken []string
	for _, entry := range entries {
		path := filepath.Join(binDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			_, err := os.Stat(path)
			if err != nil {
				broken = append(broken, entry.Name())
			}
		}
	}

	if len(broken) > 0 {
		return CheckResult{
			Name:       "Broken symlinks",
			Status:     StatusError,
			Message:    fmt.Sprintf("%d broken symlink(s) found", len(broken)),
			Suggestion: "Run: fastbrew cleanup",
			Details:    broken,
		}
	}

	return CheckResult{
		Name:    "Broken symlinks",
		Status:  StatusOK,
		Message: "None found",
	}
}

func (d *Doctor) checkOutdatedIndex() CheckResult {
	cacheDir, err := d.client.GetCacheDir()
	if err != nil {
		return CheckResult{
			Name:    "Outdated index",
			Status:  StatusOK,
			Message: "Cache directory not available",
		}
	}

	fPath := filepath.Join(cacheDir, "formula.json.zst")
	info, err := os.Stat(fPath)
	if os.IsNotExist(err) {
		return CheckResult{
			Name:       "Outdated index",
			Status:     StatusWarning,
			Message:    "Index not downloaded",
			Suggestion: "Run: fastbrew update",
		}
	}

	age := time.Since(info.ModTime())
	days := int(age.Hours() / 24)

	if days > 7 {
		return CheckResult{
			Name:       "Outdated index",
			Status:     StatusWarning,
			Message:    fmt.Sprintf("Last updated %d days ago", days),
			Suggestion: "Run: fastbrew update",
		}
	}

	return CheckResult{
		Name:    "Outdated index",
		Status:  StatusOK,
		Message: fmt.Sprintf("Updated %d day(s) ago", days),
	}
}

func (d *Doctor) checkDiskSpace() CheckResult {
	cmd := exec.Command("df", "-h", d.client.Prefix)
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "Disk space",
			Status:  StatusOK,
			Message: "Unable to check disk space",
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return CheckResult{
			Name:    "Disk space",
			Status:  StatusOK,
			Message: "Unable to parse disk space",
		}
	}

	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) >= 4 {
		available := fields[3]
		return CheckResult{
			Name:    "Disk space",
			Status:  StatusOK,
			Message: fmt.Sprintf("%s available", available),
		}
	}

	return CheckResult{
		Name:    "Disk space",
		Status:  StatusOK,
		Message: "Disk space check completed",
	}
}

func (d *Doctor) checkDuplicateBinaries() CheckResult {
	binDir := filepath.Join(d.client.Prefix, "bin")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Duplicate binaries",
			Status:  StatusOK,
			Message: "No bin directory to check",
		}
	}

	entries, err := os.ReadDir(binDir)
	if err != nil {
		return CheckResult{
			Name:    "Duplicate binaries",
			Status:  StatusOK,
			Message: "Cannot read bin directory",
		}
	}

	binaryMap := make(map[string][]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(binDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}

		dest, err := os.Readlink(path)
		if err != nil {
			continue
		}

		binaryMap[entry.Name()] = append(binaryMap[entry.Name()], dest)
	}

	var conflicts []string
	for name, targets := range binaryMap {
		if len(targets) > 1 {
			conflicts = append(conflicts, fmt.Sprintf("%s -> %v", name, targets))
		}
	}

	if len(conflicts) > 0 {
		return CheckResult{
			Name:       "Duplicate binaries",
			Status:     StatusWarning,
			Message:    fmt.Sprintf("%d conflict(s) found", len(conflicts)),
			Suggestion: "Use 'brew unlink <formula>' for one of the packages",
			Details:    conflicts,
		}
	}

	return CheckResult{
		Name:    "Duplicate binaries",
		Status:  StatusOK,
		Message: "No conflicts found",
	}
}

func (d *Doctor) checkUnlinkedKegOnly() CheckResult {
	entries, err := os.ReadDir(d.client.Cellar)
	if err != nil {
		return CheckResult{
			Name:    "Unlinked keg-only",
			Status:  StatusOK,
			Message: "No packages installed",
		}
	}

	var unlinked []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		versions, err := os.ReadDir(filepath.Join(d.client.Cellar, entry.Name()))
		if err != nil || len(versions) == 0 {
			continue
		}

		latestVersion := versions[len(versions)-1].Name()
		binDir := filepath.Join(d.client.Cellar, entry.Name(), latestVersion, "bin")

		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			continue
		}

		binEntries, err := os.ReadDir(binDir)
		if err != nil {
			continue
		}

		for _, bin := range binEntries {
			if bin.IsDir() {
				continue
			}

			linkPath := filepath.Join(d.client.Prefix, "bin", bin.Name())
			if _, err := os.Lstat(linkPath); os.IsNotExist(err) {
				unlinked = append(unlinked, entry.Name())
				break
			}
		}
	}

	if len(unlinked) > 0 {
		return CheckResult{
			Name:       "Unlinked keg-only",
			Status:     StatusWarning,
			Message:    fmt.Sprintf("%d package(s) not linked", len(unlinked)),
			Suggestion: "Run: fastbrew link <package> or check 'brew info <package>' for linking instructions",
			Details:    unlinked,
		}
	}

	return CheckResult{
		Name:    "Unlinked keg-only",
		Status:  StatusOK,
		Message: "All packages linked",
	}
}

func (d *Doctor) checkPathConfiguration() CheckResult {
	path := os.Getenv("PATH")
	if path == "" {
		return CheckResult{
			Name:       "PATH configuration",
			Status:     StatusError,
			Message:    "PATH environment variable is empty",
			Suggestion: "Set PATH in your shell configuration",
		}
	}

	binPath := filepath.Join(d.client.Prefix, "bin")
	paths := strings.Split(path, string(os.PathListSeparator))

	found := false
	for _, p := range paths {
		if strings.TrimSpace(p) == binPath {
			found = true
			break
		}
	}

	if !found {
		return CheckResult{
			Name:       "PATH configuration",
			Status:     StatusWarning,
			Message:    fmt.Sprintf("%s not in PATH", binPath),
			Suggestion: fmt.Sprintf("Add 'export PATH=\"%s:$PATH\"' to your shell config", binPath),
		}
	}

	idx := -1
	for i, p := range paths {
		if strings.TrimSpace(p) == binPath {
			idx = i
			break
		}
	}

	if idx > 0 {
		return CheckResult{
			Name:       "PATH configuration",
			Status:     StatusWarning,
			Message:    "Homebrew bin is not first in PATH",
			Suggestion: fmt.Sprintf("Move '%s' to the beginning of PATH for priority", binPath),
		}
	}

	return CheckResult{
		Name:    "PATH configuration",
		Status:  StatusOK,
		Message: fmt.Sprintf("%s is first in PATH", binPath),
	}
}

func (d *Doctor) checkCacheIntegrity() CheckResult {
	cacheDir, err := d.client.GetCacheDir()
	if err != nil {
		return CheckResult{
			Name:    "Cache integrity",
			Status:  StatusOK,
			Message: "Cache directory not available",
		}
	}

	validator := NewCacheValidator(cacheDir)
	statuses, err := validator.ValidateAll()
	if err != nil {
		return CheckResult{
			Name:    "Cache integrity",
			Status:  StatusOK,
			Message: "Unable to validate cache files",
		}
	}

	var invalid []string
	var details []string
	for _, status := range statuses {
		if !status.Valid {
			invalid = append(invalid, filepath.Base(status.Path))
			if status.Error != nil {
				details = append(details, fmt.Sprintf("%s: %s", filepath.Base(status.Path), status.Error.Error()))
			}
		}
	}

	if len(invalid) > 0 {
		return CheckResult{
			Name:       "Cache integrity",
			Status:     StatusError,
			Message:    fmt.Sprintf("%d cache file(s) corrupted or invalid", len(invalid)),
			Suggestion: "Run: fastbrew update",
			Details:    details,
		}
	}

	return CheckResult{
		Name:    "Cache integrity",
		Status:  StatusOK,
		Message: "All cache files valid",
	}
}

func (d *Doctor) PrintResults(results []CheckResult) {
	fmt.Println("🩺 FastBrew Doctor")
	fmt.Println("================")
	fmt.Println()

	var warnings, errors int

	for _, r := range results {
		switch r.Status {
		case StatusOK:
			fmt.Printf("✓ %s: %s\n", r.Name, r.Message)
		case StatusWarning:
			fmt.Printf("⚠️  %s: %s\n", r.Name, r.Message)
			if r.Suggestion != "" {
				fmt.Printf("   %s\n", r.Suggestion)
			}
			warnings++
		case StatusError:
			fmt.Printf("✗ %s: %s\n", r.Name, r.Message)
			if r.Suggestion != "" {
				fmt.Printf("   %s\n", r.Suggestion)
			}
			errors++
		}

		if d.verbose && len(r.Details) > 0 {
			for _, detail := range r.Details {
				fmt.Printf("   - %s\n", detail)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Diagnostic count: %d checks, %d warning(s), %d error(s)\n", len(results), warnings, errors)
}

func (d *Doctor) GetExitCode(results []CheckResult) int {
	for _, r := range results {
		if r.Status == StatusError || r.Status == StatusWarning {
			return 1
		}
	}
	return 0
}
