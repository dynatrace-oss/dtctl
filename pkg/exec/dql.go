package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/aidetect"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/version"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing callers continue to compile.
type (
	FilterSegmentRef      = sdkquery.FilterSegmentRef
	FilterSegmentVariable = sdkquery.FilterSegmentVariable
	DQLQueryRequest       = sdkquery.ExecuteRequest
	DQLQueryResponse      = sdkquery.Response
	DQLResult             = sdkquery.Result
	DQLMetadata           = sdkquery.Metadata
	GrailMetadata         = sdkquery.GrailMetadata
	Contributions         = sdkquery.Contributions
	BucketContribution    = sdkquery.BucketContribution
	QueryNotification     = sdkquery.Notification
	AnalysisTimeframe     = sdkquery.AnalysisTimeframe
	MetricInfo            = sdkquery.MetricInfo
	DQLVerifyRequest      = sdkquery.VerifyRequest
	DQLVerifyResponse     = sdkquery.VerifyResponse
	MetadataNotification  = sdkquery.VerifyNotification
	SyntaxPosition        = sdkquery.SyntaxPosition
	Position              = sdkquery.Position
	QueryError            = sdkquery.QueryError
)

// DQLExecutor handles DQL query execution
type DQLExecutor struct {
	client         *client.Client
	sdk            *sdkquery.Handler
	tokenRefresher func() (string, error)
}

// NewDQLExecutor creates a new DQL executor
func NewDQLExecutor(c *client.Client) *DQLExecutor {
	sdk := sdkquery.NewHandler(httpclient.Wrap(c.HTTP()))
	return &DQLExecutor{client: c, sdk: sdk}
}

// WithTokenRefresher sets an optional callback that is invoked when a poll request
// receives a 401 Unauthorized response (e.g. because the OAuth token expired during
// a long-running query). The callback must return a fresh access token. The executor
// updates the underlying HTTP client with the new token and retries the poll.
func (e *DQLExecutor) WithTokenRefresher(refresher func() (string, error)) *DQLExecutor {
	e.tokenRefresher = refresher
	return e
}

