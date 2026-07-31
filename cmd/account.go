package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// accountExperimentalEnvVar gates the still-in-development platform "account"
// command surface (account login/status/token). The command is only registered
// on the root tree when this is set to a truthy value, so released builds hide
// it entirely (an invocation yields the normal "unknown command" error). Remove
// the gate — and this env var — once the account surface is GA.
const accountExperimentalEnvVar = "DTCTL_EXPERIMENTAL_ACCOUNT"

// accountExperimentalEnabled reports whether the experimental account command
// surface should be registered. Any value other than empty/0/false/no/off
// (case-insensitive) enables it.
func accountExperimentalEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(accountExperimentalEnvVar))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Platform account administration",
	Long:  "Commands for managing Dynatrace platform account resources.",
	RunE:  requireSubcommand,
}

func init() {
	// Account administration is still under development; keep it out of released
	// builds unless explicitly opted in via DTCTL_EXPERIMENTAL_ACCOUNT. Skipping
	// registration (rather than hiding) means end users get a plain "unknown
	// command" and the feature's existence is not surfaced in help or the
	// `dtctl commands` catalog. See docs — remove this gate when account is GA.
	if !accountExperimentalEnabled() {
		return
	}
	rootCmd.AddCommand(accountCmd)
}
