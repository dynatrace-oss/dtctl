package client

import (
	"fmt"
)

// Error codes matching the CLI error handling spec
const (
	ExitSuccess         = 0
	ExitError           = 1
	ExitUsageError      = 2
	ExitAuthError       = 3
	ExitNotFoundError   = 4
	ExitPermissionError = 5
)

// ExitCodeForStatus returns the exit code for an HTTP error status.
func ExitCodeForStatus(statusCode int) int {
	switch statusCode {
	case 401:
		return ExitAuthError
	case 403:
		return ExitPermissionError
	case 404:
		return ExitNotFoundError
	default:
		return ExitError
	}
}

// WrapError wraps an error with additional context
func WrapError(err error, message string) error {
	return fmt.Errorf("%s: %w", message, err)
}
