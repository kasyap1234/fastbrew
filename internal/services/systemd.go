package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SystemdManager manages systemd services on Linux
type SystemdManager struct {
	userServicePaths   []string
	systemServicePaths []string
	parser             *ServiceFileParser
	runner             CommandRunner
}

// NewSystemdManager creates a new SystemdManager with default paths
func NewSystemdManager() *SystemdManager {
	homeDir, _ := os.UserHomeDir()

	return &SystemdManager{
		userServicePaths: []string{
			filepath.Join(homeDir, ".config", "systemd", "user"),
		},
		systemServicePaths: []string{
			"/etc/systemd/system",
			"/usr/lib/systemd/system",
		},
		parser: NewServiceFileParser(),
		runner: &DefaultCommandRunner{},
	}
}

// NewSystemdManagerWithRunner creates a new SystemdManager with a custom command runner (for testing)
func NewSystemdManagerWithRunner(runner CommandRunner) *SystemdManager {
	mgr := NewSystemdManager()
	mgr.runner = runner
	return mgr
}

// ListServices returns a list of all Homebrew systemd services
func (m *SystemdManager) ListServices() ([]Service, error) {
	var services []Service

	servicePaths, err := m.findServiceFiles()
	if err != nil {
		return nil, err
	}

	// Get systemctl output for user services (default, no root required)
	userSystemctlOutput, err := m.getSystemctlList("--user")
	if err != nil {
		return nil, err
	}

	for _, path := range servicePaths {
		service := m.parseServiceFromFile(path, userSystemctlOutput)
		if IsHomebrewService(service.Name) {
			services = append(services, service)
		}
	}

	return services, nil
}

// GetStatus returns the status of a specific service
func (m *SystemdManager) GetStatus(serviceName string) (Service, error) {
	servicePath := m.findServiceFilePath(serviceName)
	if servicePath == "" {
		return Service{}, ServiceNotFoundError{Name: serviceName}
	}

	// Try user services first (default, no root required)
	userSystemctlOutput, err := m.getSystemctlList("--user")
	if err != nil {
		return Service{}, err
	}

	service := m.parseServiceFromFile(servicePath, userSystemctlOutput)
	return service, nil
}

// findServiceFiles finds all .service files in user service directories
func (m *SystemdManager) findServiceFiles() ([]string, error) {
	var paths []string

	for _, dir := range m.userServicePaths {
		files, err := m.scanServiceDirectory(dir)
		if err != nil {
			continue
		}
		paths = append(paths, files...)
	}

	return paths, nil
}

// scanServiceDirectory scans a directory for .service files
func (m *SystemdManager) scanServiceDirectory(dir string) ([]string, error) {
	var paths []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return nil, UserServicePathError{Path: dir, Cause: err}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".service") {
			continue
		}

		paths = append(paths, filepath.Join(dir, name))
	}

	return paths, nil
}

// findServiceFilePath finds the full path to a service file by name
func (m *SystemdManager) findServiceFilePath(serviceName string) string {
	// Try user paths first
	for _, dir := range m.userServicePaths {
		path := filepath.Join(dir, serviceName+".service")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try system paths (require root, warn user)
	for _, dir := range m.systemServicePaths {
		path := filepath.Join(dir, serviceName+".service")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// getSystemctlList runs systemctl list-units and parses the output
func (m *SystemdManager) getSystemctlList(scope string) (map[string]systemctlEntry, error) {
	args := []string{"list-units", "--type=service", "--all", "--no-pager", "--no-legend"}
	if scope != "" {
		args = append([]string{scope}, args...)
	}

	output, err := m.runner.Run("systemctl", args...)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, SystemctlError{
				Command: "list-units",
				Scope:   scope,
				Cause:   err,
				Output:  string(exitErr.Stderr),
			}
		}
		return nil, SystemctlError{Command: "list-units", Scope: scope, Cause: err}
	}

	return m.parser.ParseSystemctlOutput(output), nil
}

// parseServiceFromFile creates a Service from a service file path and systemctl data
func (m *SystemdManager) parseServiceFromFile(servicePath string, systemctlData map[string]systemctlEntry) Service {
	name := GetServiceNameFromPath(servicePath)

	info, err := m.parser.ParseFile(servicePath)
	if err != nil {
		return Service{
			Name:         name,
			Status:       StatusError,
			PlistPath:    servicePath,
			Label:        name,
			LastExitCode: 0,
		}
	}

	label := info.Name
	if label == "" {
		label = name
	}

	entry, exists := systemctlData[label]

	service := Service{
		Name:      name,
		Label:     label,
		PlistPath: servicePath,
	}

	if !exists {
		service.Status = StatusStopped
	} else if entry.Active == "active" {
		service.Status = StatusRunning
		service.Pid = entry.Pid
	} else if entry.Result == "failed" || entry.SubState == "failed" {
		service.Status = StatusError
		service.LastExitCode = entry.ExitCode
	} else {
		service.Status = StatusStopped
	}

	return service
}

// IsUserService checks if a service path is a user service
func (m *SystemdManager) IsUserService(servicePath string) bool {
	homeDir, _ := os.UserHomeDir()
	return strings.HasPrefix(servicePath, homeDir)
}

// IsSystemService checks if a service path is a system service
func (m *SystemdManager) IsSystemService(servicePath string) bool {
	for _, path := range m.systemServicePaths {
		if strings.HasPrefix(servicePath, path) {
			return true
		}
	}
	return false
}

// UserServicePathError indicates an error with the user service directory
type UserServicePathError struct {
	Path  string
	Cause error
}

func (e UserServicePathError) Error() string {
	return "user service path error at " + e.Path + ": " + e.Cause.Error()
}

func (e UserServicePathError) Unwrap() error {
	return e.Cause
}
