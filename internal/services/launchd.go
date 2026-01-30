package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ServiceStatus string

const (
	StatusRunning ServiceStatus = "running"
	StatusStopped ServiceStatus = "stopped"
	StatusError   ServiceStatus = "error"
	StatusUnknown ServiceStatus = "unknown"
)

type Service struct {
	Name         string
	Status       ServiceStatus
	Pid          int
	PlistPath    string
	Label        string
	LastExitCode int
}

type LaunchdManager struct {
	userAgentPaths   []string
	systemAgentPaths []string
	parser           *PlistParser
	runner           CommandRunner
}

func NewLaunchdManager() *LaunchdManager {
	homeDir, _ := os.UserHomeDir()

	return &LaunchdManager{
		userAgentPaths: []string{
			filepath.Join(homeDir, "Library", "LaunchAgents"),
		},
		systemAgentPaths: []string{
			"/Library/LaunchAgents",
			"/Library/LaunchDaemons",
		},
		parser: NewPlistParser(),
		runner: &DefaultCommandRunner{},
	}
}

func NewLaunchdManagerWithRunner(runner CommandRunner) *LaunchdManager {
	mgr := NewLaunchdManager()
	mgr.runner = runner
	return mgr
}

func (m *LaunchdManager) ListServices() ([]Service, error) {
	var services []Service

	plistPaths, err := m.findPlistFiles()
	if err != nil {
		return nil, err
	}

	launchctlOutput, err := m.getLaunchctlList()
	if err != nil {
		return nil, err
	}

	for _, path := range plistPaths {
		service := m.parseServiceFromPlist(path, launchctlOutput)
		services = append(services, service)
	}

	return services, nil
}

func (m *LaunchdManager) GetStatus(serviceName string) (Service, error) {
	plistPath := m.findPlistPath(serviceName)
	if plistPath == "" {
		return Service{}, ServiceNotFoundError{Name: serviceName}
	}

	launchctlOutput, err := m.getLaunchctlList()
	if err != nil {
		return Service{}, err
	}

	service := m.parseServiceFromPlist(plistPath, launchctlOutput)
	return service, nil
}

func (m *LaunchdManager) findPlistFiles() ([]string, error) {
	var paths []string

	for _, dir := range m.userAgentPaths {
		files, err := m.scanPlistDirectory(dir)
		if err != nil {
			continue
		}
		paths = append(paths, files...)
	}

	return paths, nil
}

func (m *LaunchdManager) scanPlistDirectory(dir string) ([]string, error) {
	var paths []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return nil, UserAgentPathError{Path: dir, Cause: err}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".plist") {
			continue
		}

		paths = append(paths, filepath.Join(dir, name))
	}

	return paths, nil
}

func (m *LaunchdManager) findPlistPath(serviceName string) string {
	for _, dir := range m.userAgentPaths {
		path := filepath.Join(dir, serviceName+".plist")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	for _, dir := range m.systemAgentPaths {
		path := filepath.Join(dir, serviceName+".plist")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func (m *LaunchdManager) getLaunchctlList() (map[string]launchctlEntry, error) {
	output, err := m.runner.Run("launchctl", "list")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, LaunchctlError{
				Command: "list",
				Cause:   err,
				Output:  string(exitErr.Stderr),
			}
		}
		return nil, LaunchctlError{Command: "list", Cause: err}
	}

	return m.parseLaunchctlOutput(output), nil
}

type launchctlEntry struct {
	Pid      int
	LastExit int
	Label    string
}

func (m *LaunchdManager) parseLaunchctlOutput(output []byte) map[string]launchctlEntry {
	entries := make(map[string]launchctlEntry)

	lines := strings.Split(string(output), "\n")

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid := -1
		if fields[0] != "-" {
			if p, err := strconv.Atoi(fields[0]); err == nil {
				pid = p
			}
		}

		lastExit := 0
		if fields[1] != "-" {
			if e, err := strconv.Atoi(fields[1]); err == nil {
				lastExit = e
			}
		}

		label := fields[2]

		entries[label] = launchctlEntry{
			Pid:      pid,
			LastExit: lastExit,
			Label:    label,
		}
	}

	return entries
}

func (m *LaunchdManager) parseServiceFromPlist(plistPath string, launchctlData map[string]launchctlEntry) Service {
	name := GetServiceNameFromPath(plistPath)

	info, err := m.parser.ParseFile(plistPath)
	if err != nil {
		return Service{
			Name:      name,
			Status:    StatusError,
			PlistPath: plistPath,
		}
	}

	label := info.Label
	if label == "" {
		label = name
	}

	entry, exists := launchctlData[label]

	service := Service{
		Name:      name,
		Label:     label,
		PlistPath: plistPath,
	}

	if !exists {
		service.Status = StatusStopped
	} else if entry.Pid > 0 {
		service.Status = StatusRunning
		service.Pid = entry.Pid
	} else if entry.LastExit != 0 {
		service.Status = StatusError
		service.LastExitCode = entry.LastExit
	} else {
		service.Status = StatusStopped
	}

	return service
}

func (m *LaunchdManager) IsUserService(plistPath string) bool {
	homeDir, _ := os.UserHomeDir()
	return strings.HasPrefix(plistPath, homeDir)
}

func (m *LaunchdManager) IsSystemService(plistPath string) bool {
	return strings.HasPrefix(plistPath, "/Library/LaunchDaemons")
}