// dtClientContextHeader builds the JSON value for the dt-client-context HTTP header.
// callerContext is the optional caller-supplied semantic string (empty = omit field).
func dtClientContextHeader(callerContext string) string {
	type payload struct {
		App     string `json:"app"`
		Version string `json:"version"`
		Agent   string `json:"agent,omitempty"`
		Context string `json:"context,omitempty"`
	}
	p := payload{App: "dtctl", Version: version.Version, Context: callerContext}
	if info := aidetect.Detect(); info.Detected {
		p.Agent = info.Name
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// sdkHandler returns the SDK handler with the dt-client-context header set.
func (e *DQLExecutor) sdkHandler(clientContext string) *sdkquery.Handler {
	return e.sdk.WithHeaders(map[string]string{
		"dt-client-context": dtClientContextHeader(clientContext),
	})
}

// DecodeMode controls snapshot payload decoding behavior.
type DecodeMode int

const (
	// DecodeNone disables snapshot decoding (default).
	DecodeNone DecodeMode = iota
	// DecodeSimplified decodes and simplifies variant wrappers to plain values.
	DecodeSimplified
	// DecodeFull decodes but preserves the full variant tree with type annotations.
	DecodeFull
)

// DQLExecuteOptions configures DQL query execution
type DQLExecuteOptions struct {
	// Output formatting options
	OutputFormat string
	JQFilter     string     // jq filter expression applied before rendering
	AgentMode    bool       // Enable agent mode (e.g. for Dynatrace API)
	Decode       DecodeMode // Snapshot payload decoding mode
	Width        int        // Chart width (0 = default)
	Height       int        // Chart height (0 = default)
	Fullscreen   bool       // Use terminal dimensions for chart

	// Query limit options
	MaxResultRecords       int64   // Maximum number of result records (0 = use default)
	MaxResultBytes         int64   // Maximum result size in bytes (0 = use default)
	DefaultScanLimitGbytes float64 // Scan limit in gigabytes (0 = use default)

	// Query execution options
	DefaultSamplingRatio         float64 // Sampling ratio (0 = use default, normalized to power of 10 <= 100000)
	FetchTimeoutSeconds          int32   // Time limit for fetching data in seconds (0 = use default)
	EnablePreview                bool    // Request preview results
	EnforceQueryConsumptionLimit bool    // Enforce query consumption limit
	IncludeTypes                 bool    // Include type information in results (default: true)
	IncludeContributions         bool    // Include bucket contribution information in results

	// Timeframe options
	DefaultTimeframeStart string // Query timeframe start timestamp (ISO-8601/RFC3339)
	DefaultTimeframeEnd   string // Query timeframe end timestamp (ISO-8601/RFC3339)

	// Localization options
	Locale   string // Query locale (e.g., "en_US")
	Timezone string // Query timezone (e.g., "UTC", "Europe/Paris")

	// ProgressMode controls the live progress indicator drawn on stderr while an
	// asynchronous query is polled: "auto" (default; on for interactive TTYs),
	// "always", or "never". It never affects stdout, so structured/piped output
	// is unchanged. Empty is treated as "auto".
	ProgressMode string

	// Metadata options
	MetadataFields []string // Metadata fields to include; nil/empty = disabled, ["all"] = all fields, specific names = filtered

	// Segment options
	Segments []FilterSegmentRef // Filter segments to apply to the query

	// ClientContext is an optional caller-supplied semantic string included as the "context"
	// field in the dt-client-context request header (e.g. "root-cause-analysis").
	ClientContext string

	// Spill holds the fully-resolved result-spill settings. When Spill.Enabled()
	// is false (mode never) the spill path is bypassed entirely and output is
	// unchanged from today's behaviour.
	Spill SpillOptions

	// TenantID and ContextName are provenance recorded in the spill manifest and
	// used to partition the spill directory by context (D9). They are not part of
	// query execution and are only consulted on the spill path.
	TenantID    string
	ContextName string
}

// DQLVerifyOptions configures DQL query verification
type DQLVerifyOptions struct {
	GenerateCanonicalQuery bool   // Generate a canonical (normalized) version of the query
	Timezone               string // Query timezone (e.g., "UTC", "Europe/Paris")
	Locale                 string // Query locale (e.g., "en_US")
	ClientContext          string // Optional caller-supplied semantic string for the dt-client-context header
}

// buildExecuteRequest converts CLI options to an SDK ExecuteRequest.
func buildExecuteRequest(query string, opts DQLExecuteOptions) sdkquery.ExecuteRequest {
	req := sdkquery.ExecuteRequest{
		Query: query,
	}

	if opts.MaxResultRecords > 0 {
		req.MaxResultRecords = opts.MaxResultRecords
	}
	if opts.MaxResultBytes > 0 {
		req.MaxResultBytes = opts.MaxResultBytes
	}
	if opts.DefaultScanLimitGbytes > 0 {
		req.DefaultScanLimitGbytes = opts.DefaultScanLimitGbytes
	}
	if opts.DefaultSamplingRatio > 0 {
		req.DefaultSamplingRatio = opts.DefaultSamplingRatio
	}
	if opts.FetchTimeoutSeconds > 0 {
		req.FetchTimeoutSeconds = opts.FetchTimeoutSeconds
	}
	if opts.EnablePreview {
		req.EnablePreview = true
	}
	if opts.EnforceQueryConsumptionLimit {
		req.EnforceQueryConsumptionLimit = true
	}
	if opts.IncludeTypes {
		includeTypes := true
		req.IncludeTypes = &includeTypes
	}
	if opts.IncludeContributions {
		includeContributions := true
		req.IncludeContributions = &includeContributions
	}
	if opts.DefaultTimeframeStart != "" {
		req.DefaultTimeframeStart = opts.DefaultTimeframeStart
	}
	if opts.DefaultTimeframeEnd != "" {
		req.DefaultTimeframeEnd = opts.DefaultTimeframeEnd
	}
	if opts.Locale != "" {
		req.Locale = opts.Locale
	}
	if opts.Timezone != "" {
		req.Timezone = opts.Timezone
	}
	if len(opts.Segments) > 0 {
		req.FilterSegments = opts.Segments
	}
	// Request metric-catalogue enrichment (displayName/description/unit on
	// metadata.metrics[]) only when the caller actually wants metrics metadata.
	if wantsMetricsMetadata(opts.MetadataFields) {
		req.EnrichMetricMetadata = true
	}

	return req
}

// wantsMetricsMetadata reports whether the requested metadata fields include
// metric metadata — either explicitly ("metrics") or via the "all" selector.
func wantsMetricsMetadata(fields []string) bool {
	for _, f := range fields {
		if f == "all" || f == "metrics" {
			return true
		}
	}
	return false
}

// Execute executes a DQL query
func (e *DQLExecutor) Execute(query string, outputFormat string) error {
	return e.ExecuteWithOptions(query, DQLExecuteOptions{OutputFormat: outputFormat})
}

// ExecuteWithOptions executes a DQL query with full options
func (e *DQLExecutor) ExecuteWithOptions(query string, opts DQLExecuteOptions) error {
	return e.ExecuteWithContext(context.Background(), query, opts)
}

// ExecuteWithContext executes a DQL query with a cancellable context and prints the results.
func (e *DQLExecutor) ExecuteWithContext(ctx context.Context, query string, opts DQLExecuteOptions) error {
	result, err := e.ExecuteQueryWithContext(ctx, query, opts)
	if err != nil {
		return err
	}
	if result == nil {
		return nil // context was cancelled; message already printed to stderr
	}
	return e.printResults(query, result, opts)
}

// ExecuteQuery executes a DQL query and returns the raw result
func (e *DQLExecutor) ExecuteQuery(query string) (*DQLQueryResponse, error) {
	return e.ExecuteQueryWithOptions(query, DQLExecuteOptions{})
}

// ExecuteQueryWithOptions executes a DQL query with options and returns the raw result
func (e *DQLExecutor) ExecuteQueryWithOptions(query string, opts DQLExecuteOptions) (*DQLQueryResponse, error) {
	return e.ExecuteQueryWithContext(context.Background(), query, opts)
}

// ExecuteQueryWithContext executes a DQL query with a cancellable context.
// If ctx is cancelled while the query is polling, a best-effort cancel request is sent
// to the Grail backend before returning.
func (e *DQLExecutor) ExecuteQueryWithContext(ctx context.Context, query string, opts DQLExecuteOptions) (*DQLQueryResponse, error) {
	req := buildExecuteRequest(query, opts)
	handler := e.sdkHandler(opts.ClientContext)

	// Build the token refresher callback for the SDK. The SDK's ExecuteAndPoll will
	// call this on 401; we refresh the token and update the underlying HTTP client.
	var onUnauthorized func() (string, error)
	if e.tokenRefresher != nil {
		onUnauthorized = func() (string, error) {
			newToken, err := e.tokenRefresher()
			if err != nil {
				return "", err
			}
			e.client.SetToken(newToken)
			return newToken, nil
		}
	}

	// Live progress on stderr. The reporter is a no-op unless stderr is an
	// interactive TTY (or --progress=always), so piped/agent/structured output
	// is untouched. Stop() erases the line on success, error, and cancellation.
	progressMode := opts.ProgressMode
	if progressMode == "" {
		progressMode = output.ProgressAuto
	}
	reporter := output.NewProgressReporter(progressMode, opts.AgentMode)
	defer reporter.Stop()

	result, err := handler.ExecuteAndPollWithOptions(ctx, req, sdkquery.ExecuteAndPollOptions{
		OnUnauthorized: onUnauthorized,
		OnUpdate: func(u sdkquery.PollUpdate) {
			state := output.ProgressState{
				Progress:       u.Progress,
				ScannedBytes:   u.ScannedBytes,
				ScannedRecords: u.ScannedRecords,
			}
			if u.Preview != nil {
				state.PreviewRows = len(u.Preview.Records)
			}
			reporter.Update(state)
		},
	})
	if err != nil {
		// Clear the bar before any further stderr output (cancellation notice,
		// error hints). Stop is idempotent; the defer remains a safety net.
		reporter.Stop()
		// If context was cancelled, print cancellation message
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "\nQuery cancelled.")
			return nil, nil
		}
		// Enhance known error types with CLI-specific hints
		var qErr *QueryError
		if ok := isQueryError(err, &qErr); ok && qErr.ErrorType == "FILTER_SEGMENT_REQUIRES_VARIABLE" {
			return nil, formatSegmentVariableError(qErr)
		}
		return nil, err
	}

	// On success, replace the bar with a completion summary carrying the final
	// scan totals from the result metadata (the terminal poll does not fire an
	// OnUpdate, so the reporter's live state stops one poll short).
	final := output.ProgressState{Progress: 100}
	if m := result.GetMetadata(); m != nil {
		final.ScannedBytes = m.ScannedBytes
		final.ScannedRecords = m.ScannedRecords
	}
	reporter.Complete(final)

	return result, nil
}

