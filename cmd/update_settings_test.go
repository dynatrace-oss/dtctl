package cmd

import (
	"strings"
	"testing"
)

func TestUpdateSettingsRedirectsToApply(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"settings with file flag", []string{"update", "settings", "some-id", "-f", "config.yaml"}},
		{"setting alias", []string{"update", "setting", "some-id", "-f", "config.yaml"}},
		{"settings without flags", []string{"update", "settings"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected error with redirect hint")
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "dtctl apply -f") {
				t.Errorf("expected hint to use 'dtctl apply -f', got: %s", errMsg)
			}
			if !strings.Contains(errMsg, "objectId") {
				t.Errorf("expected hint to mention objectId, got: %s", errMsg)
			}
		})
	}
}
