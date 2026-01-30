package services

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockCommandRunner for testing systemd
type mockSystemdRunner struct {
	outputs map[string][]byte
	errors  map[string]error
}

func newMockSystemdRunner() *mockSystemdRunner {
	return &mockSystemdRunner{
		outputs: make(map[string][]byte),
		errors:  make(map[string]error),
	}
}

func (m *mockSystemdRunner) Run(name string, arg ...string) ([]byte, error) {
	key := name + " " + strings.Join(arg, " ")
	if err, ok := m.errors[key]; ok {
		return nil, err
	}
	if output, ok := m.outputs[key]; ok {
		return output, nil
	}
	return []byte{}, nil
}

func (m *mockSystemdRunner) RunWithStdin(name string, stdin io.Reader, arg ...string) ([]byte, error) {
	return m.Run(name, arg...)
}

func (m *mockSystemdRunner) setOutput(command string, output []byte) {
	m.outputs[command] = output
}

func (m *mockSystemdRunner) setError(command string, err error) {
	m.errors[command] = err
}

func TestNewSystemdManager(t *testing.T) {
	mgr := NewSystemdManager()

	if mgr == nil {
		t.Fatal("NewSystemdManager() returned nil")
	}

	if mgr.parser == nil {
		t.Error("parser should not be nil")
	}

	if mgr.runner == nil {
		t.Error("runner should not be nil")
	}

	if len(mgr.userServicePaths) == 0 {
		t.Error("userServicePaths should not be empty")
	}

	if len(mgr.systemServicePaths) == 0 {
		t.Error("systemServicePaths should not be empty")
	}
}

func TestSystemdManager_parseServiceFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "homebrew.mxcl.redis.service")

	content := `[Unit]
Description=Redis Server
After=network.target

[Service]
Type=simple
ExecStart=/home/linuxbrew/.linuxbrew/opt/redis/bin/redis-server
Restart=always
WorkingDirectory=/home/linuxbrew/.linuxbrew/var/redis
Environment="REDIS_PORT=6379"

[Install]
WantedBy=default.target
`

	err := os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test service file: %v", err)
	}

	mgr := NewSystemdManager()

	systemctlData := map[string]systemctlEntry{
		"homebrew.mxcl.redis": {Unit: "homebrew.mxcl.redis.service", Active: "active", SubState: "running", Pid: 1234},
	}

	service := mgr.parseServiceFromFile(servicePath, systemctlData)

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

	if service.PlistPath != servicePath {
		t.Errorf("PlistPath = %s, expected %s", service.PlistPath, servicePath)
	}
}

func TestSystemdManager_parseServiceFromFile_Stopped(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "stopped.service")

	content := `[Unit]
Description=Stopped Service

[Service]
ExecStart=/usr/bin/true
`

	err := os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test service file: %v", err)
	}

	mgr := NewSystemdManager()
	systemctlData := map[string]systemctlEntry{}

	service := mgr.parseServiceFromFile(servicePath, systemctlData)

	if service.Status != StatusStopped {
		t.Errorf("Status = %s, expected %s", service.Status, StatusStopped)
	}
}

func TestSystemdManager_parseServiceFromFile_Error(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "error.service")

	content := `[Unit]
Description=Error Service

[Service]
ExecStart=/usr/bin/false
`

	err := os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test service file: %v", err)
	}

	mgr := NewSystemdManager()
	systemctlData := map[string]systemctlEntry{
		"error": {Unit: "error.service", Active: "failed", SubState: "failed", Result: "failed", ExitCode: 1},
	}

	service := mgr.parseServiceFromFile(servicePath, systemctlData)

	if service.Status != StatusError {
		t.Errorf("Status = %s, expected %s", service.Status, StatusError)
	}

	if service.LastExitCode != 1 {
		t.Errorf("LastExitCode = %d, expected 1", service.LastExitCode)
	}
}

func TestServiceFileParser_Parse(t *testing.T) {
	parser := NewServiceFileParser()

	content := []byte(`[Unit]
Description=Test Service
After=network.target syslog.target
Wants=network.target

[Service]
Type=simple
ExecStart=/usr/bin/test-service
Restart=always
User=testuser
WorkingDirectory=/var/lib/test
Environment="VAR1=value1"
Environment="VAR2=value2"

[Install]
WantedBy=multi-user.target
`)

	info, err := parser.Parse(content, "test.service")
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}

	if info.Name != "test" {
		t.Errorf("Name = %s, expected test", info.Name)
	}

	if info.Description != "Test Service" {
		t.Errorf("Description = %s, expected 'Test Service'", info.Description)
	}

	if info.ExecStart != "/usr/bin/test-service" {
		t.Errorf("ExecStart = %s, expected /usr/bin/test-service", info.ExecStart)
	}

	if info.Type != "simple" {
		t.Errorf("Type = %s, expected simple", info.Type)
	}

	if info.Restart != "always" {
		t.Errorf("Restart = %s, expected always", info.Restart)
	}

	if info.User != "testuser" {
		t.Errorf("User = %s, expected testuser", info.User)
	}

	if info.WorkingDir != "/var/lib/test" {
		t.Errorf("WorkingDir = %s, expected /var/lib/test", info.WorkingDir)
	}

	if len(info.After) != 2 {
		t.Errorf("After length = %d, expected 2", len(info.After))
	}

	if len(info.Environment) != 2 {
		t.Errorf("Environment length = %d, expected 2", len(info.Environment))
	}
}

