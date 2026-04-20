package cmd

import (
	"strings"
	"testing"
)

func TestCreateExtensionFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "neither --file nor --hub-extension",
			args:    []string{"create", "extension"},
			wantErr: "either --file or --hub-extension is required",
		},
		{
			name:    "--file and --hub-extension together",
			args:    []string{"create", "extension", "-f", "foo.zip", "--hub-extension", "com.dynatrace.extension.foo"},
			wantErr: "--file and --hub-extension are mutually exclusive",
		},
		{
			name:    "--version with --file is rejected",
			args:    []string{"create", "extension", "-f", "foo.zip", "--version", "1.2.3"},
			wantErr: "--version only applies to --hub-extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags between cases since they're package-global on the cobra command.
			_ = createExtensionCmd.Flags().Set("file", "")
			_ = createExtensionCmd.Flags().Set("hub-extension", "")
			_ = createExtensionCmd.Flags().Set("version", "")

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
