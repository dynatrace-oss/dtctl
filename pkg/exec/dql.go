package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// DQLExecutor handles DQL query execution
type DQLExecutor struct {
	client *client.Client
}

// NewDQLExecutor creates a new DQL executor
func NewDQLExecutor(c *client.Client) *DQLExecutor {
	return &DQLExecutor{client: c}
}

// DQLExecuteOptions configures DQL query execution
type DQLExecuteOptions struct {
	// Output formatting options
	OutputFormat string
	Width        int  // Chart width (0 = default)
	Height       int  // Chart height (0 = default)
	Fullscreen   bool // Use terminal dimensions for chart

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

	// Timeframe options
	DefaultTimeframeStart string // Query timeframe start timestamp (ISO-8601/RFC3339)
	DefaultTimeframeEnd   string // Query timeframe end timestamp (ISO-8601/RFC3339)

	// Localization options
	Locale   string // Query locale (e.g., "en_US")
	Timezone string // Query timezone (e.g., "UTC", "Europe/Paris")
}

// DQLVerifyOptions configures DQL query verification
type DQLVerifyOptions struct {
	GenerateCanonicalQuery bool   // Generate a canonical (normalized) version of the query
	Timezone               string // Query timezone (e.g., "UTC", "Europe/Paris")
	Locale                 string // Query locale (e.g., "en_US")
}

// DQLQueryRequest represents a DQL query request
type DQLQueryRequest struct {
	Query                        string  `json:"query"`
	RequestTimeoutMilliseconds   int64   `json:"requestTimeoutMilliseconds,omitempty"`
	MaxResultRecords             int64   `json:"maxResultRecords,omitempty"`
	MaxResultBytes               int64   `json:"maxResultBytes,omitempty"`
	DefaultScanLimitGbytes       float64 `json:"defaultScanLimitGbytes,omitempty"`
	DefaultSamplingRatio         float64 `json:"defaultSamplingRatio,omitempty"`
	FetchTimeoutSeconds          int32   `json:"fetchTimeoutSeconds,omitempty"`
	EnablePreview                bool    `json:"enablePreview,omitempty"`
	EnforceQueryConsumptionLimit bool    `json:"enforceQueryConsumptionLimit,omitempty"`
	IncludeTypes                 *bool   `json:"includeTypes,omitempty"` // Pointer to distinguish between unset and false
	DefaultTimeframeStart        string  `json:"defaultTimeframeStart,omitempty"`
	DefaultTimeframeEnd          string  `json:"defaultTimeframeEnd,omitempty"`
	Locale                       string  `json:"locale,omitempty"`
	Timezone                     string  `json:"timezone,omitempty"`
}

// DQLQueryResponse represents a DQL query response
type DQLQueryResponse struct {
	State        string                   `json:"state"`
	RequestToken string                   `json:"requestToken,omitempty"`
	Result       *DQLResult               `json:"result,omitempty"`
	Records      []map[string]interface{} `json:"records,omitempty"` // For backward compatibility
	Progress     int                      `json:"progress,omitempty"`
	Metadata     *DQLMetadata             `json:"metadata,omitempty"`
}

// DQLResult represents the result section of a DQL response
type DQLResult struct {
	Records  []map[string]interface{} `json:"records"`
	Metadata *DQLMetadata             `json:"metadata,omitempty"` // Metadata can appear here too
}

// DQLMetadata represents the metadata section of a DQL response
type DQLMetadata struct {
	Grail *GrailMetadata `json:"grail,omitempty"`
}

// GrailMetadata represents Grail-specific metadata
type GrailMetadata struct {
	Query                     string              `json:"query,omitempty"`
	CanonicalQuery            string              `json:"canonicalQuery,omitempty"`
	QueryID                   string              `json:"queryId,omitempty"`
	DQLVersion                string              `json:"dqlVersion,omitempty"`
	Timezone                  string              `json:"timezone,omitempty"`
	Locale                    string              `json:"locale,omitempty"`
	ExecutionTimeMilliseconds int64               `json:"executionTimeMilliseconds,omitempty"`
	ScannedRecords            int64               `json:"scannedRecords,omitempty"`
	ScannedBytes              int64               `json:"scannedBytes,omitempty"`
	ScannedDataPoints         int64               `json:"scannedDataPoints,omitempty"`
	Sampled                   bool                `json:"sampled,omitempty"`
	Notifications             []QueryNotification `json:"notifications,omitempty"`
	AnalysisTimeframe         *AnalysisTimeframe  `json:"analysisTimeframe,omitempty"`
}

// QueryNotification represents a notification/warning from query execution
type QueryNotification struct {
	Severity         string   `json:"severity,omitempty"`         // INFO, WARNING, ERROR
	NotificationType string   `json:"notificationType,omitempty"` // e.g., SCAN_LIMIT_GBYTES
	Message          string   `json:"message,omitempty"`
	MessageFormat    string   `json:"messageFormat,omitempty"`
	Arguments        []string `json:"arguments,omitempty"`
}

