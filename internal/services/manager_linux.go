//go:build linux

package services

import (
	"os"
	"path/filepath"
)

func NewServiceManager() ServiceManager {
	return NewSystemdManager()
}

func newUserScopeManager() ServiceManager {
	mgr := NewSystemdManager()
	mgr.userServicePaths = []string{}
	homeDir, _ := os.UserHomeDir()
	mgr.userServicePaths = append(mgr.userServicePaths, filepath.Join(homeDir, ".config", "systemd", "user"))
	mgr.systemServicePaths = []string{}
	return mgr
}

func newSystemScopeManager() ServiceManager {
	mgr := NewSystemdManager()
	mgr.userServicePaths = []string{}
	mgr.systemServicePaths = []string{
		"/etc/systemd/system",
		"/usr/lib/systemd/system",
	}
	return mgr
}

func newAllScopeManager() ServiceManager {
	return NewSystemdManager()
}
