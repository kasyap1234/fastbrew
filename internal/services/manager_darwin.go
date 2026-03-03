//go:build darwin

package services

import (
	"os"
	"path/filepath"
)

func NewServiceManager() ServiceManager {
	return NewLaunchdManager()
}

func newUserScopeManager() ServiceManager {
	mgr := NewLaunchdManager()
	mgr.userAgentPaths = []string{}
	homeDir, _ := getHomeDir()
	mgr.userAgentPaths = append(mgr.userAgentPaths, filepath.Join(homeDir, "Library", "LaunchAgents"))
	mgr.systemAgentPaths = []string{}
	return mgr
}

func newSystemScopeManager() ServiceManager {
	mgr := NewLaunchdManager()
	mgr.userAgentPaths = []string{}
	mgr.systemAgentPaths = []string{
		"/Library/LaunchDaemons",
	}
	return mgr
}

func newAllScopeManager() ServiceManager {
	return NewLaunchdManager()
}

func getHomeDir() (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return "", ErrInvalidScope
	}
	return homeDir, nil
}