// isQueryError extracts a *QueryError from the error chain.
func isQueryError(err error, target **QueryError) bool {
	return errors.As(err, target)
}

// formatSegmentVariableError produces a helpful error message when a segment
// requires variable bindings, including ready-to-use -S inline and --segments-file examples.
func formatSegmentVariableError(qErr *QueryError) error {
	args := qErr.Arguments
	segmentID := "unknown"
	variableName := "unknown"
	if len(args) >= 1 {
		segmentID = strings.Trim(args[0], "`")
	}
	if len(args) >= 3 {
		variableName = strings.TrimPrefix(args[2], "$")
	}

	return fmt.Errorf("segment %s requires variable %q\n\n"+
		"Bind the variable inline on -S using URL-query syntax:\n\n"+
		"  dtctl query \"...\" -S \"%s?%s=your-value-here\"\n\n"+
		"Or use --segments-file with a YAML file for complex cases:\n\n"+
		"  # segments.yaml\n"+
		"  - id: %s\n"+
		"    variables:\n"+
		"      - name: %s\n"+
		"        values: [\"your-value-here\"]\n\n"+
		"  dtctl query \"...\" --segments-file segments.yaml",
		segmentID, variableName, segmentID, variableName, segmentID, variableName)
}

// VerifyQuery verifies a DQL query without executing it
func (e *DQLExecutor) VerifyQuery(query string, opts DQLVerifyOptions) (*DQLVerifyResponse, error) {
	req := sdkquery.VerifyRequest{
		Query:                  query,
		GenerateCanonicalQuery: opts.GenerateCanonicalQuery,
		Timezone:               opts.Timezone,
		Locale:                 opts.Locale,
	}

	handler := e.sdkHandler(opts.ClientContext)

	// Create context with 30-second timeout (verify is fast)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return handler.Verify(ctx, req)
}

