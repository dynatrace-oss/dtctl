package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errEmptyFlagValue is the parse failure an explicitly empty flag value
// produces. pflag wraps it in a *pflag.InvalidValueError that carries the flag
// itself, so enhanceFlagError identifies the case by type instead of matching
// the rendered message — which %q-escapes a tab or a non-breaking space into
// something no "is it blank" pattern recognizes.
var errEmptyFlagValue = errors.New("must not be empty")

// nonEmptyStringValue wraps a string flag so that an explicitly empty value
// (--task "", or --task "$UNSET") fails the parse. Cobra's MarkFlagRequired
// only checks that the flag was given, not that it carries a usable value.
type nonEmptyStringValue struct {
	pflag.Value
	defValue string
}

func (v *nonEmptyStringValue) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errEmptyFlagValue
	}
	return v.Value.Set(value)
}

// Reset restores the declared default. The per-invocation flag reset
// (resetFlagSet) calls it instead of Set(DefValue), which the empty check
// would reject.
func (v *nonEmptyStringValue) Reset() {
	_ = v.Value.Set(v.defValue)
}

// rejectEmptyFlag makes an explicitly empty value for the named string flag
// a usage error.
func rejectEmptyFlag(cmd *cobra.Command, name string) {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		// Persistent flags only join cmd.Flags() once cobra parses, so a
		// global flag has to be looked up where it was declared.
		flag = cmd.PersistentFlags().Lookup(name)
	}
	if flag == nil {
		panic(fmt.Sprintf("rejectEmptyFlag: %s has no --%s flag", cmd.Name(), name))
	}
	flag.Value = &nonEmptyStringValue{Value: flag.Value, defValue: flag.DefValue}
}

// markFlagRequiredNonEmpty marks the named string flag as required and
// rejects an empty value for it.
func markFlagRequiredNonEmpty(cmd *cobra.Command, name string) {
	rejectEmptyFlag(cmd, name)
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
