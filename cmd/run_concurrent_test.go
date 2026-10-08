package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
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

// TestSessionInvocationsReadNoHostConfig holds what a Session promises: a
// session-backed invocation is detached from the host's config, so it does not
// open DTCTL_CONFIG, a .dtctl.yaml above the host's working directory, or the
// global config, not even to discard what it read. An invocation without a
// session still loads it (for its aliases), which is what shows the check
// watches the right call.
func TestSessionInvocationsReadNoHostConfig(t *testing.T) {
	var loads atomic.Int64
	orig := loadHostConfig
	loadHostConfig = func() (*config.Config, error) {
		loads.Add(1)
		return nil, errors.New("no host config in this test")
	}
	t.Cleanup(func() { loadHostConfig = orig })

	session := &Session{EnvironmentURL: "https://env.example.test", Token: "dt0c01.T.T"}
	for _, tc := range []struct {
		name string
		opts RunOptions
	}{
		{"concurrent", RunOptions{Concurrent: true, Session: session}},
		{"serialized", RunOptions{Session: session}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loads.Store(0)
			var stdout, stderr bytes.Buffer
			tc.opts.Stdout, tc.opts.Stderr = &stdout, &stderr
			if code := Run([]string{"version"}, tc.opts); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			if n := loads.Load(); n != 0 {
				t.Errorf("a session-backed invocation loaded the host config %d time(s)", n)
			}
		})
	}

	t.Run("without a session", func(t *testing.T) {
		loads.Store(0)
		var stdout, stderr bytes.Buffer
		Run([]string{"version"}, RunOptions{Stdout: &stdout, Stderr: &stderr})
		if loads.Load() == 0 {
			t.Error("an invocation without a session did not load the host config for its aliases; this test no longer watches the right call")
		}
	})
}
