package apply

import (
	"fmt"
	"strings"
)

// HookRejectedError is returned when a pre-apply hook exits with a non-zero
// exit code, indicating the resource was rejected by the hook.
type HookRejectedError struct {
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
}

func (e *HookRejectedError) Error() string {
	msg := "pre-apply hook rejected the resource"
	if e.Stdout != "" {
		msg += "\n\nHook stdout:\n"
		for _, line := range strings.Split(strings.TrimSpace(e.Stdout), "\n") {
			msg += "  " + line + "\n"
		}
	}
	if e.Stderr != "" {
		msg += "\n\nHook stderr:\n"
		for _, line := range strings.Split(strings.TrimSpace(e.Stderr), "\n") {
			msg += "  " + line + "\n"
		}
	}
	msg += fmt.Sprintf("\nHook command: %s\nExit code: %d", e.Command, e.ExitCode)
	return msg
}

// ListApplyError is returned when some items in a batch apply fail.
// It includes results for successful items alongside the error details.
type ListApplyError struct {
	Total    int
	Failed   int
	Messages []string
}

func (e *ListApplyError) Error() string {
	msg := fmt.Sprintf("%d of %d items failed to apply", e.Failed, e.Total)
	for _, m := range e.Messages {
		msg += "\n  " + m
	}
	return msg
}
