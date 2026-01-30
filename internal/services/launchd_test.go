package services

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockCommandRunner struct {
	outputs map[string][]byte
	errors  map[string]error
}

func newMockCommandRunner() *mockCommandRunner {
	return &mockCommandRunner{
		outputs: make(map[string][]byte),
		errors:  make(map[string]error),
	}
}

func (m *mockCommandRunner) Run(name string, arg ...string) ([]byte, error) {
	key := name + " " + strings.Join(arg, " ")
	if err, ok := m.errors[key]; ok {
		return nil, err
	}
	if output, ok := m.outputs[key]; ok {
		return output, nil
	}
	return []byte{}, nil
}

func (m *mockCommandRunner) RunWithStdin(name string, stdin io.Reader, arg ...string) ([]byte, error) {
	return m.Run(name, arg...)
}

func (m *mockCommandRunner) setOutput(command string, output []byte) {
	m.outputs[command] = output
}

func (m *mockCommandRunner) setError(command string, err error) {
	m.errors[command] = err
}

func TestNewLaunchdManager(t *testing.T) {
	mgr := NewLaunchdManager()

	if mgr == nil {
		t.Fatal("NewLaunchdManager() returned nil")
	}

	if mgr.parser == nil {
		t.Error("parser should not be nil")
	}

	if mgr.runner == nil {
		t.Error("runner should not be nil")
	}

	if len(mgr.userAgentPaths) == 0 {
		t.Error("userAgentPaths should not be empty")
	}
}

func TestLaunchdManager_parseLaunchctlOutput(t *testing.T) {
	mgr := NewLaunchdManager()

	output := `PID	Status	Label
-	0	com.apple.test1
123	0	com.apple.test2
-	1	com.apple.test3
-	0	homebrew.mxcl.nginx
		
`

	entries := mgr.parseLaunchctlOutput([]byte(output))

	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}

	entry1, ok := entries["com.apple.test1"]
	if !ok {
		t.Error("expected com.apple.test1 to be in entries")
	} else {
		if entry1.Pid != -1 {
			t.Errorf("expected Pid -1, got %d", entry1.Pid)
		}
		if entry1.LastExit != 0 {
			t.Errorf("expected LastExit 0, got %d", entry1.LastExit)
		}
	}

	entry2, ok := entries["com.apple.test2"]
	if !ok {
		t.Error("expected com.apple.test2 to be in entries")
	} else {
		if entry2.Pid != 123 {
			t.Errorf("expected Pid 123, got %d", entry2.Pid)
		}
		if entry2.LastExit != 0 {
			t.Errorf("expected LastExit 0, got %d", entry2.LastExit)
		}
	}

	entry3, ok := entries["com.apple.test3"]
	if !ok {
		t.Error("expected com.apple.test3 to be in entries")
	} else {
		if entry3.LastExit != 1 {
			t.Errorf("expected LastExit 1, got %d", entry3.LastExit)
		}
	}
}

func TestGetServiceNameFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/Users/test/Library/LaunchAgents/homebrew.mxcl.nginx.plist", "homebrew.mxcl.nginx"},
		{"/Library/LaunchAgents/test.plist", "test"},
		{"test.plist", "test"},
		{"/path/to/service", "service"},
	}

	for _, test := range tests {
		result := GetServiceNameFromPath(test.path)
		if result != test.expected {
			t.Errorf("GetServiceNameFromPath(%s) = %s, expected %s", test.path, result, test.expected)
		}
	}
}

func TestIsHomebrewService(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"homebrew.mxcl.nginx", true},
		{"brew.services.redis", true},
		{"com.apple.test", false},
		{"random.service", false},
		{"HOMEBREW.test", true},
	}

	for _, test := range tests {
		result := IsHomebrewService(test.name)
		if result != test.expected {
			t.Errorf("IsHomebrewService(%s) = %v, expected %v", test.name, result, test.expected)
		}
	}
}

func TestPlistParser_Parse(t *testing.T) {
	parser := NewPlistParser()

	validPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>homebrew.mxcl.nginx</string>
	<key>Program</key>
	<string>/opt/homebrew/opt/nginx/bin/nginx</string>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>`

	info, err := parser.Parse([]byte(validPlist), "test.plist")
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}

	if info.Label != "homebrew.mxcl.nginx" {
		t.Errorf("Label = %s, expected homebrew.mxcl.nginx", info.Label)
	}

	if info.Program != "/opt/homebrew/opt/nginx/bin/nginx" {
		t.Errorf("Program = %s, expected /opt/homebrew/opt/nginx/bin/nginx", info.Program)
	}

	if !info.RunAtLoad {
		t.Error("RunAtLoad should be true")
	}
}

func TestPlistParser_Parse_MissingLabel(t *testing.T) {
	parser := NewPlistParser()

	invalidPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Program</key>
	<string>/test</string>
</dict>
</plist>`

	_, err := parser.Parse([]byte(invalidPlist), "test.plist")
	if err == nil {
		t.Error("Parse() should return error for plist without Label")
	}

	if _, ok := err.(InvalidPlistError); !ok {
		t.Errorf("expected InvalidPlistError, got %T", err)
	}
}

