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

	// Metadata options
	MetadataFields []string // Metadata fields to include; nil/empty = disabled, ["all"] = all fields, specific names = filtered

	// Segment options
	Segments []FilterSegmentRef // Filter segments to apply to the query

	// ClientContext is an optional caller-supplied semantic string included as the "context"
	// field in the dt-client-context request header (e.g. "root-cause-analysis").
	ClientContext string
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

	return req
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
	return e.printResults(result, opts)
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

	result, err := handler.ExecuteAndPoll(ctx, req, onUnauthorized)
	if err != nil {
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

// getHintForNotification returns a CLI hint for a given notification type or message
func getHintForNotification(notificationType, message string) string {
	hints := map[string]string{
		"SCAN_LIMIT_GBYTES":       "Use --default-scan-limit-gbytes <value> to increase the limit (e.g., dtctl q '<query>' --default-scan-limit-gbytes 2000)",
		"RESULT_LIMIT_RECORDS":    "Use --max-result-records <value> to increase the record limit",
		"RESULT_LIMIT_BYTES":      "Use --max-result-bytes <value> to increase the result size limit",
		"FETCH_TIMEOUT":           "Use --fetch-timeout-seconds <value> to increase the fetch timeout",
		"SAMPLING_APPLIED":        "Use --default-sampling-ratio <value> to adjust sampling (1.0 = no sampling)",
		"QUERY_CONSUMPTION_LIMIT": "Use --enforce-query-consumption-limit=false to disable consumption limits",
	}

	if hint, ok := hints[notificationType]; ok {
		return hint
	}

	if len(message) > 0 {
		msgLower := strings.ToLower(message)
		if strings.Contains(msgLower, "result has been limited") || strings.Contains(msgLower, "limited to") {
			return "Use --max-result-records <value> to increase the record limit (e.g., dtctl q '<query>' --max-result-records 5000)"
		}
		if strings.Contains(msgLower, "scan") && strings.Contains(msgLower, "gigabyte") {
			return "Use --default-scan-limit-gbytes <value> to increase the limit (e.g., dtctl q '<query>' --default-scan-limit-gbytes 2000)"
		}
	}

	return ""
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
func (e *DQLExecutor) printResults(result *DQLQueryResponse, opts DQLExecuteOptions) error {
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

		switch opts.OutputFormat {
		case "", "table", "wide", "csv":
			records = output.SummarizeSnapshotForTable(records)
		}
	}

	// Extract metadata if requested
	var meta *output.QueryMetadata
	if len(opts.MetadataFields) > 0 {
		meta = extractQueryMetadata(result)
	}

	printer := output.NewPrinterWithOpts(output.PrinterOptions{
		Format:     opts.OutputFormat,
		Width:      opts.Width,
		Height:     opts.Height,
		Fullscreen: opts.Fullscreen,
	})

	switch opts.OutputFormat {
	case "table", "wide":
		var err error
		if opts.OutputFormat == "table" {
			err = e.printTable(records)
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

// extractQueryMetadata converts DQL response metadata to the output-layer QueryMetadata type.
func extractQueryMetadata(result *DQLQueryResponse) *output.QueryMetadata {
	g := result.GetMetadata()
	if g == nil {
		return nil
	}

	meta := &output.QueryMetadata{
		ExecutionTimeMilliseconds: g.ExecutionTimeMilliseconds,
		ScannedRecords:            g.ScannedRecords,
		ScannedBytes:              g.ScannedBytes,
		ScannedDataPoints:         g.ScannedDataPoints,
		Sampled:                   g.Sampled,
		QueryID:                   g.QueryID,
		DQLVersion:                g.DQLVersion,
		Query:                     g.Query,
		CanonicalQuery:            g.CanonicalQuery,
		Timezone:                  g.Timezone,
		Locale:                    g.Locale,
	}

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

// printTable prints query results as a table
func (e *DQLExecutor) printTable(records []map[string]interface{}) error {
	if len(records) == 0 {
		return nil
	}

	data, err := json.Marshal(records)
	if err != nil {
		return err
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		return err
	}

	printer := output.NewPrinter("table")
	return printer.PrintList(results)
}
