package brew

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Environment holds all Homebrew-related paths and configuration
type Environment struct {
	OS   string
	Arch string

	Prefix     string
	Repository string
	Cellar     string
	Caskroom   string
	Cache      string
	TapsDir    string

	BinDir   string
	SbinDir  string
	LibDir   string
	ShareDir string
	EtcDir   string
	VarDir   string

	// macOS-specific
	ApplicationsDir string
	FrameworksDir   string

	// Service directories
	UserServiceDir   string
	SystemServiceDir string

	// Shell environment values
	HomebrewPrefix     string
	HomebrewCellar     string
	HomebrewRepository string
	HomebrewCache      string
}

// detectPrefix attempts to find Homebrew prefix from various sources
func detectPrefix() string {
	// 1. Check environment variable
	if prefix := os.Getenv("HOMEBREW_PREFIX"); prefix != "" {
		return prefix
	}

	// 2. Check known platform locations
	switch runtime.GOOS {
	case "darwin":
		// macOS ARM
		if runtime.GOARCH == "arm64" {
			if _, err := os.Stat("/opt/homebrew"); err == nil {
				return "/opt/homebrew"
			}
		}
		// macOS Intel
		if _, err := os.Stat("/usr/local"); err == nil {
			return "/usr/local"
		}

	case "linux":
		if _, err := os.Stat("/home/linuxbrew/.linuxbrew"); err == nil {
			return "/home/linuxbrew/.linuxbrew"
		}
		// Legacy Linuxbrew
		if _, err := os.Stat("/usr/local"); err == nil {
			return "/usr/local"
		}
	}

	return ""
}

// detectRepository finds the Homebrew repository location
func detectRepository(prefix string) string {
	// 1. Check environment variable
	if repo := os.Getenv("HOMEBREW_REPOSITORY"); repo != "" {
		return repo
	}

	// 2. Default location within prefix
	defaultRepo := filepath.Join(prefix, "Homebrew")
	if _, err := os.Stat(defaultRepo); err == nil {
		return defaultRepo
	}

	// 3. Linuxbrew or older Homebrew location
	if runtime.GOOS == "linux" {
		return defaultRepo
	}

	return defaultRepo
}

// detectCache finds the Homebrew cache directory
func detectCache() string {
	// 1. Check environment variable
	if cache := os.Getenv("HOMEBREW_CACHE"); cache != "" {
		return cache
	}

	// 2. Platform-specific defaults
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "Homebrew")
	case "linux":
		return filepath.Join(home, ".cache", "Homebrew")
	default:
		return filepath.Join(home, ".cache", "Homebrew")
	}
}

// GetEnvironment creates a new Environment with all Homebrew paths
func GetEnvironment() (*Environment, error) {
	prefix := detectPrefix()
	if prefix == "" {
		return nil, fmt.Errorf("could not detect Homebrew prefix; set HOMEBREW_PREFIX environment variable")
	}

	repo := detectRepository(prefix)
	cache := detectCache()

	env := &Environment{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,

		Prefix:     prefix,
		Repository: repo,
		Cellar:     filepath.Join(prefix, "Cellar"),
		Caskroom:   filepath.Join(prefix, "Caskroom"),
		Cache:      cache,
		TapsDir:    filepath.Join(repo, "Library", "Taps"),

		BinDir:   filepath.Join(prefix, "bin"),
		SbinDir:  filepath.Join(prefix, "sbin"),
		LibDir:   filepath.Join(prefix, "lib"),
		ShareDir: filepath.Join(prefix, "share"),
		EtcDir:   filepath.Join(prefix, "etc"),
		VarDir:   filepath.Join(prefix, "var"),

		HomebrewPrefix:     prefix,
		HomebrewCellar:     filepath.Join(prefix, "Cellar"),
		HomebrewRepository: repo,
		HomebrewCache:      cache,
	}

	// macOS-specific paths
	if runtime.GOOS == "darwin" {
		env.ApplicationsDir = "/Applications"
		env.FrameworksDir = filepath.Join(prefix, "Frameworks")
		env.UserServiceDir = filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
		env.SystemServiceDir = "/Library/LaunchDaemons"
	} else {
		// Linux paths
		env.UserServiceDir = filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
		env.SystemServiceDir = "/etc/systemd/system"
	}

	return env, nil
}

