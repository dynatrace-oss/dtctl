//go:build !linux

package config

import (
	"context"
	"fmt"
	"runtime"
)

// EnsureKeyringCollection is only supported on Linux where D-Bus and
// Secret Service are available.
func EnsureKeyringCollection(_ context.Context) error {
	return fmt.Errorf("automatic keyring collection creation is only supported on Linux (current OS: %s)", runtime.GOOS)
}
