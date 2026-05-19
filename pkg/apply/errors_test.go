package apply

import (
	"strings"
	"testing"
)

func TestHookRejectedError_Basic(t *testing.T) {
	err := &HookRejectedError{
		Command:  "validate.sh",
		ExitCode: 1,
		Stderr:   "missing required field: title",
	}

	msg := err.Error()

	if !strings.Contains(msg, "pre-apply hook rejected the resource") {
		t.Errorf("Error() missing prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "Hook command: validate.sh") {
		t.Errorf("Error() missing command, got: %s", msg)
	}
	if !strings.Contains(msg, "Exit code: 1") {
		t.Errorf("Error() missing exit code, got: %s", msg)
	}
	// Hook output is printed by the applier before the error is raised, not embedded in Error()
	if strings.Contains(msg, "missing required field") {
		t.Errorf("Error() should not embed hook stderr (applier prints it separately), got: %s", msg)
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
	err := &HookRejectedError{
		Command:  "nonexistent-command",
		ExitCode: 127,
		Stderr:   "sh: nonexistent-command: not found",
	}

	msg := err.Error()

	if !strings.Contains(msg, "Exit code: 127") {
		t.Errorf("Error() missing exit code 127, got: %s", msg)
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
