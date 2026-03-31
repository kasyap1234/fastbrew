//go:build linux

package services

import (
	"os"
	"path/filepath"
)

func NewServiceManager() ServiceManager {
	return NewSystemdManagerWithScope(ScopeAll)
}

func newUserScopeManager() ServiceManager {
	mgr := NewSystemdManagerWithScope(ScopeUser)
	homeDir, _ := os.UserHomeDir()
	mgr.userServicePaths = []string{filepath.Join(homeDir, ".config", "systemd", "user")}
	mgr.systemServicePaths = []string{}
	return mgr
}

func newSystemScopeManager() ServiceManager {
	mgr := NewSystemdManagerWithScope(ScopeSystem)
	mgr.userServicePaths = []string{}
	mgr.systemServicePaths = []string{
		"/etc/systemd/system",
		"/usr/lib/systemd/system",
	}
	return mgr
}

func newAllScopeManager() ServiceManager {
	return NewSystemdManagerWithScope(ScopeAll)
}