// AnalysisTimeframe represents the timeframe analyzed by the query
type AnalysisTimeframe struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// DQLVerifyRequest represents a DQL query verification request
type DQLVerifyRequest struct {
	Query                  string `json:"query"`
	GenerateCanonicalQuery bool   `json:"generateCanonicalQuery,omitempty"`
	Timezone               string `json:"timezone,omitempty"`
	Locale                 string `json:"locale,omitempty"`
}

// DQLVerifyResponse represents a DQL query verification response
type DQLVerifyResponse struct {
	Valid          bool                   `json:"valid"`
	CanonicalQuery string                 `json:"canonicalQuery,omitempty"`
	Notifications  []MetadataNotification `json:"notifications,omitempty"`
}

// MetadataNotification represents a notification from query verification or execution
type MetadataNotification struct {
	Severity         string          `json:"severity"`
	NotificationType string          `json:"notificationType"`
	Message          string          `json:"message"`
	SyntaxPosition   *SyntaxPosition `json:"syntaxPosition,omitempty"`
}

// SyntaxPosition represents the position of a syntax issue in a query
type SyntaxPosition struct {
	Start *Position `json:"start,omitempty"`
	End   *Position `json:"end,omitempty"`
}

// Position represents a line and column position in text
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Execute executes a DQL query
func (e *DQLExecutor) Execute(query string, outputFormat string) error {
	return e.ExecuteWithOptions(query, DQLExecuteOptions{OutputFormat: outputFormat})
}

// ExecuteWithOptions executes a DQL query with full options
func (e *DQLExecutor) ExecuteWithOptions(query string, opts DQLExecuteOptions) error {
	result, err := e.ExecuteQueryWithOptions(query, opts)
	if err != nil {
		return err
	}
	return e.printResults(result, opts)
}

// ExecuteQuery executes a DQL query and returns the raw result
func (e *DQLExecutor) ExecuteQuery(query string) (*DQLQueryResponse, error) {
	return e.ExecuteQueryWithOptions(query, DQLExecuteOptions{})
}