// Notification categories. A query notification is mapped to one of these so a
// single set of advice serves both the human stderr hint and the agent-envelope
// suggestions. The empty string means "no specific advice".
const (
	notifScanLimit   = "scan_limit"
	notifResultLimit = "result_limit"
	notifTimeout     = "timeout"
	notifSampling    = "sampling"
	notifConsumption = "consumption"
)

// classifyNotification maps a query notification (by type, falling back to a
// message pattern) to one of the notif* categories. The message fallback exists
// because not every deployment tags notifications with a stable type.
func classifyNotification(notificationType, message string) string {
	switch notificationType {
	case "SCAN_LIMIT_GBYTES":
		return notifScanLimit
	case "RESULT_LIMIT_RECORDS", "RESULT_LIMIT_BYTES":
		return notifResultLimit
	case "FETCH_TIMEOUT":
		return notifTimeout
	case "SAMPLING_APPLIED":
		return notifSampling
	case "QUERY_CONSUMPTION_LIMIT":
		return notifConsumption
	}

	msg := strings.ToLower(message)
	switch {
	case strings.Contains(msg, "scan") && strings.Contains(msg, "gigabyte"):
		return notifScanLimit
	case strings.Contains(msg, "result has been limited") || strings.Contains(msg, "limited to"):
		return notifResultLimit
	}
	return ""
}

