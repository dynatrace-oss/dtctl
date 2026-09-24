package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// querySeriesSince is the release that introduced --series and --precision.
const querySeriesSince = "0.40.0"

// maxQueryPrecision is the largest --precision accepted: a float64 carries
// about 17 significant digits, so anything above that rounds nothing.
const maxQueryPrecision = 17

// resolveSeriesOptions validates --series and --precision against the output
// format. A summary replaces the arrays the chart formats plot, so the two
// cannot be combined; downsampling keeps a plottable series and can.
func resolveSeriesOptions(series string, precision int, format string) (output.SeriesMode, error) {
	mode, err := output.ParseSeriesMode(series)
	if err != nil {
		return output.SeriesMode{}, err
	}
	if precision < 0 || precision > maxQueryPrecision {
		return output.SeriesMode{}, fmt.Errorf("invalid --precision %d: use 1-%d significant digits, or 0 to keep full precision", precision, maxQueryPrecision)
	}
	if mode.Kind == output.SeriesSummary {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "chart", "sparkline", "spark", "barchart", "bar", "braille", "br":
			return output.SeriesMode{}, fmt.Errorf("--series=summary cannot be combined with -o %s, which plots the full series (use --series=downsample:N to reduce it instead)", format)
		}
	}
	return mode, nil
}

func init() {
	queryCmd.Flags().String("series", "full", `how to render timeseries arrays (records with timeframe and interval):
full = every datapoint (default); summary = per-series min/avg/max/p95/last/n,
peak/trough times, a sparkline and a level-shift hint; downsample:N = at most
N points per series, keeping each bucket's min and max so extremes survive`)
	queryCmd.Flags().Int("precision", 0, "round numbers in the result to N significant digits, never into the integer part (0 = full precision; --series=summary defaults to 3)")

	_ = queryCmd.RegisterFlagCompletionFunc("series", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"full\tevery datapoint (default)",
			"summary\tper-series statistics and a sparkline",
			"downsample:60\tat most 60 points per series, extremes kept",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// Both change the numbers a caller receives; they ship experimental on the
	// stable query command until the summary shape has settled.
	stability.MarkFlag(queryCmd, "series", stability.Experimental, querySeriesSince)
	stability.MarkFlag(queryCmd, "precision", stability.Experimental, querySeriesSince)
}
