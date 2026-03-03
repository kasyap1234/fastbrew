//go:build windows

package services

import "errors"

type WindowsServiceManager struct{}

func NewServiceManager() ServiceManager {
	return &WindowsServiceManager{}
}

func newUserScopeManager() ServiceManager {
	return &WindowsServiceManager{}
}

func newSystemScopeManager() ServiceManager {
	return &WindowsServiceManager{}
}

func newAllScopeManager() ServiceManager {
	return &WindowsServiceManager{}
}

func (m *WindowsServiceManager) ListServices() ([]Service, error) {
	return nil, errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) GetStatus(name string) (Service, error) {
	return Service{}, errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) Start(name string) error {
	return errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) Stop(name string) error {
	return errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) Restart(name string) error {
	return errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) Enable(name string) error {
	return errors.New("services management not supported on Windows")
}

func (m *WindowsServiceManager) Disable(name string) error {
	return errors.New("services management not supported on Windows")
}
