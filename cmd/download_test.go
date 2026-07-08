package cmd

import (
	"strings"
	"testing"
)

func TestDownloadExtensionValidation(t *testing.T) {
	origAgentMode := agentMode
	origOutputFormat := outputFormat
	defer func() {
		agentMode = origAgentMode
		outputFormat = origOutputFormat
	}()

	tests := []struct {
		name      string
		args      []string
		agentMode bool
		wantErr   string
	}{
		{
			name:    "requires version",
			args:    []string{"download", "extension", "com.dynatrace.extension.postgres"},
			wantErr: "--version is required",
		},
		{
			name:    "rejects output flag",
			args:    []string{"download", "extension", "com.dynatrace.extension.postgres", "--version", "1.2.3", "-o", "json"},
			wantErr: "does not support -o output formatting",
		},
		{
			name:      "rejects agent mode",
			args:      []string{"download", "extension", "com.dynatrace.extension.postgres", "--version", "1.2.3"},
			agentMode: true,
			wantErr:   "incompatible with agent mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentMode = tt.agentMode
			outputFormat = "table"

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
