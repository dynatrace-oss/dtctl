package cmd

import (
	"strings"
	"testing"
)

func TestCreateSchedulingRuleFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing --file flag",
			args:    []string{"create", "scheduling-rule"},
			wantErr: "--file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = createSchedulingRuleCmd.Flags().Set("file", "")

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

func TestCreateSchedulingRuleFileFlagRegistered(t *testing.T) {
	f := createSchedulingRuleCmd.Flags().Lookup("file")
	if f == nil {
		t.Fatal("expected --file flag to be registered on createSchedulingRuleCmd")
	}
}
