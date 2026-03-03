package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	HomebrewTapsLinux    = "/usr/local/Library/Taps"
	HomebrewTapsDarwin   = "/opt/homebrew/Library/Taps"
	HomebrewPrefixLinux  = "/usr/local"
	HomebrewPrefixDarwin = "/opt/homebrew"
)

var (
	homebrewPrefix   string
	homebrewTapsDir  string
	homebrewCellar   string
	homebrewCaskroom string
	detectErr        error
	onceInit         sync.Once
)

func detectHomebrewPaths() {
	onceInit.Do(func() {
		goos := runtime.GOOS
		if goos == "darwin" {
			homebrewPrefix = "/opt/homebrew"
			homebrewTapsDir = "/opt/homebrew/Library/Taps"
			homebrewCellar = "/opt/homebrew/Cellar"
			homebrewCaskroom = "/opt/homebrew/Caskroom"
		} else {
			homebrewPrefix = "/usr/local"
			homebrewTapsDir = "/usr/local/Library/Taps"
			homebrewCellar = "/usr/local/Cellar"
			homebrewCaskroom = "/usr/local/Caskroom"
		}
	})
}

type Tap struct {
	Name        string    `json:"name"`
	RemoteURL   string    `json:"remote_url"`
	LocalPath   string    `json:"local_path"`
	InstalledAt time.Time `json:"installed_at"`
	IsCustom    bool      `json:"is_custom"`
}

type TapInfo struct {
	Tap       Tap
	Formulae  []string
	Casks     []string
	Installed []string
}

type TapManager struct {
	registryPath string
	taps         map[string]Tap
	mu           sync.RWMutex
	onInvalid    func(event string)
}

func NewTapManager() (*TapManager, error) {
	detectHomebrewPaths()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get home directory: %w", err)
	}

	fastbrewDir := filepath.Join(homeDir, ".fastbrew")
	if err := os.MkdirAll(fastbrewDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create fastbrew directory: %w", err)
	}

	registryPath := filepath.Join(fastbrewDir, "taps.json")
	tm := &TapManager{
		registryPath: registryPath,
		taps:         make(map[string]Tap),
	}

	if err := tm.loadRegistry(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return tm, nil
}

func (tm *TapManager) loadRegistry() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	data, err := os.ReadFile(tm.registryPath)
	if err != nil {
		return err
	}

	var taps []Tap
	if err := json.Unmarshal(data, &taps); err != nil {
		return fmt.Errorf("could not parse taps registry: %w", err)
	}

	for _, tap := range taps {
		tm.taps[tap.Name] = tap
	}

	return nil
}

func (tm *TapManager) saveRegistry() error {
	tm.mu.RLock()
	taps := make([]Tap, 0, len(tm.taps))
	for _, tap := range tm.taps {
		taps = append(taps, tap)
	}
	tm.mu.RUnlock()

	data, err := json.MarshalIndent(taps, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal taps registry: %w", err)
	}

	if err := os.WriteFile(tm.registryPath, data, 0644); err != nil {
		return fmt.Errorf("could not save taps registry: %w", err)
	}

	return nil
}

func (tm *TapManager) SetInvalidationHook(fn func(event string)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onInvalid = fn
}

func (tm *TapManager) notifyInvalidation(event string) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.onInvalid != nil {
		tm.onInvalid(event)
	}
}

func normalizeTapRepoInput(repo string) (string, string, error) {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimSuffix(repo, "/")

	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://") {
		return "", repo, nil
	}

	if strings.HasPrefix(repo, "git@") {
		repo = strings.TrimPrefix(repo, "git@")
		repo = strings.ReplaceAll(repo, ":", "/")
		if strings.HasPrefix(repo, "github.com/") {
			repo = strings.TrimPrefix(repo, "github.com/")
			return repo, fmt.Sprintf("https://github.com/%s.git", repo), nil
		}
		return repo, "", nil
	}

	if strings.Count(repo, "/") == 1 {
		parts := strings.Split(repo, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			fullURL := fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1])
			return repo, fullURL, nil
		}
	}

	return "", "", fmt.Errorf("invalid tap repo format: %s (expected user/repo or full URL)", repo)
}

func tapLocalPath(repo string) string {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	user, repoName := parts[0], parts[1]

	if !strings.HasPrefix(repoName, "homebrew-") {
		repoName = "homebrew-" + repoName
	}

	return filepath.Join(homebrewTapsDir, user, repoName)
}

