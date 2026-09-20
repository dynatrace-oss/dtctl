package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// TestEmptyFlagValueIsAUsageError covers the whole invocation: a live run
// showed the rejection exiting 1 with cobra's raw wording, where a bad flag
// value is a usage error (exit 2).
func TestEmptyFlagValueIsAUsageError(t *testing.T) {
	args := []string{"logs", "wfe", "exec-1", "--task", "", "--plain"}
	t.Cleanup(func() { resetFlagSet(logsWorkflowExecutionCmd.Flags()) })

	if code := Run(args, RunOptions{}); code != client.ExitUsageError {
		t.Errorf("exit code = %d, want %d", code, client.ExitUsageError)
	}
}

// TestEmptyFlagValueIsRejected covers #494: an explicitly empty value, as an
// unset shell variable produces, must fail the parse instead of counting as
// "flag given" (required flags) or as "flag absent" (logs --task).
func TestEmptyFlagValueIsRejected(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"logs task", logsWorkflowExecutionCmd, []string{"logs", "wfe", "exec-1", "--task", ""}},
		{"logs task with all", logsWorkflowExecutionCmd, []string{"logs", "wfe", "exec-1", "--task=", "--all"}},
		{"task result", getWfeTaskResultCmd, []string{"get", "wfe-task-result", "exec-1", "--task", " "}},
		{"settings schema", createSettingsCmd, []string{"create", "settings", "-f", "x.yaml", "--schema", "", "--scope", "environment"}},
		{"apply file", applyCmd, []string{"apply", "-f", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				resetFlagSet(tt.cmd.Flags())
			})

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected an error for an empty flag value, got nil")
			}
			// Either cobra's own wording, or the usage error enhanceFlagError
			// makes of it once the error handlers are installed.
			if !strings.Contains(err.Error(), "must not be empty") &&
				!strings.Contains(err.Error(), "empty value") {
				t.Errorf("error = %q, want it to say the value must not be empty", err)
			}
		})
	}
}

func TestNonEmptyFlagResetRestoresDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("name", "fallback", "")
	rejectEmptyFlag(cmd, "name")

	if err := cmd.Flags().Set("name", "given"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	resetFlagSet(cmd.Flags())

	if got, _ := cmd.Flags().GetString("name"); got != "fallback" {
		t.Errorf("after reset --name = %q, want %q", got, "fallback")
	}
}
