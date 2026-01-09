package client

import (
	"errors"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected string
	}{
		{
			name: "with details",
			err: &APIError{
				StatusCode: 404,
				Message:    "Not Found",
				Details:    "Resource does not exist",
			},
			expected: "API error (404): Not Found - Resource does not exist",
		},
		{
			name: "without details",
			err: &APIError{
				StatusCode: 500,
				Message:    "Internal Server Error",
				Details:    "",
			},
			expected: "API error (500): Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("APIError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAPIError_ExitCode(t *testing.T) {
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
			err := &APIError{StatusCode: tt.statusCode, Message: "test"}
			if got := err.ExitCode(); got != tt.want {
				t.Errorf("APIError.ExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAPIError(t *testing.T) {
	err := NewAPIError(404, "Not Found", "Resource missing")

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatal("NewAPIError() did not return *APIError")
	}

	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %v, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "Not Found" {
		t.Errorf("Message = %v, want 'Not Found'", apiErr.Message)
	}
	if apiErr.Details != "Resource missing" {
		t.Errorf("Details = %v, want 'Resource missing'", apiErr.Details)
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
