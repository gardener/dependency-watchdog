package types

// Manager is the convenience interface to manage lifecycle of probers.
type Manager interface {
	// Register registers the given prober with the manager. It should return false if prober is already registered.
	Register(prober Prober) bool
	// Unregister closes the prober and removes it from the manager. It should return false if prober is not registered with the manager.
	Unregister(key string) bool
	// GetProber uses the given key to get a registered prober from the manager. It returns false if prober is not found.
	GetProber(key string) (Prober, bool)
	// GetAllProbers returns a slice of all the probers registered with the manager.
	GetAllProbers() []Prober
}
