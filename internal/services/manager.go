package services

type ServiceManager interface {
	ListServices() ([]Service, error)
	GetStatus(name string) (Service, error)
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
}