// ExecuteQueryWithOptions executes a DQL query with options and returns the raw result
func (e *DQLExecutor) ExecuteQueryWithOptions(query string, opts DQLExecuteOptions) (*DQLQueryResponse, error) {
	req := DQLQueryRequest{
		Query:                      query,
		RequestTimeoutMilliseconds: 60000, // Wait up to 60 seconds for results
	}

	// Set query limit parameters in request body if specified
	if opts.MaxResultRecords > 0 {
		req.MaxResultRecords = opts.MaxResultRecords
	}
	if opts.MaxResultBytes > 0 {
		req.MaxResultBytes = opts.MaxResultBytes
	}
	if opts.DefaultScanLimitGbytes > 0 {
		req.DefaultScanLimitGbytes = opts.DefaultScanLimitGbytes
	}

	// Set query execution parameters
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

	// Set timeframe parameters
	if opts.DefaultTimeframeStart != "" {
		req.DefaultTimeframeStart = opts.DefaultTimeframeStart
	}
	if opts.DefaultTimeframeEnd != "" {
		req.DefaultTimeframeEnd = opts.DefaultTimeframeEnd
	}

	// Set localization parameters
	if opts.Locale != "" {
		req.Locale = opts.Locale
	}
	if opts.Timezone != "" {
		req.Timezone = opts.Timezone
	}

	var result DQLQueryResponse

	// Create context with 5 minute timeout to accommodate Grail's maximum query time
	// The server will return 202 if query takes longer than requestTimeoutMilliseconds
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Note: Client-level retries won't trigger for 202 responses (success status)
	httpReq := e.client.HTTP().R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result)

	resp, err := httpReq.Post("/platform/storage/query/v1/query:execute")

	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Handle both 200 (completed) and 202 (accepted/running) responses
	if resp.StatusCode() == 202 || (resp.StatusCode() == 200 && result.State == "RUNNING") {
		if result.RequestToken == "" {
			return nil, fmt.Errorf("query is running but no request token provided")
		}
		// Query is running, poll for results
		result, err = e.pollForResultsWithOptions(result.RequestToken, opts)
		if err != nil {
			return nil, err
		}
	} else if resp.IsError() {
		return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// VerifyQuery verifies a DQL query without executing it
func (e *DQLExecutor) VerifyQuery(query string, opts DQLVerifyOptions) (*DQLVerifyResponse, error) {
	req := DQLVerifyRequest{
		Query:                  query,
		GenerateCanonicalQuery: opts.GenerateCanonicalQuery,
	}

	// Set localization parameters
	if opts.Timezone != "" {
		req.Timezone = opts.Timezone
	}
	if opts.Locale != "" {
		req.Locale = opts.Locale
	}

	var result DQLVerifyResponse

	// Create context with 30-second timeout (verify is fast)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	httpReq := e.client.HTTP().R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result)

	resp, err := httpReq.Post("/platform/storage/query/v1/query:verify")

	if err != nil {
		return nil, fmt.Errorf("failed to verify query: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("query verification failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// GetNotifications returns notifications from the response (checking both top-level and result metadata)
func (r *DQLQueryResponse) GetNotifications() []QueryNotification {
	// Check top-level metadata first
	if r.Metadata != nil && r.Metadata.Grail != nil && len(r.Metadata.Grail.Notifications) > 0 {
		return r.Metadata.Grail.Notifications
	}
	// Check result-level metadata
	if r.Result != nil && r.Result.Metadata != nil && r.Result.Metadata.Grail != nil {
		return r.Result.Metadata.Grail.Notifications
	}
	return nil
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

// isStderrTerminal checks if stderr is a terminal (for color output)
func isStderrTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
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

	// Check by notification type first
	if hint, ok := hints[notificationType]; ok {
		return hint
	}

	// Fallback: pattern match on message content for common cases
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

// printNotifications prints query notifications/warnings to stderr
func (e *DQLExecutor) printNotifications(notifications []QueryNotification) {
	useColor := isStderrTerminal()

	for _, n := range notifications {
		severity := n.Severity
		if severity == "" {
			severity = "INFO"
		}
		// Print warnings and errors prominently to stderr
		if severity == "WARNING" || severity == "WARN" {
			if useColor {
				fmt.Fprintf(os.Stderr, "%sWarning:%s %s\n", colorYellow, colorReset, n.Message)
			} else {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", n.Message)
			}
			// Print hint if available
			if hint := getHintForNotification(n.NotificationType, n.Message); hint != "" {
				if useColor {
					fmt.Fprintf(os.Stderr, "%sHint:%s %s\n", colorCyan, colorReset, hint)
				} else {
					fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
				}
			}
		} else if severity == "ERROR" {
			if useColor {
				fmt.Fprintf(os.Stderr, "%sError:%s %s\n", colorRed, colorReset, n.Message)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", n.Message)
			}
			// Print hint for errors too
			if hint := getHintForNotification(n.NotificationType, n.Message); hint != "" {
				if useColor {
					fmt.Fprintf(os.Stderr, "%sHint:%s %s\n", colorCyan, colorReset, hint)
				} else {
					fmt.Fprintf(os.Stderr, "Hint: %s\n", hint)
				}
			}
		}
	}
}

// printResults prints the query results with the given options
func (e *DQLExecutor) printResults(result *DQLQueryResponse, opts DQLExecuteOptions) error {
	// Print any notifications/warnings first
	if notifications := result.GetNotifications(); len(notifications) > 0 {
		e.printNotifications(notifications)
	}

	// Extract records from result
	records := result.Records
	if result.Result != nil && len(result.Result.Records) > 0 {
		records = result.Result.Records
	}

	// Create printer with options
	printer := output.NewPrinterWithOpts(output.PrinterOptions{
		Format:     opts.OutputFormat,
		Width:      opts.Width,
		Height:     opts.Height,
		Fullscreen: opts.Fullscreen,
	})

	if opts.OutputFormat == "table" {
		return e.printTable(records)
	}

	if opts.OutputFormat == "csv" {
		// For CSV, print records directly without wrapping
		if len(records) == 0 {
			return nil
		}
		return printer.PrintList(records)
	}

	// For non-table output, include only records
	if len(records) > 0 {
		return printer.Print(map[string]interface{}{"records": records})
	}
	return printer.Print(result)
}

// pollForResults polls the query:poll endpoint until the query completes
//
//nolint:unused // Reserved for future polling features
func (e *DQLExecutor) pollForResults(requestToken string) (DQLQueryResponse, error) {
	return e.pollForResultsWithOptions(requestToken, DQLExecuteOptions{})
}

// pollForResultsWithOptions polls the query:poll endpoint until the query completes with options
func (e *DQLExecutor) pollForResultsWithOptions(requestToken string, opts DQLExecuteOptions) (DQLQueryResponse, error) {
	var result DQLQueryResponse

	for {
		httpReq := e.client.HTTP().R().
			SetQueryParam("request-token", requestToken).
			SetQueryParam("request-timeout-milliseconds", "60000").
			SetResult(&result)

		resp, err := httpReq.Get("/platform/storage/query/v1/query:poll")

		if err != nil {
			return result, fmt.Errorf("failed to poll query: %w", err)
		}

		if resp.IsError() {
			return result, fmt.Errorf("poll failed with status %d: %s", resp.StatusCode(), resp.String())
		}

		if result.State == "SUCCEEDED" || result.State == "FAILED" {
			break
		}

		// If still running, the long poll should have waited, but just in case
		if result.State != "RUNNING" {
			break
		}
	}

	if result.State == "FAILED" {
		return result, fmt.Errorf("query execution failed")
	}

	return result, nil
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
		fmt.Println("No results found.")
		return nil
	}

	// Convert to JSON for consistent table printing
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
