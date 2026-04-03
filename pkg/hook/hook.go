package hook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// DefaultTimeout is the maximum time a hook is allowed to run.
const DefaultTimeout = 30 * time.Second

// Result holds the outcome of a hook execution.
type Result struct {
	ExitCode int
	Stderr   string
	Duration time.Duration
}

// RunPreApply executes the pre-apply hook command.
// The command is run via "sh -c" with resource type and source file as
// positional parameters ($1 and $2). Processed JSON is piped to stdin.
//
// sourceFile is the original filename that was passed to "dtctl apply -f".
// It is informational only — the hook MUST read the resource content from
// stdin (which contains the processed JSON after YAML→JSON conversion and
// template rendering), not from this file path.
//
// Returns a Result with ExitCode 0 on success. A non-zero ExitCode means the
// hook rejected the resource (this is not an error). An error return indicates
// the hook could not be executed at all (not found, timed out, etc.).
//
// If command is empty, the hook is a no-op and returns ExitCode 0.
func RunPreApply(ctx context.Context, command string, resourceType string, sourceFile string, jsonData []byte) (*Result, error) {
	if command == "" {
		return &Result{ExitCode: 0}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Pass resource type and source file as positional parameters.
	// The "--" separates sh options from the positional args.
	// Inside the hook: $1 = resource type, $2 = source file (available but not
	// appended to the command — the hook references them explicitly if needed).
	// Note: $2 is the original filename for context/logging only. The actual
	// resource content is always on stdin (processed JSON).
	cmd := exec.CommandContext(ctx, "sh", "-c", command, "--", resourceType, sourceFile)
	cmd.Stdin = bytes.NewReader(jsonData)
	cmd.Stdout = io.Discard

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("pre-apply hook timed out after %s", DefaultTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &Result{
				ExitCode: exitErr.ExitCode(),
				Stderr:   stderr.String(),
				Duration: elapsed,
			}, nil
		}
		return nil, fmt.Errorf("pre-apply hook failed to execute: %w", err)
	}

	return &Result{ExitCode: 0, Stderr: stderr.String(), Duration: elapsed}, nil
}
