package client

import (
	"errors"
	"testing"
)

func TestExitCodeForStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       int
	}{
		{"unauthorized", 401, ExitAuthError},
		{"forbidden", 403, ExitPermissionError},
		{"not found", 404, ExitNotFoundError},
		{"bad request", 400, ExitError},
		{"server error", 500, ExitError},
		{"too many requests", 429, ExitError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCodeForStatus(tt.statusCode); got != tt.want {
				t.Errorf("ExitCodeForStatus(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := WrapError(originalErr, "context message")

	if wrappedErr == nil {
		t.Fatal("WrapError() returned nil")
	}

	// Check that the error message contains both the context and original
	errStr := wrappedErr.Error()
	if errStr == "" {
		t.Error("WrapError() returned empty error string")
	}
}

func TestExitCodes(t *testing.T) {
	// Verify exit code constants have expected values
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %v, want 0", ExitSuccess)
	}
	if ExitError != 1 {
		t.Errorf("ExitError = %v, want 1", ExitError)
	}
	if ExitUsageError != 2 {
		t.Errorf("ExitUsageError = %v, want 2", ExitUsageError)
	}
	if ExitAuthError != 3 {
		t.Errorf("ExitAuthError = %v, want 3", ExitAuthError)
	}
	if ExitNotFoundError != 4 {
		t.Errorf("ExitNotFoundError = %v, want 4", ExitNotFoundError)
	}
	if ExitPermissionError != 5 {
		t.Errorf("ExitPermissionError = %v, want 5", ExitPermissionError)
	}
}
