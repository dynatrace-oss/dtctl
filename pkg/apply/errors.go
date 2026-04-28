package apply

import "fmt"

// HookRejectedError is returned when a pre-apply hook exits with a non-zero
// exit code, indicating the resource was rejected by the hook.
type HookRejectedError struct {
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
}

func (e *HookRejectedError) Error() string {
	return fmt.Sprintf("pre-apply hook rejected the resource\nHook command: %s\nExit code: %d", e.Command, e.ExitCode)
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
