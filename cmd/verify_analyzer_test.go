package cmd

import (
	"errors"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
)

func TestGetAnalyzerValidateExitCode_Valid(t *testing.T) {
	result := &analyzer.ValidationResult{Valid: true}
	if got := getAnalyzerValidateExitCode(result, nil); got != 0 {
		t.Errorf("expected exit code 0, got %d", got)
	}
}

func TestGetAnalyzerValidateExitCode_Invalid(t *testing.T) {
	result := &analyzer.ValidationResult{Valid: false}
	if got := getAnalyzerValidateExitCode(result, nil); got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}
}

func TestGetAnalyzerValidateExitCode_NilResult(t *testing.T) {
	if got := getAnalyzerValidateExitCode(nil, nil); got != 1 {
		t.Errorf("expected exit code 1 for nil result, got %d", got)
	}
}

func TestGetAnalyzerValidateExitCode_AuthError(t *testing.T) {
	if got := getAnalyzerValidateExitCode(nil, errors.New("request failed with status 401")); got != 2 {
		t.Errorf("expected exit code 2 for 401, got %d", got)
	}
	if got := getAnalyzerValidateExitCode(nil, errors.New("request failed with status 403")); got != 2 {
		t.Errorf("expected exit code 2 for 403, got %d", got)
	}
}

func TestGetAnalyzerValidateExitCode_NetworkError(t *testing.T) {
	if got := getAnalyzerValidateExitCode(nil, errors.New("request failed with status 500")); got != 3 {
		t.Errorf("expected exit code 3 for 500, got %d", got)
	}
	if got := getAnalyzerValidateExitCode(nil, errors.New("dial tcp: connection refused")); got != 3 {
		t.Errorf("expected exit code 3 for connection error, got %d", got)
	}
	if got := getAnalyzerValidateExitCode(nil, errors.New("context deadline exceeded (timeout)")); got != 3 {
		t.Errorf("expected exit code 3 for timeout, got %d", got)
	}
}

func TestGetAnalyzerValidateExitCode_OtherError(t *testing.T) {
	if got := getAnalyzerValidateExitCode(nil, errors.New("some client-side problem")); got != 1 {
		t.Errorf("expected exit code 1 for generic error, got %d", got)
	}
}