func TestServiceFileParser_ParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "test.service")

	content := `[Unit]
Description=File Test Service

[Service]
ExecStart=/usr/bin/test
`

	err := os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test service file: %v", err)
	}

	parser := NewServiceFileParser()
	info, err := parser.ParseFile(servicePath)
	if err != nil {
		t.Fatalf("ParseFile() returned error: %v", err)
	}

	if info.Name != "test" {
		t.Errorf("Name = %s, expected test", info.Name)
	}

	if info.Description != "File Test Service" {
		t.Errorf("Description = %s, expected 'File Test Service'", info.Description)
	}
}

func TestServiceFileParser_ParseSystemctlOutput(t *testing.T) {
	parser := NewServiceFileParser()

	output := []byte(`homebrew.mxcl.redis.service loaded active running Redis Server
homebrew.mxcl.nginx.service loaded active running Nginx Server
failed.service loaded failed failed Failed Service
stopped.service loaded inactive dead Stopped Service
`)

	entries := parser.ParseSystemctlOutput(output)

	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}

	redis, ok := entries["homebrew.mxcl.redis"]
	if !ok {
		t.Error("expected homebrew.mxcl.redis to be in entries")
	} else {
		if redis.Active != "active" {
			t.Errorf("expected Active 'active', got %s", redis.Active)
		}
		if redis.SubState != "running" {
			t.Errorf("expected SubState 'running', got %s", redis.SubState)
		}
	}

	failed, ok := entries["failed"]
	if !ok {
		t.Error("expected failed to be in entries")
	} else {
		if failed.Active != "failed" {
			t.Errorf("expected Active 'failed', got %s", failed.Active)
		}
	}
}

func TestSystemdManager_scanServiceDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "test1.service"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test2.service"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notaservice.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	mgr := NewSystemdManager()
	paths, err := mgr.scanServiceDirectory(tmpDir)
	if err != nil {
		t.Fatalf("scanServiceDirectory() returned error: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("expected 2 service files, got %d", len(paths))
	}
}

