package services

type ServiceScope string

const (
	ScopeUser   ServiceScope = "user"
	ScopeSystem ServiceScope = "system"
	ScopeAll    ServiceScope = "all"
)

type ServiceManager interface {
	ListServices() ([]Service, error)
	GetStatus(name string) (Service, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Enable(name string) error
	Disable(name string) error
}

func NewServiceManagerWithScope(scope ServiceScope) (ServiceManager, error) {
	switch scope {
	case ScopeUser:
		return newUserScopeManager(), nil
	case ScopeSystem:
		return newSystemScopeManager(), nil
	case ScopeAll:
		return newAllScopeManager(), nil
	default:
		return nil, ErrInvalidScope
	}
}
