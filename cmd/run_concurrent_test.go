package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestRLockRunGivesUpAndReleasesLate(t *testing.T) {
	runMu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := rlockRun(ctx)
	runMu.Unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rlockRun = %v, want DeadlineExceeded", err)
	}

	locked := make(chan struct{})
	go func() {
		runMu.Lock()
		close(locked)
	}()
	select {
	case <-locked:
		runMu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("the shared lock acquired after giving up was never released")
	}
}