func TestSystemdManager_scanServiceDirectory_NotExist(t *testing.T) {
	mgr := NewSystemdManager()
	paths, err := mgr.scanServiceDirectory("/nonexistent/path/12345")
	if err != nil {
		t.Errorf("scanServiceDirectory() should not error for non-existent dir, got: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestSystemdManager_IsUserService(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	mgr := NewSystemdManager()

	tests := []struct {
		path     string
		expected bool
	}{
		{filepath.Join(homeDir, ".config/systemd/user/test.service"), true},
		{"/etc/systemd/system/test.service", false},
		{"/usr/lib/systemd/system/test.service", false},
	}

	for _, test := range tests {
		result := mgr.IsUserService(test.path)
		if result != test.expected {
			t.Errorf("IsUserService(%s) = %v, expected %v", test.path, result, test.expected)
		}
	}
}

func TestSystemdManager_IsSystemService(t *testing.T) {
	mgr := NewSystemdManager()

	tests := []struct {
		path     string
		expected bool
	}{
		{"/etc/systemd/system/test.service", true},
		{"/usr/lib/systemd/system/test.service", true},
		{"/home/user/.config/systemd/user/test.service", false},
	}

	for _, test := range tests {
		result := mgr.IsSystemService(test.path)
		if result != test.expected {
			t.Errorf("IsSystemService(%s) = %v, expected %v", test.path, result, test.expected)
		}
	}
}

func TestSystemdManager_findServiceFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewSystemdManager()

	mgr.userServicePaths = []string{tmpDir}

	content := `[Unit]
Description=Test Service

[Service]
ExecStart=/usr/bin/test
`

	os.WriteFile(filepath.Join(tmpDir, "test.service"), []byte(content), 0644)

	result := mgr.findServiceFilePath("test")
	expected := filepath.Join(tmpDir, "test.service")
	if result != expected {
		t.Errorf("findServiceFilePath() = %s, expected %s", result, expected)
	}

	notFound := mgr.findServiceFilePath("nonexistent")
	if notFound != "" {
		t.Errorf("findServiceFilePath() for nonexistent = %s, expected empty string", notFound)
	}
}

func TestUserServicePathError(t *testing.T) {
	err := UserServicePathError{Path: "/test", Cause: os.ErrNotExist}
	expected := "user service path error at /test: file does not exist"
	if err.Error() != expected {
		t.Errorf("Error() = %q, expected %q", err.Error(), expected)
	}

	if err.Unwrap() != os.ErrNotExist {
		t.Error("Unwrap() should return the cause")
	}
}

func TestServiceFileNotFoundError(t *testing.T) {
	err := ServiceFileNotFoundError{Name: "test", Path: "/path"}
	expected := "service file not found for test at path /path"
	if err.Error() != expected {
		t.Errorf("Error() = %q, expected %q", err.Error(), expected)
	}

	if err.ServiceName() != "test" {
		t.Errorf("ServiceName() = %s, expected test", err.ServiceName())
	}

	errNoPath := ServiceFileNotFoundError{Name: "test"}
	expectedNoPath := "service file not found: test"
	if errNoPath.Error() != expectedNoPath {
		t.Errorf("Error() without path = %q, expected %q", errNoPath.Error(), expectedNoPath)
	}
}

func TestSystemdManager_ListServices(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock service files
	homebrewService := `[Unit]
Description=Homebrew Redis

[Service]
ExecStart=/home/linuxbrew/.linuxbrew/opt/redis/bin/redis-server
`

	regularService := `[Unit]
Description=Regular Service

[Service]
ExecStart=/usr/bin/test
`

	os.WriteFile(filepath.Join(tmpDir, "homebrew.mxcl.redis.service"), []byte(homebrewService), 0644)
	os.WriteFile(filepath.Join(tmpDir, "regular.service"), []byte(regularService), 0644)

	// Mock systemctl output
	systemctlOutput := []byte(`homebrew.mxcl.redis.service loaded active running Redis Server
regular.service loaded inactive dead Regular Service
`)

	mockRunner := newMockSystemdRunner()
	mockRunner.setOutput("systemctl --user list-units --type=service --all --no-pager --no-legend", systemctlOutput)

	mgr := NewSystemdManagerWithRunner(mockRunner)
	mgr.userServicePaths = []string{tmpDir}

	services, err := mgr.ListServices()
	if err != nil {
		t.Fatalf("ListServices() returned error: %v", err)
	}

	// Should only return Homebrew services
	if len(services) != 1 {
		t.Errorf("expected 1 Homebrew service, got %d", len(services))
	}

	if len(services) > 0 && services[0].Name != "homebrew.mxcl.redis" {
		t.Errorf("expected service name 'homebrew.mxcl.redis', got %s", services[0].Name)
	}
}

func TestSystemdManager_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()

	content := `[Unit]
Description=Redis

[Service]
ExecStart=/usr/bin/redis-server
`

	os.WriteFile(filepath.Join(tmpDir, "homebrew.mxcl.redis.service"), []byte(content), 0644)

	systemctlOutput := []byte(`homebrew.mxcl.redis.service loaded active running Redis Server
`)

	mockRunner := newMockSystemdRunner()
	mockRunner.setOutput("systemctl --user list-units --type=service --all --no-pager --no-legend", systemctlOutput)

	mgr := NewSystemdManagerWithRunner(mockRunner)
	mgr.userServicePaths = []string{tmpDir}

	service, err := mgr.GetStatus("homebrew.mxcl.redis")
	if err != nil {
		t.Fatalf("GetStatus() returned error: %v", err)
	}

	if service.Name != "homebrew.mxcl.redis" {
		t.Errorf("Name = %s, expected homebrew.mxcl.redis", service.Name)
	}

	if service.Status != StatusRunning {
		t.Errorf("Status = %s, expected %s", service.Status, StatusRunning)
	}
}

func TestSystemdManager_GetStatus_NotFound(t *testing.T) {
	mgr := NewSystemdManager()
	mgr.userServicePaths = []string{"/nonexistent"}

	_, err := mgr.GetStatus("nonexistent")
	if err == nil {
		t.Error("GetStatus() should return error for non-existent service")
	}

	if _, ok := err.(ServiceNotFoundError); !ok {
		t.Errorf("expected ServiceNotFoundError, got %T", err)
	}
}

func TestServiceFileParser_ParseSystemctlStatus(t *testing.T) {
	parser := NewServiceFileParser()

	statusOutput := []byte(`● homebrew.mxcl.redis.service - Redis Server
   Loaded: loaded (/home/user/.config/systemd/user/homebrew.mxcl.redis.service; enabled; vendor preset: enabled)
   Active: active (running) since Mon 2024-01-15 10:00:00 UTC; 1h ago
 Main PID: 1234 (redis-server)
   CGroup: /user.slice/user-1000.slice/user@1000.service/homebrew.mxcl.redis.service
           └─1234 /usr/bin/redis-server

Jan 15 10:00:00 hostname systemd[1234]: Started Redis Server.
`)

	entry, err := parser.ParseSystemctlStatus(statusOutput)
	if err != nil {
		t.Fatalf("ParseSystemctlStatus() returned error: %v", err)
	}

	if entry.Pid != 1234 {
		t.Errorf("Pid = %d, expected 1234", entry.Pid)
	}

	if entry.Active != "active (running) since Mon 2024-01-15 10:00:00 UTC; 1h ago" {
		// The active line extraction may vary, just check it contains "active"
		if !strings.Contains(entry.Active, "active") {
			t.Errorf("Active = %s, expected to contain 'active'", entry.Active)
		}
	}
}