// getHintForNotification returns a concise CLI hint (a single line, for stderr)
// for a given notification type or message. For a truncating notification it
// leads with data-reduction advice — raising the limit is the expensive last
// resort, not the first suggestion.
func getHintForNotification(notificationType, message string) string {
	switch classifyNotification(notificationType, message) {
	case notifScanLimit:
		return "Result is PARTIAL (scan stopped early). Reduce data scanned — narrow the timeframe, restrict to specific buckets (bucket:{...}; use --include-contributions to find the heavy ones), filter early with == (not contains()), or sample logs/spans (add samplingRatio:1000 to the fetch, or pass --default-sampling-ratio). Raising --default-scan-limit-gbytes <value> works but costs more."
	case notifResultLimit:
		return "Result was truncated. Aggregate in DQL (| summarize ...) to shrink it, or raise --max-result-records / --max-result-bytes <value>."
	case notifTimeout:
		return "Fetch timed out (result may be incomplete). Narrow the timeframe or add filters, or raise --fetch-timeout-seconds <value>."
	case notifSampling:
		return "Results are sampled/extrapolated (approximate). For exact values remove samplingRatio from the fetch, or pass --default-sampling-ratio 1 (higher scan cost)."
	case notifConsumption:
		return "Query hit a consumption limit. Reduce the data scanned, or pass --enforce-query-consumption-limit=false to bypass."
	}
	return ""
}

// agentAdviceForNotification returns agent-envelope suggestion lines (the "# ..."
// style used across the spill envelope) for a notification. Unlike the one-line
// human hint, it spells out the concrete alternatives an agent can act on and
// leads with the fact that the result may be incomplete.
func agentAdviceForNotification(notificationType, message string) []string {
	switch classifyNotification(notificationType, message) {
	case notifScanLimit:
		return []string{
			"# the scan limit was reached — this result is PARTIAL (the scan stopped early), not the full dataset",
			"# reduce the DATA SCANNED (aggregation alone does not help — it still scans everything). Raising the limit works but costs more and is slower:",
			"#   - narrow the timeframe, e.g. from:now()-1h instead of a wide range (usually the biggest lever)",
			"#   - restrict to the relevant bucket(s): fetch logs, bucket:{\"<bucket>\"}",
			"#   - find which bucket dominates the scan: dtctl query --include-contributions --metadata=contributions -o json '<query>' and read matchedRecordsRatio per bucket",
			"#   - filter early and prefer equality, e.g. | filter <field> == \"...\" (== scans far less than contains()/matchesPhrase())",
			"#   - for logs/spans, sample so only a fraction is scanned: add samplingRatio to the fetch, e.g. fetch logs, samplingRatio:1000 (DQL-native, portable) — or pass dtctl's --default-sampling-ratio 1000; extrapolate counts with sum(dt.system.sampling_ratio)",
			"# only if you truly need the full scan: --default-scan-limit-gbytes <value> (higher cost & duration; -1 = unlimited)",
		}
	case notifResultLimit:
		return []string{
			"# the result was truncated to the record/byte limit — rows are missing",
			"# aggregate in DQL (| summarize ...) to shrink the result, or raise the cap with --max-result-records <N> / --max-result-bytes <N>",
		}
	case notifTimeout:
		return []string{
			"# the fetch timed out — this result may be incomplete",
			"# narrow the timeframe or add filters to speed the query, or raise --fetch-timeout-seconds <N>",
		}
	case notifSampling:
		return []string{
			"# results are sampled/extrapolated (approximate, not exact); for exact values remove samplingRatio from the fetch (or pass --default-sampling-ratio 1) — higher scan cost",
		}
	case notifConsumption:
		return []string{
			"# the query hit a consumption limit and was stopped — this result may be incomplete",
			"# reduce the data scanned (narrower timeframe/filters), or pass --enforce-query-consumption-limit=false to bypass",
		}
	}
	return nil
}

// notificationAdvice converts query notifications into agent-envelope warnings
// and suggestions. Unlike PrintNotifications (which writes to stderr for a
// human), this surfaces the same information inside the JSON envelope on stdout,
// so an agent parsing the result on stdout learns the result may be PARTIAL and
// what to do next — otherwise a truncated scan looks like a complete answer.
func notificationAdvice(notifications []QueryNotification) (warnings, suggestions []string) {
	for _, n := range notifications {
		switch strings.ToUpper(n.Severity) {
		case "WARNING", "WARN", "ERROR":
		default:
			continue
		}
		if n.Message != "" {
			warnings = append(warnings, n.Message)
		}
		suggestions = append(suggestions, agentAdviceForNotification(n.NotificationType, n.Message)...)
	}
	return warnings, suggestions
}

