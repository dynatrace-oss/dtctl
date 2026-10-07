package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func TestRunConcurrentRequiresSession(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version"}, RunOptions{
		Concurrent: true,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != client.ExitUsageError {
		t.Fatalf("exit code = %d, want %d (ExitUsageError)", code, client.ExitUsageError)
	}
	if !strings.Contains(stderr.String(), "RunOptions.Concurrent requires a Session") {
		t.Fatalf("stderr = %q, want the options error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing: the command must not run", stdout.String())
	}
}
