package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/profile"
)

var execProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Run a code-level profiling analysis on a Dynatrace entity",
	Long: `Run a code-level profiling analysis via the Dynatrace code-level analysis API.

The analysis is asynchronous server-side; this command polls until it completes.

Kinds:
  hotspots        Method hotspots for a SERVICE entity
  threads         Thread analysis for a SERVICE entity
  memory          Memory allocation analysis for a PROCESS_GROUP or PGI entity
  memory-details  Drill-down into a specific type/method (requires --type and --method)

Examples:
  # Method hotspots for the last hour
  dtctl exec profile --kind hotspots --entity SERVICE-ABC123 --last 1h

  # Thread analysis with a 30-minute window
  dtctl exec profile --kind threads --entity SERVICE-ABC123 --last 30m

  # Memory allocation, survivors only
  dtctl exec profile --kind memory --entity PROCESS_GROUP-DEF456 --last 1h --survivors-only

  # Memory drill-down
  dtctl exec profile --kind memory-details --entity PROCESS_GROUP-DEF456 --last 1h \
    --type java.lang.String --method "java.lang.String.intern()"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kindShort, _ := cmd.Flags().GetString("kind")
		entityID, _ := cmd.Flags().GetString("entity")
		lastStr, _ := cmd.Flags().GetString("last")
		fromStr, _ := cmd.Flags().GetString("from")
		toStr, _ := cmd.Flags().GetString("to")

		apiKind, ok := profile.Kind[kindShort]
		if !ok {
			return fmt.Errorf("unknown kind %q: use hotspots, threads, memory, or memory-details", kindShort)
		}
		if entityID == "" {
			return fmt.Errorf("--entity is required")
		}

		var from, to int64
		if lastStr != "" {
			last, err := time.ParseDuration(lastStr)
			if err != nil {
				return fmt.Errorf("invalid --last value %q: %w", lastStr, err)
			}
			now := time.Now()
			from, to = now.Add(-last).UnixMilli(), now.UnixMilli()
		} else {
			if fromStr == "" || toStr == "" {
				return fmt.Errorf("provide --last <duration> or both --from and --to")
			}
			var err error
			from, err = parseProfileTimestamp(fromStr)
			if err != nil {
				return err
			}
			to, err = parseProfileTimestamp(toStr)
			if err != nil {
				return err
			}
		}

		serviceFilter, _ := cmd.Flags().GetString("service-filter")
		showWaiting, _ := cmd.Flags().GetBool("show-waiting")
		problemID, _ := cmd.Flags().GetString("problem-id")
		survivorsOnly, _ := cmd.Flags().GetBool("survivors-only")
		typeFilter, _ := cmd.Flags().GetString("type-filter")
		apiFilter, _ := cmd.Flags().GetString("api-filter")
		methodFQNFilter, _ := cmd.Flags().GetString("method-fqn-filter")
		allocType, _ := cmd.Flags().GetString("type")
		method, _ := cmd.Flags().GetString("method")

		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := profile.NewHandler(c)
		stopSpinner := startSpinner(fmt.Sprintf("running %s analysis", kindShort))
		resp, err := handler.Run(cmd.Context(), profile.Payload{
			Kind:            apiKind,
			EntityID:        entityID,
			From:            from,
			To:              to,
			ServiceFilter:   serviceFilter,
			ShowWaiting:     showWaiting,
			ProblemID:       problemID,
			SurvivorsOnly:   survivorsOnly,
			TypeFilter:      typeFilter,
			APIFilter:       apiFilter,
			MethodFQNFilter: methodFQNFilter,
			Type:            allocType,
			Method:          method,
		})
		stopSpinner()
		if err != nil {
			return err
		}

		// Marshal resp to a generic map so enrich/compact helpers see the
		// expected envelope shape: {"status":..., "result":{...}, ...}
		b, _ := json.Marshal(resp)
		var result interface{}
		_ = json.Unmarshal(b, &result)
		full, _ := cmd.Flags().GetBool("full")
		if !full {
			result = compactResult(result)
		}
		result = profile.EnrichResult(apiKind, result)

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "" {
			outputFormat = "json"
		}
		if outputFormat == "stacktree" {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil || w <= 0 {
				w = 120
			}
			depth, _ := cmd.Flags().GetInt("depth")
			appOnly, _ := cmd.Flags().GetBool("app-only")
			abbrev, _ := cmd.Flags().GetBool("abbrev")
			if s := profile.ToStackTree(apiKind, result, w, depth, appOnly, abbrev); s != "" {
				fmt.Print(s)
				return nil
			}
			outputFormat = "json"
		}
		if outputFormat == "tree" {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil || w <= 0 {
				w = 120
			}
			if s := profile.ToTree(apiKind, result, w); s != "" {
				fmt.Print(s)
				return nil
			}
			outputFormat = "json"
		}
		if outputFormat == "bars" {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil || w <= 0 {
				w = 120
			}
			top, _ := cmd.Flags().GetInt("top")
			if s := profile.ToBars(apiKind, result, w, top); s != "" {
				fmt.Print(s)
				return nil
			}
			outputFormat = "json"
		}
		if outputFormat == "flamegraph" {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil || w <= 0 {
				w = 120
			}
			if fg := profile.ToFlamegraph(apiKind, result, w); fg != "" {
				fmt.Print(fg)
				return nil
			}
			outputFormat = "json"
		}
		if outputFormat == "table" {
			if rows := profile.ToTableRows(apiKind, result); rows != nil {
				top, _ := cmd.Flags().GetInt("top")
				return output.NewPrinter("table").PrintList(profile.LimitRows(rows, top))
			}
			outputFormat = "json"
		}
		return output.NewPrinter(outputFormat).Print(result)
	},
}

func parseProfileTimestamp(s string) (int64, error) {
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ms, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("cannot parse time %q: use RFC3339 or epoch millis", s)
	}
	return t.UnixMilli(), nil
}

func init() {
	execProfileCmd.Flags().StringP("kind", "k", "", "analysis kind: hotspots, threads, memory, memory-details (required)")
	execProfileCmd.Flags().StringP("entity", "e", "", "entity ID (SERVICE-xxx or PROCESS_GROUP-xxx) (required)")
	execProfileCmd.Flags().String("last", "", "time window relative to now, e.g. 1h, 30m (max 2h)")
	execProfileCmd.Flags().String("from", "", "window start — RFC3339 or epoch millis")
	execProfileCmd.Flags().String("to", "", "window end — RFC3339 or epoch millis")

	// hotspots / threads only
	execProfileCmd.Flags().String("service-filter", "", "service call filter (hotspots/threads)")
	execProfileCmd.Flags().Bool("show-waiting", false, "include waiting (non-running) samples (hotspots/threads)")

	// memory* only
	execProfileCmd.Flags().String("problem-id", "", "scope to a problem ID (memory*)")
	execProfileCmd.Flags().Bool("survivors-only", false, "restrict to surviving allocations (memory*)")
	execProfileCmd.Flags().String("type-filter", "", "`;`-separated allocated-type filter (memory*)")
	execProfileCmd.Flags().String("api-filter", "", "`;`-separated API filter (memory*)")
	execProfileCmd.Flags().String("method-fqn-filter", "", "`;`-separated method FQN filter (memory*)")

	// memory-details only
	execProfileCmd.Flags().String("type", "", "allocated type to drill into (memory-details)")
	execProfileCmd.Flags().String("method", "", "method to drill into (memory-details)")

	execProfileCmd.Flags().Bool("full", false, "include raw timeseries dataPoints (stripped by default)")
	execProfileCmd.Flags().Int("top", 0, "limit table output to top N rows by running samples (0 = all)")
	execProfileCmd.Flags().Int("depth", 0, "limit stacktree output to N levels deep (0 = all)")
	execProfileCmd.Flags().Bool("app-only", false, "stacktree: show only com.dynatrace.* frames")
	execProfileCmd.Flags().Bool("abbrev", false, "stacktree: abbreviate package segments to first letter (e.g. com.example → c.e)")

	_ = execProfileCmd.MarkFlagRequired("kind")
	_ = execProfileCmd.MarkFlagRequired("entity")
}

// startSpinner writes an animated spinner to stderr while work is in progress.
// Call the returned function to stop it and clear the line.
// Skipped in plain/agent mode (non-TTY) to keep output machine-readable.
func startSpinner(label string) func() {
	if agentMode || plainMode || !output.ColorEnabled() {
		return func() {}
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r\033[K") // clear line
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], label)
				i++
			}
		}
	}()
	return func() { close(done) }
}

// visualOnlyKeys are fields that carry UI rendering metadata with no analytical value.
var visualOnlyKeys = map[string]bool{
	"color":    true, // hex colour for charts
	"uuid":     true, // internal render ID
	"fileName": true, // usually null in profiling results
	"filePath": true, // usually null in profiling results
}

// compactResult strips timeseries dataPoints and visual-only fields so agents
// receive signal without chart/render noise.
func compactResult(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, child := range val {
			if visualOnlyKeys[k] {
				continue
			}
			if k == "dataPoints" {
				if arr, ok := child.([]interface{}); ok {
					out["dataPointsCount"] = len(arr)
					continue
				}
			}
			out[k] = compactResult(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = compactResult(item)
		}
		return out
	}
	return v
}
