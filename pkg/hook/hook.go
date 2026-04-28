package hook

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
)

// DefaultTimeout is the maximum time a hook is allowed to run.
const DefaultTimeout = 30 * time.Second

// Result holds the outcome of a hook execution.
type Result struct {
	ExitCode int
	Stderr   string
	Stdout   string
	Duration time.Duration
}

// tokenizeCommand splits a hook command string into argv using POSIX-style
// shell quoting rules so that paths with spaces and quoted arguments work:
//
//	bash "/Users/joe/My Hooks/validate.sh"   -> ["bash", "/Users/joe/My Hooks/validate.sh"]
//	node validate.js --rule "no-empty-titles" -> ["node", "validate.js", "--rule", "no-empty-titles"]
//
// The command is NOT run through a shell, so pipes/redirections/glob
// expansion still require an explicit interpreter (e.g. `bash -c '<script>'`).
//
// Returns (nil, nil) for an empty or whitespace-only command, signalling
// "no-op hook". Returns an error if the quoting is malformed (e.g. an
// unterminated single-quoted string).
func tokenizeCommand(command string) ([]string, error) {
	if strings.TrimSpace(command) == "" {
		return nil, nil
	}
	tokens, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hook command (check for unmatched quotes): %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	return tokens, nil
}

// RunPreApply executes the pre-apply hook command.
//
// The command string is tokenized with POSIX-style shell quoting (see
// tokenizeCommand) and executed directly (NOT via "sh -c"). The resource
// type and source file are appended as the two final arguments of the
// process. Processed JSON is piped to stdin.
//
// Example: command `bash /path/to/validate.sh` becomes
// exec.Command("bash", "/path/to/validate.sh", "<rtype>", "<sourceFile>").
// Quoting is honoured, so `bash "/path with spaces/validate.sh"` works.
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

	tokens, err := tokenizeCommand(command)
	if err != nil {
		return nil, fmt.Errorf("pre-apply hook: %w", err)
	}
	if len(tokens) == 0 {
		return &Result{ExitCode: 0}, nil
	}
	args := make([]string, 0, len(tokens)-1+2)
	args = append(args, tokens[1:]...)
	args = append(args, resourceType, sourceFile)
	cmd := exec.CommandContext(ctx, tokens[0], args...)
	cmd.Stdin = bytes.NewReader(jsonData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("pre-apply hook timed out after %s", DefaultTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &Result{
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Duration: elapsed,
			}, nil
		}
		return nil, fmt.Errorf("pre-apply hook failed to execute: %w", err)
	}

	return &Result{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(), Duration: elapsed}, nil
}

// RunPostApply executes the post-apply hook command.
//
// Invoked the same way as RunPreApply — POSIX-style tokenize, append
// resource type and source file as the two final args, run directly (no
// "sh -c"). Stdin is the apply result as JSON, so the hook can read the
// created/updated
// resource's id, name, url, etc. Stdout and stderr are both captured and
// returned; the caller is responsible for printing them to the user (a
// post-apply hook's output is always relevant, including on success).
//
// The caller is responsible for deciding how a non-zero ExitCode maps to
// dtctl apply's overall result — post-apply runs after the resource is
// already persisted, so a hook-level failure is typically a warning.
func RunPostApply(ctx context.Context, command string, resourceType string, sourceFile string, resultJSON []byte) (*Result, error) {
	if command == "" {
		return &Result{ExitCode: 0}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	tokens, err := tokenizeCommand(command)
	if err != nil {
		return nil, fmt.Errorf("post-apply hook: %w", err)
	}
	if len(tokens) == 0 {
		return &Result{ExitCode: 0}, nil
	}
	args := make([]string, 0, len(tokens)-1+2)
	args = append(args, tokens[1:]...)
	args = append(args, resourceType, sourceFile)
	cmd := exec.CommandContext(ctx, tokens[0], args...)
	cmd.Stdin = bytes.NewReader(resultJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("post-apply hook timed out after %s", DefaultTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &Result{
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Duration: elapsed,
			}, nil
		}
		return nil, fmt.Errorf("post-apply hook failed to execute: %w", err)
	}

	return &Result{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: elapsed,
	}, nil
}