// PrintNotifications prints query notifications/warnings to stderr
func (e *DQLExecutor) PrintNotifications(notifications []QueryNotification) {
	for _, n := range notifications {
		severity := n.Severity
		if severity == "" {
			severity = "INFO"
		}
		if severity == "WARNING" || severity == "WARN" {
			output.PrintWarning("%s", n.Message)
			if hint := getHintForNotification(n.NotificationType, n.Message); hint != "" {
				output.PrintHint("%s", hint)
			}
		} else if severity == "ERROR" {
			output.PrintHumanError("%s", n.Message)
			if hint := getHintForNotification(n.NotificationType, n.Message); hint != "" {
				output.PrintHint("%s", hint)
			}
		}
	}
}

// printResults prints the query results with the given options
func (e *DQLExecutor) printResults(query string, result *DQLQueryResponse, opts DQLExecuteOptions) error {
	effectiveFormat := opts.OutputFormat
	if opts.JQFilter != "" {
		effectiveFormat = output.NormalizeJQOutputFormat(effectiveFormat)
	}

	// Print any notifications/warnings first
	if notifications := result.GetNotifications(); len(notifications) > 0 {
		e.PrintNotifications(notifications)
	}

	// Extract records from result
	records := result.GetRecords()

	// Apply snapshot decoding if requested
	if opts.Decode != DecodeNone && len(records) > 0 {
		simplify := opts.Decode == DecodeSimplified
		records = output.DecodeSnapshotRecords(records, simplify)

		switch effectiveFormat {
		case "", "table", "wide", "csv":
			records = output.SummarizeSnapshotForTable(records)
		}
	}

	// Spill path (D2/D3/D19-buffered): when spilling is enabled, a large result
	// is written to disk and a compact summary is emitted in place of the rows.
	// This is strictly additive — when it decides "inline" (or spilling is
	// disabled) it falls through to the unchanged output path below. Note: a
	// small/empty result under `auto` decides inline via the threshold, while
	// `always`/`--spill-to` honours the "write the file" contract even for an
	// empty result.
	//
	// Agent mode always enters this path even under --spill=never: the
	// spill-aware emitter is what produces the structured envelope, and a
	// `never` result decides inline via a self-describing kind:"records"
	// envelope (D2/D31) rather than reverting to a human table. The path still
	// falls through (handled=false) for an explicit non-JSON encoding or a --jq
	// transform, so an agent that asked for `-o toon`/`--jq` keeps that shape.
	if opts.Spill.Enabled() || opts.AgentMode {
		handled, err := e.trySpill(query, result, records, effectiveFormat, opts)
		if handled || err != nil {
			return err
		}
	}

	// Extract metadata if requested
	var meta *output.QueryMetadata
	if len(opts.MetadataFields) > 0 {
		meta = extractQueryMetadata(result)
	}

	printer := output.NewPrinterWithOpts(output.PrinterOptions{
		Format:     effectiveFormat,
		JQFilter:   opts.JQFilter,
		AgentMode:  opts.AgentMode,
		Width:      opts.Width,
		Height:     opts.Height,
		Fullscreen: opts.Fullscreen,
		Types:      columnTypeMappings(result),
	})

	switch effectiveFormat {
	case "table", "wide":
		var err error
		if effectiveFormat == "table" {
			err = printer.PrintList(records)
		} else {
			if len(records) == 0 {
				return nil
			}
			err = printer.PrintList(records)
		}
		if err != nil {
			return err
		}
		if meta != nil {
			fmt.Fprint(os.Stderr, output.FormatMetadataFooter(meta, opts.MetadataFields))
		}
		return nil

	case "csv":
		if len(records) == 0 {
			return nil
		}
		if meta != nil {
			fmt.Fprint(os.Stderr, output.FormatMetadataCSVComments(meta, opts.MetadataFields))
		}
		return printer.PrintList(records)

	case "jsonl":
		// An empty JSONL file (zero lines) is valid output, so skip on no records.
		if len(records) == 0 {
			return nil
		}
		return printer.PrintList(records)

	case "parquet":
		// Always emit a Parquet file, even for an empty result: a zero-byte file
		// is not valid Parquet. The printer writes a valid schema-bearing file
		// with zero rows.
		return printer.PrintList(records)

	case "chart", "sparkline", "spark", "barchart", "bar", "braille", "br":
		if meta != nil {
			output.PrintWarning("--metadata is not supported with chart output formats")
		}
		if len(records) > 0 {
			return printer.Print(map[string]interface{}{"records": records})
		}
		return printer.Print(result)

	default:
		out := make(map[string]interface{})
		if len(records) > 0 {
			out["records"] = records
		} else if result.Result != nil {
			out["records"] = result.Result.Records
		}
		if meta != nil {
			out["metadata"] = output.MetadataToMap(meta, opts.MetadataFields)
		}
		if len(out) > 0 {
			return printer.Print(out)
		}
		return printer.Print(result)
	}
}

