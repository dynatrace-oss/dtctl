// Package credstore provides secure credential storage using the OS keyring
// with a file-based fallback for headless environments.
//
// The Store interface abstracts the storage backend so callers can switch
// between keyring and file storage, or provide a custom implementation
// for testing.
//
// Deprecated: dtctl's credential resolution lives in
// github.com/dynatrace-oss/dtctl/sdk/session (TokenStore, OAuthFileStore),
// which is the implementation dtctl actually runs and the one the
// config contract (docs/dev/CONFIG_CONTRACT.md) is tested against. This
// package predates that promotion, was never wired up, and will be removed
// before the sdk's first tagged release. The on-disk formats are compatible
// (same sanitization, same JSON-file layout).
package credstore
