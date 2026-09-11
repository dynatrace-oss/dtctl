package cmd

import (
	"os"
	"strings"
)

// ExperimentalEnabled reports whether the experimental feature gated by the
// given environment variable is switched on. Any value other than
// empty/0/false/no/off (case-insensitive) enables it.
//
// Experimental surfaces are gated by *skipping registration*, not by hiding the
// command: an end user of a released build gets a plain "unknown command", and
// the feature's existence is not advertised in help or the `dtctl commands`
// catalog. It is exported so surfaces that live outside this package can share
// one definition of "enabled" — `dtctl serve` (pkg/serve, wired in main) is the
// case that needs it.
func ExperimentalEnabled(envVar string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envVar))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
