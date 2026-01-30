package brew

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type PlatformInfo struct {
	OS               string
	Arch             string
	Version          string
	IsContainer      bool
	IsWSL            bool
	HomebrewPrefix   string
	PlatformID       string
	SupportsServices bool
}

func DetectPlatform() (*PlatformInfo, error) {
	info := &PlatformInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	detectArchitecture(info)
	detectContainer(info)
	detectWSL(info)

	var err error
	info.PlatformID, err = GetPlatform()
	if err != nil {
		return nil, err
	}

	info.HomebrewPrefix = detectHomebrewPrefix()
	info.SupportsServices = !info.IsContainer

	if info.OS == "darwin" {
		info.Version, _ = getMacOSVersion()
	} else if info.OS == "linux" {
		info.Version = detectLinuxDistro()
	}

	return info, nil
}

func detectArchitecture(info *PlatformInfo) {
	switch info.Arch {
	case "amd64":
		info.Arch = "x86_64"
	case "arm64":
		info.Arch = "arm64"
	case "386":
		info.Arch = "i386"
	}
}

func detectContainer(info *PlatformInfo) {
	containerFiles := []string{
		"/.dockerenv",
		"/run/.containerenv",
		"/.singularity.d",
	}

	for _, file := range containerFiles {
		if _, err := os.Stat(file); err == nil {
			info.IsContainer = true
			return
		}
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "lxc") {
			info.IsContainer = true
			return
		}
	}
}

func detectWSL(info *PlatformInfo) {
	if info.OS != "linux" {
		return
	}

	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return
	}

	content := strings.ToLower(string(data))
	if strings.Contains(content, "microsoft") || strings.Contains(content, "wsl") {
		info.IsWSL = true
		info.IsContainer = false
	}
}

func detectLinuxDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, `"`)
			return id
		}
	}

	return "unknown"
}

func detectHomebrewPrefix() string {
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		return p
	}

	paths := []string{
		"/home/linuxbrew/.linuxbrew",
		"/opt/homebrew",
		"/usr/local",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	cmd := exec.Command("brew", "--prefix")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	return "/usr/local"
}

func (p *PlatformInfo) IsSupported() bool {
	if p.OS != "linux" && p.OS != "darwin" {
		return false
	}

	if p.Arch != "x86_64" && p.Arch != "arm64" && p.Arch != "i386" {
		return false
	}

	return true
}

func (p *PlatformInfo) String() string {
	return fmt.Sprintf("%s_%s", p.Arch, p.OS)
}
