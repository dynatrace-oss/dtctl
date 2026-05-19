package hook

import (
	"bytes"
	"context"
	"fmt"
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
	// Use forward slashes so shlex tokenization works on Windows too
	// (backslashes in Windows paths are consumed as escape chars by shlex).
	return "bash " + filepath.ToSlash(path)
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

// TestRunPreApply_PathWithSpaces verifies that quoting in the hook command
// string survives tokenization, so paths containing spaces work. This is the
// common macOS case ("/Users/joe/Library/Application Support/...") that
// naive whitespace splitting would silently break.
func TestRunPreApply_PathWithSpaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scriptPath := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	command := `bash "` + filepath.ToSlash(scriptPath) + `"`
	result, err := RunPreApply(context.Background(), command, "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
}

// TestRunPreApply_QuotedArgsPreserved verifies that quoted arguments
// containing spaces are passed as a single argv entry, not split.
func TestRunPreApply_QuotedArgsPreserved(t *testing.T) {
	// Hook script asserts $1 (the first user arg, before the appended
	// resourceType/sourceFile) is the multi-word string passed via quoting.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "hook.sh")
	body := `#!/usr/bin/env bash
test "$1" = "two words" || { echo "got '$1'" >&2; exit 1; }
test "$2" = workflow || { echo "got '$2'" >&2; exit 1; }
test "$3" = my.yaml || { echo "got '$3'" >&2; exit 1; }
`
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	command := `bash ` + filepath.ToSlash(scriptPath) + ` "two words"`
	result, err := RunPreApply(context.Background(), command, "workflow", "my.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
}

// TestRunPreApply_UnterminatedQuoteIsError verifies that a malformed command
// string (unterminated quote) produces a clear error instead of silently
// running with the wrong tokens.
func TestRunPreApply_UnterminatedQuoteIsError(t *testing.T) {
	_, err := RunPreApply(context.Background(), `bash "missing-end`, "dashboard", "x.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unterminated quote, got nil")
	}
	if !strings.Contains(err.Error(), "hook command") {
		t.Errorf("error = %v, want it to mention hook command parsing", err)
	}
}

func TestRunPreApply_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run — exec.CommandContext must propagate the error

	cmd := writeScript(t, "sleep 10\n")
	_, err := RunPreApply(ctx, cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestRunPreApply_LargeJSONPayload(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{"items":[`)
	for i := range 1000 {
		if i > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, `{"id":"item-%s","value":%s}`,
			strings.Repeat("x", 80), strings.Repeat("1", 10))
	}
	buf.WriteString(`]}`)
	payload := buf.Bytes()
	if len(payload) < 50000 {
		t.Fatalf("payload too small: %d bytes", len(payload))
	}

	cmd := writeScript(t, `count=$(wc -c | tr -d ' '); test "$count" -gt 50000`)
	result, err := RunPreApply(context.Background(), cmd, "workflow", "large.json", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (hook should receive full payload); stderr=%q", result.ExitCode, result.Stderr)
	}
}

func TestRunPreApply_BinaryDataOnStdin(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	// Drain stdin and exit 0 — verifies binary data doesn't cause pipe or exec failures.
	cmd := writeScript(t, "cat > /dev/null\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (binary data should pass through)", result.ExitCode)
	}
}

func TestRunPreApply_EmptySourceFile(t *testing.T) {
	cmd := writeScript(t, `test "$2" = ""`)
	result, err := RunPreApply(context.Background(), cmd, "workflow", "", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (empty sourceFile should be passed as empty $2); stderr=%q", result.ExitCode, result.Stderr)
	}
}

func TestRunPreApply_HighExitCode(t *testing.T) {
	cmd := writeScript(t, "exit 255\n")
	result, err := RunPreApply(context.Background(), cmd, "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 255 {
		t.Errorf("ExitCode = %d, want 255", result.ExitCode)
	}
}
