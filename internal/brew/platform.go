package brew

import (
	"fmt"
	"runtime"
	"strings"
	"os/exec"
)

// GetPlatform returns the Homebrew-style platform string (e.g., "x86_64_linux", "arm64_sonoma")
func GetPlatform() (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	if osName == "linux" {
		if arch == "amd64" {
			return "x86_64_linux", nil
		}
		if arch == "arm64" {
			return "arm64_linux", nil
		}
		return "", fmt.Errorf("unsupported linux architecture: %s", arch)
	}

	if osName == "darwin" {
		// Verify arch
		if arch != "amd64" && arch != "arm64" {
			return "", fmt.Errorf("unsupported darwin architecture: %s", arch)
		}

		// Get macOS version name
		version, err := getMacOSVersion()
		if err != nil {
			return "", err
		}

		if arch == "arm64" {
			return "arm64_" + version, nil
		}
		return version, nil
	}

	return "", fmt.Errorf("unsupported OS: %s", osName)
}

func getMacOSVersion() (string, error) {
	cmd := exec.Command("sw_vers", "-productVersion")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	ver := strings.TrimSpace(string(out))
	// Valid mappings as of 2026 (Homebrew convention)
	// 14.x -> sonoma
	// 13.x -> ventura
	// 12.x -> monterey
	// 11.x -> big_sur
	
	parts := strings.Split(ver, ".")
	if len(parts) < 1 {
		return "", fmt.Errorf("unknown mac version format: %s", ver)
	}
	
	major := parts[0]
	switch major {
	case "16":
		return "sequoia", nil
	case "15":
		return "sequoia", nil // Fallback/Current guess for 2025/2026
	case "14":
		return "sonoma", nil
	case "13":
		return "ventura", nil
	case "12":
		return "monterey", nil
	case "11":
		return "big_sur", nil
	}
	
	return "", fmt.Errorf("unsupported macOS major version: %s", major)
}