// columnTypeMappings flattens the DQL per-column type info from the response
// (populated when includeTypes is set) into the output-layer representation used
// by the Parquet printer to build a schema. Returns nil when no type info is
// present. When multiple type groups disagree on a column, the first wins.
func columnTypeMappings(result *DQLQueryResponse) []output.ColumnTypeMapping {
	groups := result.GetTypes()
	if len(groups) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []output.ColumnTypeMapping
	for _, g := range groups {
		for name, ct := range g.Mappings {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, output.ColumnTypeMapping{Name: name, Type: ct.Type})
		}
	}
	return out
}

// extractQueryMetadata converts DQL response metadata to the output-layer QueryMetadata type.
// Grail metadata (execution stats, query text, ...) and Metrics (timeseries metric descriptors)
// are independent siblings of the response's metadata section, so either one being present is
// enough to produce a non-nil result — gating on Grail alone would silently drop Metrics for
// responses that populate metadata.metrics without metadata.grail.
func extractQueryMetadata(result *DQLQueryResponse) *output.QueryMetadata {
	g := result.GetMetadata()
	metrics := result.GetMetrics()
	if g == nil && len(metrics) == 0 {
		return nil
	}

	meta := &output.QueryMetadata{}

	if g != nil {
		meta.ExecutionTimeMilliseconds = g.ExecutionTimeMilliseconds
		meta.ScannedRecords = g.ScannedRecords
		meta.ScannedBytes = g.ScannedBytes
		meta.ScannedDataPoints = g.ScannedDataPoints
		meta.Sampled = g.Sampled
		meta.QueryID = g.QueryID
		meta.DQLVersion = g.DQLVersion
		meta.Query = g.Query
		meta.CanonicalQuery = g.CanonicalQuery
		meta.Timezone = g.Timezone
		meta.Locale = g.Locale

		if g.AnalysisTimeframe != nil {
			meta.AnalysisTimeframe = &output.MetadataTimeframe{
				Start: g.AnalysisTimeframe.Start,
				End:   g.AnalysisTimeframe.End,
			}
		}

		if g.Contributions != nil && len(g.Contributions.Buckets) > 0 {
			contribs := &output.MetadataContribs{}
			for _, b := range g.Contributions.Buckets {
				contribs.Buckets = append(contribs.Buckets, output.MetadataBucket{
					Name:                b.Name,
					Table:               b.Table,
					ScannedBytes:        b.ScannedBytes,
					MatchedRecordsRatio: b.MatchedRecordsRatio,
				})
			}
			meta.Contributions = contribs
		}
	}

	for _, m := range metrics {
		meta.Metrics = append(meta.Metrics, output.MetricInfo{
			MetricKey:   m.MetricKey,
			FieldName:   m.FieldName,
			Aggregation: m.Aggregation,
			DisplayName: m.DisplayName,
			Description: m.Description,
			Unit:        m.Unit,
		})
	}

	return meta
}

// CancelQuery sends a best-effort cancellation request for a running query.
// Errors are written to stderr but not returned — cancellation is best-effort.
func (e *DQLExecutor) CancelQuery(requestToken string) {
	if requestToken == "" {
		return
	}
	handler := e.sdkHandler("")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := handler.Cancel(ctx, requestToken); err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to cancel query: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, "\nQuery cancelled.")
}

// ExecuteFromFile executes a DQL query from a file
func (e *DQLExecutor) ExecuteFromFile(filename string, outputFormat string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return e.Execute(string(data), outputFormat)
}
