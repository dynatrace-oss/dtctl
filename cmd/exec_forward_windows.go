//go:build windows

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// execForward runs the target binary (a dtctl-* plugin) as a child
// with inherited stdio — Windows has no process replacement — and returns
// the child's exit code verbatim so scripts see the target's code, not a
// cobra-wrapped error.
func execForward(bin string, argv []string, env []string) (int, error) {
	cmd := exec.Command(bin, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to launch %s: %w", bin, err)
	}
	return 0, nil
}
