package interfaces

// Registrar registers and deregisters a service instance with a service
// registry (e.g. Consul).  Call Register on startup and Deregister on shutdown.
type Registrar interface {
	Register()
	Deregister()
}