func TestPlistParser_ParseFile_NotFound(t *testing.T) {
	parser := NewPlistParser()

	_, err := parser.ParseFile("/nonexistent/path/test.plist")
	if err == nil {
		t.Error("ParseFile() should return error for non-existent file")
	}

	if _, ok := err.(PlistNotFoundError); !ok {
		t.Errorf("expected PlistNotFoundError, got %T", err)
	}
}

func TestPlistParser_ParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "test.plist")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>test.service</string>
	<key>Program</key>
	<string>/usr/bin/true</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test plist: %v", err)
	}

	parser := NewPlistParser()
	info, err := parser.ParseFile(plistPath)
	if err != nil {
		t.Fatalf("ParseFile() returned error: %v", err)
	}

	if info.Label != "test.service" {
		t.Errorf("Label = %s, expected test.service", info.Label)
	}
}

func TestLaunchdManager_parseServiceFromPlist(t *testing.T) {
	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "homebrew.mxcl.redis.plist")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>homebrew.mxcl.redis</string>
	<key>Program</key>
	<string>/opt/homebrew/opt/redis/bin/redis-server</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test plist: %v", err)
	}

	mgr := NewLaunchdManager()

	launchctlData := map[string]launchctlEntry{
		"homebrew.mxcl.redis": {Pid: 1234, LastExit: 0, Label: "homebrew.mxcl.redis"},
	}

	service := mgr.parseServiceFromPlist(plistPath, launchctlData)

	if service.Name != "homebrew.mxcl.redis" {
		t.Errorf("Name = %s, expected homebrew.mxcl.redis", service.Name)
	}

	if service.Label != "homebrew.mxcl.redis" {
		t.Errorf("Label = %s, expected homebrew.mxcl.redis", service.Label)
	}

	if service.Status != StatusRunning {
		t.Errorf("Status = %s, expected %s", service.Status, StatusRunning)
	}

	if service.Pid != 1234 {
		t.Errorf("Pid = %d, expected 1234", service.Pid)
	}

	if service.PlistPath != plistPath {
		t.Errorf("PlistPath = %s, expected %s", service.PlistPath, plistPath)
	}
}

func TestLaunchdManager_parseServiceFromPlist_Stopped(t *testing.T) {
	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "stopped.plist")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>stopped.service</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test plist: %v", err)
	}

	mgr := NewLaunchdManager()
	launchctlData := map[string]launchctlEntry{}

	service := mgr.parseServiceFromPlist(plistPath, launchctlData)

	if service.Status != StatusStopped {
		t.Errorf("Status = %s, expected %s", service.Status, StatusStopped)
	}
}

func TestLaunchdManager_parseServiceFromPlist_Error(t *testing.T) {
	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "error.plist")

	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>error.service</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test plist: %v", err)
	}

	mgr := NewLaunchdManager()
	launchctlData := map[string]launchctlEntry{
		"error.service": {Pid: -1, LastExit: 1, Label: "error.service"},
	}

	service := mgr.parseServiceFromPlist(plistPath, launchctlData)

	if service.Status != StatusError {
		t.Errorf("Status = %s, expected %s", service.Status, StatusError)
	}

	if service.LastExitCode != 1 {
		t.Errorf("LastExitCode = %d, expected 1", service.LastExitCode)
	}
}

func TestServiceErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ServiceNotFoundError",
			err:      ServiceNotFoundError{Name: "test"},
			expected: "service not found: test",
		},
		{
			name:     "PlistNotFoundError with path",
			err:      PlistNotFoundError{Name: "test", Path: "/path"},
			expected: "plist not found for service test at path /path",
		},
		{
			name:     "PlistNotFoundError without path",
			err:      PlistNotFoundError{Name: "test"},
			expected: "plist not found for service: test",
		},
		{
			name:     "UserAgentPathError",
			err:      UserAgentPathError{Path: "/test", Cause: os.ErrNotExist},
			expected: "user agent path error at /test: file does not exist",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err.Error() != test.expected {
				t.Errorf("Error() = %q, expected %q", test.err.Error(), test.expected)
			}
		})
	}
}

