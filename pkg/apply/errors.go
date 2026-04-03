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
	Stderr   string
}

func (e *HookRejectedError) Error() string {
	msg := "pre-apply hook rejected the resource"
	if e.Stderr != "" {
		msg += "\n\nHook stderr:\n"
		for _, line := range strings.Split(strings.TrimSpace(e.Stderr), "\n") {
			msg += "  " + line + "\n"
		}
	}
	msg += fmt.Sprintf("\nHook command: %s\nExit code: %d", e.Command, e.ExitCode)
	return msg
}