func (tm *TapManager) ListTaps() ([]Tap, error) {
	taps := make([]Tap, 0)

	entries, err := os.ReadDir(homebrewTapsDir)
	if err != nil {
		if os.IsNotExist(err) {
			tm.mu.RLock()
			for _, tap := range tm.taps {
				taps = append(taps, tap)
			}
			tm.mu.RUnlock()
			return taps, nil
		}
		return nil, err
	}

	for _, userEntry := range entries {
		if !userEntry.IsDir() {
			continue
		}

		userDir := filepath.Join(homebrewTapsDir, userEntry.Name())
		repoEntries, err := os.ReadDir(userDir)
		if err != nil {
			continue
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}

			tapPath := filepath.Join(userDir, repoEntry.Name())
			gitDir := filepath.Join(tapPath, ".git")
			if _, err := os.Stat(gitDir); err != nil {
				continue
			}

			repoName := repoEntry.Name()
			if strings.HasPrefix(repoName, "homebrew-") {
				repoName = strings.TrimPrefix(repoName, "homebrew-")
			}
			fullRepo := fmt.Sprintf("%s/%s", userEntry.Name(), repoName)

			stat, _ := os.Stat(tapPath)

			tap := Tap{
				Name:        fullRepo,
				LocalPath:   tapPath,
				InstalledAt: time.Now(),
				IsCustom:    !strings.HasPrefix(fullRepo, "homebrew/"),
			}

			if stat != nil {
				tap.InstalledAt = stat.ModTime()
			}

			cmd := exec.Command("git", "-C", tapPath, "remote", "get-url", "origin")
			if output, err := cmd.Output(); err == nil {
				tap.RemoteURL = strings.TrimSpace(string(output))
			}

			taps = append(taps, tap)

			tm.mu.Lock()
			tm.taps[fullRepo] = tap
			tm.mu.Unlock()
		}
	}

	tm.mu.RLock()
	for name, tap := range tm.taps {
		found := false
		for _, t := range taps {
			if t.Name == name {
				found = true
				break
			}
		}
		if !found && tap.LocalPath != "" {
			if _, err := os.Stat(tap.LocalPath); err == nil {
				taps = append(taps, tap)
			}
		}
	}
	tm.mu.RUnlock()

	if err := tm.saveRegistry(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save tap registry: %v\n", err)
	}

	return taps, nil
}

func (tm *TapManager) Tap(repo string, full bool) error {
	repoName, remoteURL, err := normalizeTapRepoInput(repo)
	if err != nil {
		return err
	}

	if remoteURL == "" {
		return fmt.Errorf("could not determine remote URL for %s", repo)
	}

	localPath := tapLocalPath(repoName)
	if localPath == "" {
		return fmt.Errorf("could not determine local path for %s", repo)
	}

	if _, err := os.Stat(localPath); err == nil {
		cmd := exec.Command("git", "-C", localPath, "remote", "get-url", "origin")
		if output, err := cmd.Output(); err == nil {
			existingRemote := strings.TrimSpace(string(output))
			if existingRemote != remoteURL {
				return fmt.Errorf("tap already exists with different remote: %s (expected %s)", existingRemote, remoteURL)
			}
		}

		fmt.Printf("Tap %s already present\n", repoName)
		tm.mu.Lock()
		tm.taps[repoName] = Tap{
			Name:        repoName,
			RemoteURL:   remoteURL,
			LocalPath:   localPath,
			InstalledAt: time.Now(),
			IsCustom:    !strings.HasPrefix(repoName, "homebrew/"),
		}
		tm.mu.Unlock()
		return tm.saveRegistry()
	}

	parentDir := filepath.Dir(localPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("could not create tap parent directory: %w", err)
	}

	fmt.Printf("Cloning into '%s'...\n", localPath)

	args := []string{"clone"}
	if !full {
		args = append(args, "--depth=1")
	}
	args = append(args, remoteURL, localPath)

	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s: %w", remoteURL, err)
	}

	if err := tm.validateTapContents(localPath); err != nil {
		os.RemoveAll(localPath)
		return fmt.Errorf("tap validation failed: %w", err)
	}

	tap := Tap{
		Name:        repoName,
		RemoteURL:   remoteURL,
		LocalPath:   localPath,
		InstalledAt: time.Now(),
		IsCustom:    !strings.HasPrefix(repoName, "homebrew/"),
	}

	tm.mu.Lock()
	tm.taps[repoName] = tap
	tm.mu.Unlock()

	if err := tm.saveRegistry(); err != nil {
		return fmt.Errorf("tap added but failed to save registry: %w", err)
	}
	tm.notifyInvalidation(EventTapChanged)

	return nil
}

func (tm *TapManager) validateTapContents(localPath string) error {
	formulaeDir := filepath.Join(localPath, "Formula")
	casksDir := filepath.Join(localPath, "Casks")

	hasFormulae := false
	if entries, err := os.ReadDir(formulaeDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
				hasFormulae = true
				break
			}
		}
	}

	hasCasks := false
	if entries, err := os.ReadDir(casksDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
				hasCasks = true
				break
			}
		}
	}

	if !hasFormulae && !hasCasks {
		if entries, err := os.ReadDir(localPath); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
					return nil
				}
			}
		}
		return fmt.Errorf("tap does not contain any formulae or casks")
	}

	return nil
}

