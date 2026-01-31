//go:build darwin

package services

func NewServiceManager() ServiceManager {
	return NewLaunchdManager()
}
