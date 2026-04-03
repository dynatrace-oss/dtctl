package hook

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunPreApply_Success(t *testing.T) {
	result, err := RunPreApply(context.Background(), "cat > /dev/null", "dashboard", "test.yaml", []byte(`{"title":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_Rejected(t *testing.T) {
	result, err := RunPreApply(context.Background(), "echo 'bad' >&2; exit 1", "dashboard", "test.yaml", []byte(`{}`))
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

	_, err := RunPreApply(ctx, "sleep 1", "dashboard", "test.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want it to contain 'timed out'", err.Error())
	}
}

func TestRunPreApply_CommandNotFound(t *testing.T) {
	// When run via sh -c, a missing command results in exit code 127, not a Go exec error.
	result, err := RunPreApply(context.Background(), "nonexistent-binary-that-does-not-exist-xyz", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 127 {
		t.Errorf("ExitCode = %d, want 127 (command not found)", result.ExitCode)
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

func TestRunPreApply_ReceivesJSON(t *testing.T) {
	// Read stdin and verify content matches expected JSON
	result, err := RunPreApply(context.Background(),
		`input=$(cat); test "$input" = '{"title":"test"}'`,
		"dashboard", "test.yaml", []byte(`{"title":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (stdin content mismatch)", result.ExitCode)
	}
}

func TestRunPreApply_ReceivesArgs(t *testing.T) {
	// $1 and $2 are available as positional parameters via sh -c
	result, err := RunPreApply(context.Background(),
		`test "$1" = "workflow" && test "$2" = "my-wf.yaml"`,
		"workflow", "my-wf.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (args mismatch)", result.ExitCode)
	}
}

func TestRunPreApply_FilenameWithSpaces(t *testing.T) {
	result, err := RunPreApply(context.Background(),
		`test "$2" = "my file with spaces.yaml"`,
		"dashboard", "my file with spaces.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (filename with spaces not handled correctly)", result.ExitCode)
	}
}

func TestRunPreApply_ExitCode2(t *testing.T) {
	result, err := RunPreApply(context.Background(), "exit 2", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", result.ExitCode)
	}
}

