// Package credstore provides secure credential storage using the OS keyring
// with a file-based fallback for headless environments.
//
// The Store interface abstracts the storage backend so callers can switch
// between keyring and file storage, or provide a custom implementation
// for testing.
package credstore
