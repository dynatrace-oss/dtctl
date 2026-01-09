package client

import (
	"fmt"

	"github.com/cockroachdb/errors"
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

// APIError represents an error from the Dynatrace API
type APIError struct {
	StatusCode int
	Message    string
	Details    string
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("API error (%d): %s - %s", e.StatusCode, e.Message, e.Details)
	}
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Message)
}

// ExitCode returns the appropriate exit code for the error
func (e *APIError) ExitCode() int {
	switch e.StatusCode {
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

// NewAPIError creates a new API error
func NewAPIError(statusCode int, message, details string) error {
	return &APIError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
	}
}

// WrapError wraps an error with additional context
func WrapError(err error, message string) error {
	return errors.Wrap(err, message)
}
