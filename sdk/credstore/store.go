package credstore

// Store is the interface for secure credential storage.
type Store interface {
	// Get retrieves a credential by name.
	Get(name string) (string, error)
	// Set stores a credential.
	Set(name, value string) error
	// Delete removes a credential.
	Delete(name string) error
}
