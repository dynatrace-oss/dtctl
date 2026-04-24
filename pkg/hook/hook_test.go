package hook

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript writes a bash script with the given body to a tmp file in
// t.TempDir() and returns a hook-command string ("bash /abs/path").
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	return "bash " + path
}

func TestRunPreApply_Success(t *testing.T) {
	cmd := writeScript(t, "cat > /dev/null\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{"title":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_Rejected(t *testing.T) {
	cmd := writeScript(t, "echo bad >&2; exit 1\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "bad") {
		t.Errorf("Stderr = %q, want it to contain 'bad'", result.Stderr)
	}
}

func TestRunPreApply_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// A script that sleeps long enough to exceed the context deadline,
	// regardless of the trailing resource-type and source-file args dtctl
	// appends.
	cmd := writeScript(t, "sleep 5\n")
	_, err := RunPreApply(ctx, cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want it to contain 'timed out'", err.Error())
	}
}

func TestRunPreApply_CommandNotFound(t *testing.T) {
	// Direct exec of a missing binary returns a Go "not found" error
	// (distinct from a 127 exit that sh -c would produce).
	_, err := RunPreApply(context.Background(), "nonexistent-binary-that-does-not-exist-xyz", "dashboard", "test.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestRunPreApply_EmptyCommand(t *testing.T) {
	result, err := RunPreApply(context.Background(), "", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_WhitespaceOnlyCommand(t *testing.T) {
	result, err := RunPreApply(context.Background(), "   \t  ", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_ReceivesJSON(t *testing.T) {
	cmd := writeScript(t, `input=$(cat); test "$input" = '{"title":"test"}'`+"\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{"title":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (stdin content mismatch); stderr=%q", result.ExitCode, result.Stderr)
	}
}

func TestRunPreApply_ReceivesResourceTypeAndSourceAsArgs(t *testing.T) {
	// The hook is invoked directly (no sh -c). The resource type and source
	// file are appended as the final two positional args. The script sees
	// them as $1 and $2 (bash). This is the contract that replaces the
	// old sh -c '<cmd>' -- $rtype $src form.
	cmd := writeScript(t, `test "$1" = dashboard && test "$2" = test.yaml`+"\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (positional arg mismatch); stderr=%q", result.ExitCode, result.Stderr)
	}
}

func TestRunPreApply_CapturesStdoutAndStderr(t *testing.T) {
	cmd := writeScript(t, "echo hello-stdout; echo hello-stderr >&2\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello-stdout") {
		t.Errorf("Stdout = %q, want it to contain hello-stdout", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "hello-stderr") {
		t.Errorf("Stderr = %q, want it to contain hello-stderr", result.Stderr)
	}
}

func TestRunPostApply_Success(t *testing.T) {
	cmd := writeScript(t, "cat > /dev/null; echo hello-stdout; echo hello-stderr >&2\n")
	result, err := RunPostApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`[{"id":"x"}]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello-stdout") {
		t.Errorf("Stdout = %q, want it to contain hello-stdout", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "hello-stderr") {
		t.Errorf("Stderr = %q, want it to contain hello-stderr", result.Stderr)
	}
}

func TestRunPostApply_Failure(t *testing.T) {
	cmd := writeScript(t, "echo broken >&2; exit 7\n")
	result, err := RunPostApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "broken") {
		t.Errorf("Stderr = %q, want it to contain broken", result.Stderr)
	}
}

func TestRunPostApply_EmptyCommand(t *testing.T) {
	result, err := RunPostApply(context.Background(), "", "dashboard", "test.yaml", []byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPostApply_ReceivesResultJSONAndArgs(t *testing.T) {
	cmd := writeScript(t,
		`input=$(cat); test "$input" = '[{"id":"abc"}]' && test "$1" = notebook && test "$2" = src.json`+"\n")
	result, err := RunPostApply(context.Background(), cmd, "notebook", "src.json", []byte(`[{"id":"abc"}]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
}
