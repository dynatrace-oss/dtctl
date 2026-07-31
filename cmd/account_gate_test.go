package cmd

import "testing"

// TestAccountExperimentalEnabled documents the truthiness matrix for the
// DTCTL_EXPERIMENTAL_ACCOUNT gate that controls whether the in-development
// account command surface is registered on the root tree.
func TestAccountExperimentalEnabled(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"no", false},
		{"off", false},
		{" off ", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"on", true},
		{"enabled", true},
	}
	for _, tc := range cases {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv(accountExperimentalEnvVar, tc.val)
			if got := accountExperimentalEnabled(); got != tc.want {
				t.Fatalf("accountExperimentalEnabled() with %q = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

// TestAccountCommandNotRegisteredByDefault guards the release-safety invariant:
// with the gate unset (the default under `go test`), the account command must
// not be present on the command tree.
func TestAccountCommandNotRegisteredByDefault(t *testing.T) {
	if accountExperimentalEnabled() {
		t.Skip("DTCTL_EXPERIMENTAL_ACCOUNT is set in this environment; gate is intentionally on")
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == "account" {
			t.Errorf("account command is registered but DTCTL_EXPERIMENTAL_ACCOUNT is unset")
		}
	}
}