func TestInvalidPlistError(t *testing.T) {
	cause := os.ErrNotExist
	err := InvalidPlistError{Name: "test", Path: "/path", Cause: cause}

	expected := "invalid plist for service test at /path: file does not exist"
	if err.Error() != expected {
		t.Errorf("Error() = %q, expected %q", err.Error(), expected)
	}

	if err.Unwrap() != cause {
		t.Error("Unwrap() should return the cause")
	}

	errNoPath := InvalidPlistError{Name: "test", Cause: cause}
	expectedNoPath := "invalid plist for service test: file does not exist"
	if errNoPath.Error() != expectedNoPath {
		t.Errorf("Error() without path = %q, expected %q", errNoPath.Error(), expectedNoPath)
	}
}

func TestLaunchctlError(t *testing.T) {
	tests := []struct {
		name     string
		err      LaunchctlError
		expected string
	}{
		{
			name:     "with output",
			err:      LaunchctlError{Command: "list", Cause: os.ErrNotExist, Output: "error msg"},
			expected: "launchctl list failed: file does not exist (output: error msg)",
		},
		{
			name:     "without output",
			err:      LaunchctlError{Command: "list", Cause: os.ErrNotExist},
			expected: "launchctl list failed: file does not exist",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err.Error() != test.expected {
				t.Errorf("Error() = %q, expected %q", test.err.Error(), test.expected)
			}
		})
	}
}

func TestLaunchdManager_scanPlistDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "test1.plist"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test2.plist"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notaplist.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	mgr := NewLaunchdManager()
	paths, err := mgr.scanPlistDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanPlistDirectory() returned error: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 plist files, got %d", len(paths))
	}
}

func TestLaunchdManager_scanPlistDirectory_NotExist(t *testing.T) {
	mgr := NewLaunchdManager()
	paths, err := mgr.scanPlistDirectory("/nonexistent/path/12345")
	if err != nil {
		t.Errorf("scanPlistDirectory() should not error for non-existent dir, got: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestLaunchdManager_IsUserService(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	mgr := NewLaunchdManager()

	tests := []struct {
		path     string
		expected bool
	}{
		{filepath.Join(homeDir, "Library/LaunchAgents/test.plist"), true},
		{"/Library/LaunchAgents/test.plist", false},
		{"/Library/LaunchDaemons/test.plist", false},
	}

	for _, test := range tests {
		result := mgr.IsUserService(test.path)
		if result != test.expected {
			t.Errorf("IsUserService(%s) = %v, expected %v", test.path, result, test.expected)
		}
	}
}

func TestLaunchdManager_IsSystemService(t *testing.T) {
	mgr := NewLaunchdManager()

	tests := []struct {
		path     string
		expected bool
	}{
		{"/Library/LaunchDaemons/test.plist", true},
		{"/Library/LaunchAgents/test.plist", false},
		{"/Users/test/Library/LaunchAgents/test.plist", false},
	}

	for _, test := range tests {
		result := mgr.IsSystemService(test.path)
		if result != test.expected {
			t.Errorf("IsSystemService(%s) = %v, expected %v", test.path, result, test.expected)
		}
	}
}

func TestServiceNotFoundError_ServiceName(t *testing.T) {
	err := ServiceNotFoundError{Name: "my-service"}
	if err.ServiceName() != "my-service" {
		t.Errorf("ServiceName() = %s, expected my-service", err.ServiceName())
	}
}

func TestPlistNotFoundError_ServiceName(t *testing.T) {
	err := PlistNotFoundError{Name: "my-service", Path: "/path"}
	if err.ServiceName() != "my-service" {
		t.Errorf("ServiceName() = %s, expected my-service", err.ServiceName())
	}
}

func TestInvalidPlistError_ServiceName(t *testing.T) {
	err := InvalidPlistError{Name: "my-service", Path: "/path", Cause: os.ErrNotExist}
	if err.ServiceName() != "my-service" {
		t.Errorf("ServiceName() = %s, expected my-service", err.ServiceName())
	}
}

func TestLaunchdManager_findPlistPath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewLaunchdManager()

	mgr.userAgentPaths = []string{tmpDir}

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>test</string>
</dict>
</plist>`

	os.WriteFile(filepath.Join(tmpDir, "test.plist"), []byte(plistContent), 0644)

	result := mgr.findPlistPath("test")
	expected := filepath.Join(tmpDir, "test.plist")
	if result != expected {
		t.Errorf("findPlistPath() = %s, expected %s", result, expected)
	}

	notFound := mgr.findPlistPath("nonexistent")
	if notFound != "" {
		t.Errorf("findPlistPath() for nonexistent = %s, expected empty string", notFound)
	}
}
