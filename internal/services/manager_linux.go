//go:build linux

package services

func NewServiceManager() ServiceManager {
	return NewSystemdManager()
}