func TestRunPreApply_StdoutIgnored(t *testing.T) {
	result, err := RunPreApply(context.Background(), "echo 'stdout noise'", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stderr != "" {
		t.Errorf("Stderr = %q, want empty (stdout should not leak into stderr)", result.Stderr)
	}
}

func TestRunPreApply_RealisticHookCommand(t *testing.T) {
	// Simulate a realistic hook: a script that uses args explicitly
	result, err := RunPreApply(context.Background(),
		`resource_type=$1; file=$2; test "$resource_type" = "dashboard" && test "$file" = "dash.yaml"`,
		"dashboard", "dash.yaml", []byte(`{"title":"My Dashboard"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_LargeJSONPayload(t *testing.T) {
	// Build a large JSON payload (~100KB)
	var buf bytes.Buffer
	buf.WriteString(`{"items":[`)
	for i := 0; i < 1000; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"id":"item-`)
		buf.WriteString(strings.Repeat("x", 80))
		buf.WriteString(`","value":`)
		buf.WriteString(strings.Repeat("1", 10))
		buf.WriteString(`}`)
	}
	buf.WriteString(`]}`)

	payload := buf.Bytes()
	if len(payload) < 50000 {
		t.Fatalf("payload too small: %d bytes, want >50KB", len(payload))
	}

	// Hook reads all of stdin and counts bytes
	result, err := RunPreApply(context.Background(),
		`wc -c | tr -d ' ' | { read n; test "$n" -gt 50000; }`,
		"workflow", "large.json", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (hook should receive full payload)", result.ExitCode)
	}
}

func TestRunPreApply_MultiLineStderr(t *testing.T) {
	result, err := RunPreApply(context.Background(),
		`echo "line 1" >&2; echo "line 2" >&2; echo "line 3" >&2; exit 1`,
		"dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	lines := strings.Split(strings.TrimSpace(result.Stderr), "\n")
	if len(lines) != 3 {
		t.Errorf("Stderr lines = %d, want 3; stderr = %q", len(lines), result.Stderr)
	}
	if !strings.Contains(result.Stderr, "line 1") || !strings.Contains(result.Stderr, "line 3") {
		t.Errorf("Stderr = %q, want it to contain 'line 1' and 'line 3'", result.Stderr)
	}
}

func TestRunPreApply_StdinAndArgsUsedTogether(t *testing.T) {
	// Hook reads stdin AND uses $1/$2 positional parameters
	result, err := RunPreApply(context.Background(),
		`input=$(cat); test "$input" = '{"name":"test"}' && test "$1" = "slo" && test "$2" = "slo.yaml"`,
		"slo", "slo.yaml", []byte(`{"name":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (stdin + args should both work)", result.ExitCode)
	}
}

func TestRunPreApply_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := RunPreApply(ctx, "sleep 10", "dashboard", "test.yaml", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	// The error should mention timeout or be a context error
	// (since we cancel immediately, the process may not even start)
}

func TestRunPreApply_EmptySourceFile(t *testing.T) {
	result, err := RunPreApply(context.Background(),
		`test "$2" = ""`,
		"workflow", "", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (empty sourceFile should be passed as empty $2)", result.ExitCode)
	}
}

func TestRunPreApply_SpecialCharsInResourceType(t *testing.T) {
	// Resource types with underscores (like azure_connection)
	result, err := RunPreApply(context.Background(),
		`test "$1" = "azure_connection"`,
		"azure_connection", "conn.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunPreApply_SpecialCharsInSourceFile(t *testing.T) {
	// Source file with special characters (path traversal, quotes, etc.)
	result, err := RunPreApply(context.Background(),
		`test "$2" = "../configs/my file (1).yaml"`,
		"dashboard", "../configs/my file (1).yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (special chars in filename not handled)", result.ExitCode)
	}
}

func TestRunPreApply_StdoutAndStderrSimultaneous(t *testing.T) {
	// Hook writes to both stdout and stderr — only stderr should be captured
	result, err := RunPreApply(context.Background(),
		`echo "stdout output"; echo "stderr output" >&2; exit 1`,
		"dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "stderr output") {
		t.Errorf("Stderr = %q, want it to contain 'stderr output'", result.Stderr)
	}
	if strings.Contains(result.Stderr, "stdout output") {
		t.Errorf("Stderr = %q, should not contain 'stdout output'", result.Stderr)
	}
}

func TestRunPreApply_SuccessStderrPreserved(t *testing.T) {
	// Even on success (exit 0), stderr output should be captured
	result, err := RunPreApply(context.Background(),
		`echo "warning: deprecated format" >&2; exit 0`,
		"dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "warning: deprecated format") {
		t.Errorf("Stderr = %q, want it to contain warning even on success", result.Stderr)
	}
}

func TestRunPreApply_EmptyJSONData(t *testing.T) {
	// Empty byte slice as input
	result, err := RunPreApply(context.Background(),
		`input=$(cat); test -z "$input"`,
		"dashboard", "test.yaml", []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (empty input should work)", result.ExitCode)
	}
}

func TestRunPreApply_BinaryDataOnStdin(t *testing.T) {
	// Ensure binary-safe stdin (null bytes, etc.)
	data := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	result, err := RunPreApply(context.Background(),
		`wc -c | tr -d ' ' | { read n; test "$n" -eq 5; }`,
		"dashboard", "test.yaml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (binary data should pass through)", result.ExitCode)
	}
}

func TestRunPreApply_HighExitCode(t *testing.T) {
	result, err := RunPreApply(context.Background(), "exit 255", "dashboard", "test.yaml", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 255 {
		t.Errorf("ExitCode = %d, want 255", result.ExitCode)
	}
}

func TestRunPreApply_DefaultTimeout(t *testing.T) {
	// Verify DefaultTimeout constant is 30 seconds
	if DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want 30s", DefaultTimeout)
	}
}
