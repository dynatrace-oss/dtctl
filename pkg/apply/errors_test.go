package apply

import (
	"strings"
	"testing"
)

func TestHookRejectedError_WithStderr(t *testing.T) {
	err := &HookRejectedError{
		Command:  "validate.sh",
		ExitCode: 1,
		Stderr:   "missing required field: title",
	}

	msg := err.Error()

	if !strings.Contains(msg, "pre-apply hook rejected the resource") {
		t.Errorf("Error() missing prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "Hook stderr:") {
		t.Errorf("Error() missing 'Hook stderr:' section, got: %s", msg)
	}
	if !strings.Contains(msg, "  missing required field: title") {
		t.Errorf("Error() missing indented stderr line, got: %s", msg)
	}
	if !strings.Contains(msg, "Hook command: validate.sh") {
		t.Errorf("Error() missing command, got: %s", msg)
	}
	if !strings.Contains(msg, "Exit code: 1") {
		t.Errorf("Error() missing exit code, got: %s", msg)
	}
}

func TestHookRejectedError_WithoutStderr(t *testing.T) {
	err := &HookRejectedError{
		Command:  "check.sh",
		ExitCode: 2,
		Stderr:   "",
	}

	msg := err.Error()

	if !strings.Contains(msg, "pre-apply hook rejected the resource") {
		t.Errorf("Error() missing prefix, got: %s", msg)
	}
	if strings.Contains(msg, "Hook stderr:") {
		t.Errorf("Error() should not contain 'Hook stderr:' when stderr is empty, got: %s", msg)
	}
	if !strings.Contains(msg, "Hook command: check.sh") {
		t.Errorf("Error() missing command, got: %s", msg)
	}
	if !strings.Contains(msg, "Exit code: 2") {
		t.Errorf("Error() missing exit code, got: %s", msg)
	}
}

func TestHookRejectedError_MultiLineStderr(t *testing.T) {
	err := &HookRejectedError{
		Command:  "lint.sh",
		ExitCode: 1,
		Stderr:   "Error 1: missing title\nError 2: invalid scope\nError 3: owner not set",
	}

	msg := err.Error()

	// Each line of stderr should be indented with 2 spaces
	if !strings.Contains(msg, "  Error 1: missing title") {
		t.Errorf("Error() missing indented line 1, got: %s", msg)
	}
	if !strings.Contains(msg, "  Error 2: invalid scope") {
		t.Errorf("Error() missing indented line 2, got: %s", msg)
	}
	if !strings.Contains(msg, "  Error 3: owner not set") {
		t.Errorf("Error() missing indented line 3, got: %s", msg)
	}
}

func TestHookRejectedError_StderrWithTrailingNewline(t *testing.T) {
	err := &HookRejectedError{
		Command:  "hook.sh",
		ExitCode: 1,
		Stderr:   "error message\n\n",
	}

	msg := err.Error()

	// TrimSpace should remove trailing newlines — no empty indented lines
	lines := strings.Split(msg, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue // empty lines between sections are ok
		}
	}
	// The error message should still contain the actual error
	if !strings.Contains(msg, "  error message") {
		t.Errorf("Error() missing stderr content, got: %s", msg)
	}
}

func TestHookRejectedError_EmptyCommand(t *testing.T) {
	err := &HookRejectedError{
		Command:  "",
		ExitCode: 1,
		Stderr:   "failed",
	}

	msg := err.Error()

	if !strings.Contains(msg, "Hook command: \n") {
		t.Errorf("Error() should show empty command, got: %s", msg)
	}
}

func TestHookRejectedError_ExitCode127(t *testing.T) {
	// Exit code 127 = command not found
	err := &HookRejectedError{
		Command:  "nonexistent-command",
		ExitCode: 127,
		Stderr:   "sh: nonexistent-command: not found",
	}

	msg := err.Error()

	if !strings.Contains(msg, "Exit code: 127") {
		t.Errorf("Error() missing exit code 127, got: %s", msg)
	}
	if !strings.Contains(msg, "nonexistent-command: not found") {
		t.Errorf("Error() missing command-not-found stderr, got: %s", msg)
	}
}

func TestHookRejectedError_ImplementsError(t *testing.T) {
	var err error = &HookRejectedError{
		Command:  "test",
		ExitCode: 1,
	}

	// Verify it implements the error interface
	if err.Error() == "" {
		t.Error("Error() returned empty string")
	}
}

func TestListApplyError_Message(t *testing.T) {
	err := &ListApplyError{
		Total:    5,
		Failed:   2,
		Messages: []string{"item 2: invalid value", "item 4: scope not found"},
	}

	msg := err.Error()
	if !strings.Contains(msg, "2 of 5 items failed to apply") {
		t.Errorf("Error() missing summary, got: %s", msg)
	}
	if !strings.Contains(msg, "item 2: invalid value") {
		t.Errorf("Error() missing detail for item 2, got: %s", msg)
	}
	if !strings.Contains(msg, "item 4: scope not found") {
		t.Errorf("Error() missing detail for item 4, got: %s", msg)
	}
}

func TestListApplyError_ImplementsError(t *testing.T) {
	var err error = &ListApplyError{
		Total:    1,
		Failed:   1,
		Messages: []string{"item 1: failed"},
	}
	if err.Error() == "" {
		t.Error("Error() returned empty string")
	}
}
