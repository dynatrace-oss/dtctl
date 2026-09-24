package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// --limit and --fields for every `get` list verb. They are persistent on
// getCmd so each subcommand inherits them; a subcommand that already had its
// own (server-side) --limit keeps it, because cobra lets a local flag shadow
// an inherited one of the same name. Shaping happens in the printer
// (output.ShapingPrinter), after the command fetched its data, so no
// subcommand needs to know about either flag.
var (
	getListLimit  int
	getListFields string
)

// listShapeSince is the release that introduced --limit/--fields on get.
const listShapeSince = "0.40.0"

// agentDefaultPage is how many items a get list returns in agent mode when
// --limit is not given: a full listing routinely overflows an agent's output
// budget, and the envelope says when (and how) more is available. --limit 0
// restores the full list.
const agentDefaultPage = 50

// invokedGetCmd is the get subcommand this invocation is running, set by the
// RunE wrapper installGetListPaging adds. NewPrinter has no command in hand,
// and the default page must apply to get list verbs only.
var invokedGetCmd *cobra.Command

// installGetListPaging wraps every runnable command under cmd so that it
// records itself in invokedGetCmd for the duration of its RunE. Like the scope
// preflight, it is re-applied on each invocation over the pristine tree.
func installGetListPaging(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		installGetListPaging(sub)
	}
	orig := cmd.RunE
	if orig == nil {
		return
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		invokedGetCmd = c
		defer func() { invokedGetCmd = nil }()
		return orig(c, args)
	}
}

// agentPageLimit is the limit a get list verb with its own server-side
// --limit should request: the agent-mode default page unless --limit was
// given explicitly (including --limit 0), in which case limit as passed.
func agentPageLimit(cmd *cobra.Command, limit int64) int64 {
	if agentMode && !cmd.Flags().Changed("limit") {
		return agentDefaultPage
	}
	return limit
}

func init() {
	getCmd.PersistentFlags().IntVar(&getListLimit, "limit", 0,
		"return at most this many items (0 = all; agent mode defaults to 50); applied after fetching, so --chunk-size still controls paging")
	getCmd.PersistentFlags().StringVar(&getListFields, "fields", "",
		"comma-separated fields to keep, in this order; dotted paths reach nested fields (modificationInfo.lastModifiedTime). table/csv/toon flatten them into columns")

	stability.MarkFlag(getCmd, "limit", stability.Experimental, listShapeSince)
	stability.MarkFlag(getCmd, "fields", stability.Experimental, listShapeSince)
}

// shapeListOutput wraps p with the --limit/--fields shaping when either flag
// was given, and returns p untouched otherwise so the default output stays
// byte-identical. format is the effective output format and tabular selects
// the flattened column form (see output.ShapeOptions).
func shapeListOutput(p output.Printer, format string, tabular bool) output.Printer {
	fields := output.ParseFields(getListFields)
	limit := getListLimit
	if agentMode && usesDefaultGetLimit(invokedGetCmd) {
		limit = agentDefaultPage
	}
	if limit == 0 && len(fields) == 0 {
		return p
	}
	return output.NewShapingPrinter(p, output.ShapeOptions{
		Limit:   limit,
		Fields:  fields,
		Tabular: tabular,
		Format:  format,
		Notices: os.Stderr,
	})
}

// usesDefaultGetLimit reports whether c is a get subcommand whose --limit is
// the get-wide flag and was not given. A subcommand with its own --limit
// pages server-side (agentPageLimit) and must not be cut again here.
func usesDefaultGetLimit(c *cobra.Command) bool {
	if c == nil {
		return false
	}
	f := c.Flags().Lookup("limit")
	return f != nil && f == getCmd.PersistentFlags().Lookup("limit") && !f.Changed
}
