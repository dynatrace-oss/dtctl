package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// nonEmptyStringValue wraps a string flag so that an explicitly empty value
// (--task "", or --task "$UNSET") fails the parse. Cobra's MarkFlagRequired
// only checks that the flag was given, not that it carries a usable value.
type nonEmptyStringValue struct {
	pflag.Value
	defValue string
}

func (v *nonEmptyStringValue) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("must not be empty")
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
	flag.Value = &nonEmptyStringValue{Value: flag.Value, defValue: flag.DefValue}
}

// markFlagRequiredNonEmpty marks the named string flag as required and
// rejects an empty value for it.
func markFlagRequiredNonEmpty(cmd *cobra.Command, name string) {
	rejectEmptyFlag(cmd, name)
	_ = cmd.MarkFlagRequired(name)
}
