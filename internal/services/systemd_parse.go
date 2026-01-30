package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// systemctlEntry represents a parsed entry from systemctl list-units
type systemctlEntry struct {
	Unit        string
	Load        string
	Active      string
	SubState    string
	Description string
	Pid         int
	Result      string
	ExitCode    int
}

// ServiceFile represents a parsed systemd service file
type ServiceFile struct {
	Name        string
	Description string
	ExecStart   string
	Type        string
	Restart     string
	User        string
	WorkingDir  string
	Environment map[string]string
	After       []string
	Wants       []string
}

// ServiceFileParser parses systemd service files and systemctl output
type ServiceFileParser struct{}

// NewServiceFileParser creates a new ServiceFileParser
func NewServiceFileParser() *ServiceFileParser {
	return &ServiceFileParser{}
}

// ParseFile parses a systemd service file
func (p *ServiceFileParser) ParseFile(path string) (*ServiceFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ServiceFileNotFoundError{Path: path, Name: filepath.Base(path)}
		}
		return nil, fmt.Errorf("failed to read service file: %w", err)
	}

	return p.Parse(data, path)
}

// Parse parses systemd service file content
func (p *ServiceFileParser) Parse(data []byte, sourcePath string) (*ServiceFile, error) {
	content := string(data)

	info := &ServiceFile{
		Name:        filepath.Base(sourcePath),
		Environment: make(map[string]string),
		After:       []string{},
		Wants:       []string{},
	}

	// Remove .service suffix from name
	if strings.HasSuffix(info.Name, ".service") {
		info.Name = strings.TrimSuffix(info.Name, ".service")
	}

	// Parse [Unit] section
	info.Description = p.extractValue(content, "Description")
	info.After = p.extractList(content, "After")
	info.Wants = p.extractList(content, "Wants")

	// Parse [Service] section
	info.ExecStart = p.extractValue(content, "ExecStart")
	info.Type = p.extractValue(content, "Type")
	info.Restart = p.extractValue(content, "Restart")
	info.User = p.extractValue(content, "User")
	info.WorkingDir = p.extractValue(content, "WorkingDirectory")

	// Parse environment variables
	envVars := p.extractAllMatching(content, `Environment="([^"]+)"`)
	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			info.Environment[parts[0]] = parts[1]
		}
	}

	return info, nil
}

// ParseSystemctlOutput parses the output of systemctl list-units
func (p *ServiceFileParser) ParseSystemctlOutput(output []byte) map[string]systemctlEntry {
	entries := make(map[string]systemctlEntry)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse the line
		// Format: UNIT LOAD ACTIVE SUB DESCRIPTION
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		unit := fields[0]
		// Remove .service suffix for matching
		unitName := strings.TrimSuffix(unit, ".service")

		entry := systemctlEntry{
			Unit:        unit,
			Load:        fields[1],
			Active:      fields[2],
			SubState:    fields[3],
			Description: strings.Join(fields[4:], " "),
		}

		entries[unitName] = entry
	}

	return entries
}

// ParseSystemctlStatus parses the output of systemctl status command
func (p *ServiceFileParser) ParseSystemctlStatus(output []byte) (systemctlEntry, error) {
	entry := systemctlEntry{}
	content := string(output)

	// Extract PID
	if pidStr := p.extractLineValue(content, "Main PID:"); pidStr != "" {
		// Format: "Main PID: 1234 (process)" or just "Main PID: 1234"
		fields := strings.Fields(pidStr)
		if len(fields) > 0 {
			if pid, err := strconv.Atoi(fields[0]); err == nil {
				entry.Pid = pid
			}
		}
	}

	// Extract Active status
	entry.Active = p.extractLineValue(content, "Active:")

	// Extract exit code if present
	if strings.Contains(content, "code=exited,") {
		parts := strings.Split(content, "code=exited,")
		if len(parts) > 1 {
			statusPart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(statusPart, "status=") {
				statusStr := strings.TrimPrefix(statusPart, "status=")
				statusStr = strings.Fields(statusStr)[0] // Get first field only
				if exitCode, err := strconv.Atoi(statusStr); err == nil {
					entry.ExitCode = exitCode
				}
			}
		}
	}

	return entry, nil
}

// extractValue extracts a value from a key=value pair in the service file
func (p *ServiceFileParser) extractValue(content, key string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			value := strings.TrimPrefix(line, key+"=")
			// Remove quotes if present
			value = strings.Trim(value, `"`)
			return value
		}
	}
	return ""
}

// extractList extracts a comma-separated list value
func (p *ServiceFileParser) extractList(content, key string) []string {
	value := p.extractValue(content, key)
	if value == "" {
		return []string{}
	}
	// Handle both comma-separated and space-separated values
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Split space-separated values (e.g., "network.target syslog.target")
		subParts := strings.Fields(part)
		for _, sub := range subParts {
			sub = strings.TrimSpace(sub)
			if sub != "" {
				result = append(result, sub)
			}
		}
	}
	return result
}

// extractAllMatching extracts all values matching a pattern
func (p *ServiceFileParser) extractAllMatching(content, pattern string) []string {
	var results []string
	// Simple pattern matching - look for Environment="key=value"
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Environment=") {
			value := strings.TrimPrefix(line, "Environment=")
			value = strings.Trim(value, `"`)
			results = append(results, value)
		}
	}
	return results
}

// extractLineValue extracts a value from a line starting with a prefix
func (p *ServiceFileParser) extractLineValue(content, prefix string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// ServiceFileNotFoundError indicates a service file was not found
type ServiceFileNotFoundError struct {
	Name string
	Path string
}

func (e ServiceFileNotFoundError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("service file not found for %s at path %s", e.Name, e.Path)
	}
	return fmt.Sprintf("service file not found: %s", e.Name)
}

func (e ServiceFileNotFoundError) ServiceName() string {
	return e.Name
}
