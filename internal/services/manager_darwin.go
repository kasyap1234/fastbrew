//go:build darwin

package services

import (
	"os"
	"path/filepath"
)

func NewServiceManager() ServiceManager {
	return NewLaunchdManagerWithScope(ScopeAll)
}

func newUserScopeManager() ServiceManager {
	mgr := NewLaunchdManagerWithScope(ScopeUser)
	homeDir, _ := getHomeDir()
	mgr.userAgentPaths = []string{filepath.Join(homeDir, "Library", "LaunchAgents")}
	mgr.systemAgentPaths = []string{}
	return mgr
}

func newSystemScopeManager() ServiceManager {
	mgr := NewLaunchdManagerWithScope(ScopeSystem)
	mgr.userAgentPaths = []string{}
	mgr.systemAgentPaths = []string{
		"/Library/LaunchDaemons",
	}
	return mgr
}

func newAllScopeManager() ServiceManager {
	return NewLaunchdManagerWithScope(ScopeAll)
}

func getHomeDir() (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return "", ErrInvalidScope
	}
	return homeDir, nil
}
