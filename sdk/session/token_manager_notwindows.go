//go:build !windows

package session

// isWindowsAdminKeyringErr always returns false on non-Windows platforms.
// The Windows-specific check lives in token_manager_windows.go.
func isWindowsAdminKeyringErr(_ error) bool { return false }
