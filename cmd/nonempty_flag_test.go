package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

// TestEmptyFlagValueIsAUsageError covers the whole invocation: a live run
// showed the rejection exiting 1 with cobra's raw wording, where a bad flag
// value is a usage error (exit 2). The tab and the non-breaking space are
// there because %q escapes them, which a message-matching enhancer missed.
func TestEmptyFlagValueIsAUsageError(t *testing.T) {
	values := map[string]string{
		"empty":              "",
		"space":              " ",
		"tab":                "\t",
		"newline":            "\n",
		"non-breaking space": " ",
	}

	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { resetFlagSet(logsWorkflowExecutionCmd.Flags()) })

			args := []string{"--plain", "logs", "wfe", "exec-1", "--task", value}
			if code := Run(args, RunOptions{}); code != client.ExitUsageError {
				t.Errorf("exit code = %d, want %d", code, client.ExitUsageError)
			}
		})
	}
}

// TestEmptyFlagValueIsRejected covers #494: an explicitly empty value, as an
// unset shell variable produces, must fail the parse instead of counting as
// "flag given" (required flags) or as "flag absent" (logs --task).
func TestEmptyFlagValueIsRejected(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *cobra.Command
		wantFlag string
		args     []string
	}{
		{"logs task", logsWorkflowExecutionCmd, "task", []string{"logs", "wfe", "exec-1", "--task", ""}},
		{"logs task with all", logsWorkflowExecutionCmd, "task", []string{"logs", "wfe", "exec-1", "--task=", "--all"}},
		{"task result", getWfeTaskResultCmd, "task", []string{"get", "wfe-task-result", "exec-1", "--task", " "}},
		{"settings schema", createSettingsCmd, "schema", []string{"create", "settings", "-f", "x.yaml", "--schema", "", "--scope", "environment"}},
		{"apply file", applyCmd, "file", []string{"apply", "-f", ""}},
		{"global context", rootCmd, "context", []string{"get", "workflows", "--context", ""}},
		{"global config", rootCmd, "config", []string{"get", "workflows", "--config", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				resetFlagSet(tt.cmd.Flags())
				resetFlagSet(rootCmd.PersistentFlags())
			})

			setupErrorHandlers(rootCmd)
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected an error for an empty flag value, got nil")
			}

			var flagErr *suggest.FlagError
			if !errors.As(err, &flagErr) {
				t.Fatalf("error = %q (%T), want a *suggest.FlagError so the invocation exits with the usage code", err, err)
			}
			if flagErr.Flag != tt.wantFlag {
				t.Errorf("flag = %q, want %q", flagErr.Flag, tt.wantFlag)
			}
			if code := exitCodeForError(err); code != client.ExitUsageError {
				t.Errorf("exit code = %d, want %d", code, client.ExitUsageError)
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

// TestUnknownFlagIsAUsageError guards the enhancer against enhancing twice:
// the second pass no longer recognizes its own wording and used to hand back
// an untyped error, which cost the invocation its exit code.
func TestUnknownFlagIsAUsageError(t *testing.T) {
	t.Cleanup(func() { resetFlagSet(rootCmd.PersistentFlags()) })

	if code := Run([]string{"--plain", "logs", "wfe", "exec-1", "--bogus"}, RunOptions{}); code != client.ExitUsageError {
		t.Errorf("exit code = %d, want %d", code, client.ExitUsageError)
	}
}