// GetFastbrewEnvironment returns paths for fastbrew-specific data
func GetFastbrewEnvironment() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("could not get home directory: %w", err)
	}

	fastbrewDir := filepath.Join(home, ".fastbrew")
	cacheDir := filepath.Join(fastbrewDir, "cache")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", "", fmt.Errorf("could not create fastbrew directory: %w", err)
	}

	return fastbrewDir, cacheDir, nil
}

// GetEnvPath returns the PATH with Homebrew bin directories
func (e *Environment) GetEnvPath() string {
	paths := []string{e.BinDir}

	// Add system paths
	if e.OS == "darwin" {
		paths = append(paths, "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin")
	} else {
		paths = append(paths, "/usr/local/bin", "/usr/bin", "/bin", "/usr/local/sbin", "/usr/sbin", "/sbin")
	}

	return strings.Join(paths, string(os.PathListSeparator))
}

// GetEnvManpath returns the MANPATH with Homebrew man directories
func (e *Environment) GetEnvManpath() string {
	manDir := filepath.Join(e.Prefix, "share", "man")
	return manDir
}

// ShellEnv generates shell configuration for the given shell
func (e *Environment) ShellEnv(shell string) string {
	var sb strings.Builder

	switch shell {
	case "fish":
		fmt.Fprintf(&sb, "set -gx HOMEBREW_PREFIX %s\n", e.Prefix)
		fmt.Fprintf(&sb, "set -gx HOMEBREW_CELLAR %s\n", e.Cellar)
		fmt.Fprintf(&sb, "set -gx HOMEBREW_REPOSITORY %s\n", e.Repository)
		fmt.Fprintf(&sb, "set -gx PATH %s $PATH\n", e.BinDir)
		fmt.Fprintf(&sb, "set -gx MANPATH %s $MANPATH\n", e.GetEnvManpath())
	case "zsh", "bash", "sh":
		fmt.Fprintf(&sb, "export HOMEBREW_PREFIX=\"%s\"\n", e.Prefix)
		fmt.Fprintf(&sb, "export HOMEBREW_CELLAR=\"%s\"\n", e.Cellar)
		fmt.Fprintf(&sb, "export HOMEBREW_REPOSITORY=\"%s\"\n", e.Repository)
		fmt.Fprintf(&sb, "export PATH=\"%s:$PATH\"\n", e.BinDir)
		fmt.Fprintf(&sb, "export MANPATH=\"%s:$MANPATH\"\n", e.GetEnvManpath())
	default:
		// Default to bash-style
		fmt.Fprintf(&sb, "export HOMEBREW_PREFIX=\"%s\"\n", e.Prefix)
		fmt.Fprintf(&sb, "export HOMEBREW_CELLAR=\"%s\"\n", e.Cellar)
		fmt.Fprintf(&sb, "export HOMEBREW_REPOSITORY=\"%s\"\n", e.Repository)
		fmt.Fprintf(&sb, "export PATH=\"%s:$PATH\"\n", e.BinDir)
		fmt.Fprintf(&sb, "export MANPATH=\"%s:$MANPATH\"\n", e.GetEnvManpath())
	}

	return sb.String()
}

// CellarPath returns the path for a specific formula version
func (e *Environment) CellarPath(formula, version string) string {
	return filepath.Join(e.Cellar, formula, version)
}

// CaskroomPath returns the path for a specific cask version
func (e *Environment) CaskroomPath(cask, version string) string {
	return filepath.Join(e.Caskroom, cask, version)
}

// TapPath returns the path for a tap repository
func (e *Environment) TapPath(tap string) string {
	// tap is expected to be "user/repo" format
	parts := strings.SplitN(tap, "/", 2)
	if len(parts) != 2 {
		return ""
	}

	user, repo := parts[0], parts[1]
	// Homebrew convention: repos are prefixed with "homebrew-"
	if !strings.HasPrefix(repo, "homebrew-") {
		repo = "homebrew-" + repo
	}

	return filepath.Join(e.TapsDir, user, repo)
}

// TapFromPath determines the tap name from a path
func (e *Environment) TapFromPath(path string) string {
	rel, err := filepath.Rel(e.TapsDir, path)
	if err != nil {
		return ""
	}

	// Check if path is outside TapsDir
	if strings.HasPrefix(rel, "..") {
		return ""
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return ""
	}

	user := parts[0]
	repo := parts[1]

	// Remove homebrew- prefix for canonical name
	repo = strings.TrimPrefix(repo, "homebrew-")

	return user + "/" + repo
}