func (tm *TapManager) Untap(repo string, force bool) error {
	repoName, _, err := normalizeTapRepoInput(repo)
	if err != nil {
		return err
	}

	localPath := tapLocalPath(repoName)
	if localPath == "" {
		localPath = tm.findLocalPathForRepo(repoName)
	}

	if localPath == "" {
		return fmt.Errorf("tap %s not found", repoName)
	}

	if _, err := os.Stat(localPath); err != nil {
		return fmt.Errorf("tap %s not found", repoName)
	}

	if !force {
		installed, err := tm.getInstalledFromTap(localPath)
		if err != nil {
			return fmt.Errorf("failed to check installed formulae: %w", err)
		}

		if len(installed) > 0 {
			return fmt.Errorf("refusing to untap %s; installed formulae/casks found: %s. Use --force to override", repoName, strings.Join(installed, ", "))
		}
	}

	if err := os.RemoveAll(localPath); err != nil {
		return fmt.Errorf("failed to remove tap directory: %w", err)
	}

	tm.mu.Lock()
	delete(tm.taps, repoName)
	tm.mu.Unlock()

	if err := tm.saveRegistry(); err != nil {
		return fmt.Errorf("untap succeeded but failed to save registry: %w", err)
	}
	tm.notifyInvalidation(EventTapChanged)

	return nil
}

func (tm *TapManager) findLocalPathForRepo(repoName string) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tap, ok := tm.taps[repoName]; ok {
		return tap.LocalPath
	}

	return ""
}

func (tm *TapManager) getInstalledFromTap(tapPath string) ([]string, error) {
	var installed []string

	formulaePaths := []string{
		filepath.Join(tapPath, "Formula"),
		tapPath,
	}

	for _, formulaPath := range formulaePaths {
		if entries, err := os.ReadDir(formulaPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rb") {
					continue
				}
				name := strings.TrimSuffix(entry.Name(), ".rb")

				pkgPath := filepath.Join(homebrewCellar, name)
				if _, err := os.Stat(pkgPath); err == nil {
					installed = append(installed, name)
				}
			}
		}
	}

	casksPath := filepath.Join(tapPath, "Casks")
	if entries, err := os.ReadDir(casksPath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rb") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".rb")

			caskPath := filepath.Join(homebrewCaskroom, name)
			if _, err := os.Stat(caskPath); err == nil {
				installed = append(installed, name)
			}
		}
	}

	return installed, nil
}

func (tm *TapManager) GetTapInfo(repo string, installedOnly bool) (*TapInfo, error) {
	repoName, _, err := normalizeTapRepoInput(repo)
	if err != nil {
		return nil, err
	}

	localPath := tapLocalPath(repoName)
	if localPath == "" {
		localPath = tm.findLocalPathForRepo(repoName)
	}

	if localPath == "" {
		return nil, fmt.Errorf("tap %s not found", repoName)
	}
	if _, err := os.Stat(localPath); err != nil {
		return nil, fmt.Errorf("tap %s not found", repoName)
	}

	cmd := exec.Command("git", "-C", localPath, "remote", "get-url", "origin")
	remoteURL := ""
	if output, err := cmd.Output(); err == nil {
		remoteURL = strings.TrimSpace(string(output))
	}

	tap := Tap{
		Name:      repoName,
		RemoteURL: remoteURL,
		LocalPath: localPath,
		IsCustom:  !strings.HasPrefix(repoName, "homebrew/"),
	}

	info := &TapInfo{
		Tap: tap,
	}

	formulaeDirs := []string{
		filepath.Join(localPath, "Formula"),
		localPath,
	}

	for _, formulaeDir := range formulaeDirs {
		if entries, err := os.ReadDir(formulaeDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rb") {
					continue
				}
				name := strings.TrimSuffix(entry.Name(), ".rb")

				found := false
				for _, f := range info.Formulae {
					if f == name {
						found = true
						break
					}
				}
				if !found {
					info.Formulae = append(info.Formulae, name)
				}
			}
		}
	}

	casksDir := filepath.Join(localPath, "Casks")
	if entries, err := os.ReadDir(casksDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rb") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".rb")
			info.Casks = append(info.Casks, name)
		}
	}

	if installedOnly {
		installed, err := tm.getInstalledFromTap(localPath)
		if err == nil {
			info.Installed = installed
		}
	}

	return info, nil
}

func (tm *TapManager) GetTap(repo string) (Tap, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tap, exists := tm.taps[repo]
	return tap, exists
}

func isValidTapRepo(repo string) bool {
	_, _, err := normalizeTapRepoInput(repo)
	return err == nil
}
