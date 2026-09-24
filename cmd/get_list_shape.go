package cmd

import (
	"os"

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

func init() {
	getCmd.PersistentFlags().IntVar(&getListLimit, "limit", 0,
		"return at most this many items (0 = all); applied after fetching, so --chunk-size still controls paging")
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
	if getListLimit == 0 && len(fields) == 0 {
		return p
	}
	return output.NewShapingPrinter(p, output.ShapeOptions{
		Limit:   getListLimit,
		Fields:  fields,
		Tabular: tabular,
		Format:  format,
		Notices: os.Stderr,
	})
}
