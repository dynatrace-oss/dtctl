// Package query provides a typed client for the Dynatrace DQL Query API
// (/platform/storage/query/v1/).
//
// It supports synchronous and asynchronous query execution with automatic
// polling, query verification, and cancellation.
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles DQL query execution against the Dynatrace Query API.
type Handler struct {
	client *httpclient.Client

	// headers is an optional map of extra HTTP headers to include on every request
	// (e.g., dt-client-context).
	headers map[string]string
}

// NewHandler creates a new query handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// WithHeaders returns a shallow copy of the handler with extra HTTP headers
// set on every request. This is useful for the dt-client-context header.
func (h *Handler) WithHeaders(headers map[string]string) *Handler {
	cp := *h
	cp.headers = headers
	return &cp
}

// --- Request / Response types ---

// FilterSegmentRef identifies a segment and optional variable bindings for query execution.
type FilterSegmentRef struct {
	ID        string                  `json:"id"`
	Variables []FilterSegmentVariable `json:"variables,omitempty"`
}

// FilterSegmentVariable defines a variable binding for a filter segment.
type FilterSegmentVariable struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// ExecuteRequest represents a DQL query execution request body.
type ExecuteRequest struct {
	Query                      string  `json:"query"`
	RequestTimeoutMilliseconds int64   `json:"requestTimeoutMilliseconds,omitempty"`
	MaxResultRecords           int64   `json:"maxResultRecords,omitempty"`
	MaxResultBytes             int64   `json:"maxResultBytes,omitempty"`
	DefaultScanLimitGbytes     float64 `json:"defaultScanLimitGbytes,omitempty"`
	DefaultSamplingRatio       float64 `json:"defaultSamplingRatio,omitempty"`
	FetchTimeoutSeconds        int32   `json:"fetchTimeoutSeconds,omitempty"`
	// PollingPromiseSeconds bounds the maximum gap, in seconds, between
	// successive polls of an asynchronous query. If the client does not issue
	// the next poll within this window after the previous response, the backend
	// auto-cancels the query. Optional.
	PollingPromiseSeconds        int32              `json:"pollingPromiseSeconds,omitempty"`
	EnablePreview                bool               `json:"enablePreview,omitempty"`
	EnforceQueryConsumptionLimit bool               `json:"enforceQueryConsumptionLimit,omitempty"`
	IncludeTypes                 *bool              `json:"includeTypes,omitempty"`
	IncludeContributions         *bool              `json:"includeContributions,omitempty"`
	DefaultTimeframeStart        string             `json:"defaultTimeframeStart,omitempty"`
	DefaultTimeframeEnd          string             `json:"defaultTimeframeEnd,omitempty"`
	Locale                       string             `json:"locale,omitempty"`
	Timezone                     string             `json:"timezone,omitempty"`
	FilterSegments               []FilterSegmentRef `json:"filterSegments,omitempty"`

	// EnrichMetricMetadata requests metric-catalogue enrichment of the response's
	// metadata.metrics[] entries (displayName, description, unit). It is sent as the
	// `enrich=metric-metadata` query-string parameter on both query:execute and
	// query:poll — not in the request body — hence json:"-".
	EnrichMetricMetadata bool `json:"-"`
}

// enrichMetricMetadataParam is the query-string value that asks the Grail query
// API to enrich metadata.metrics[] with metric-catalogue fields.
const enrichMetricMetadataParam = "metric-metadata"

// Response represents a DQL query response from execute or poll.
type Response struct {
	State        string                   `json:"state"`
	RequestToken string                   `json:"requestToken,omitempty"`
	Result       *Result                  `json:"result,omitempty"`
	Records      []map[string]interface{} `json:"records,omitempty"` // backward compatibility
	Progress     int                      `json:"progress,omitempty"`
	Metadata     *Metadata                `json:"metadata,omitempty"`
}

// Result represents the result section of a DQL response.
type Result struct {
	Records  []map[string]interface{} `json:"records"`
	Types    []ColumnTypes            `json:"types,omitempty"`
	Metadata *Metadata                `json:"metadata,omitempty"`
}

// ColumnTypes describes the column types for a contiguous range of records,
// as returned by the DQL API when includeTypes is requested. The API groups
// records that share the same column type mappings under a single entry.
type ColumnTypes struct {
	// IndexRange is the [start, end] record index range (inclusive) that these
	// mappings apply to.
	IndexRange []int `json:"indexRange"`
	// Mappings maps a column name to its DQL type descriptor.
	Mappings map[string]ColumnType `json:"mappings"`
}

// ColumnType is the DQL type descriptor for a single column, e.g. {"type": "long"}.
type ColumnType struct {
	Type string `json:"type"`
}

// MetricInfo describes a single metric referenced in a timeseries query result.
// It maps the DQL column name (FieldName) to the underlying metric descriptor.
// DisplayName, Description, and Unit are present only when the API returns metric
// catalogue data; they are absent for tenants or queries that do not populate them.
type MetricInfo struct {
	MetricKey   string `json:"metric.key,omitempty"`
	FieldName   string `json:"fieldName,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Unit        string `json:"unit,omitempty"`
}

// Metadata represents the metadata section of a DQL response.
type Metadata struct {
	Grail   *GrailMetadata `json:"grail,omitempty"`
	Metrics []MetricInfo   `json:"metrics,omitempty"`
}

// GrailMetadata represents Grail-specific query execution metadata.
type GrailMetadata struct {
	Query                     string             `json:"query,omitempty"`
	CanonicalQuery            string             `json:"canonicalQuery,omitempty"`
	QueryID                   string             `json:"queryId,omitempty"`
	DQLVersion                string             `json:"dqlVersion,omitempty"`
	Timezone                  string             `json:"timezone,omitempty"`
	Locale                    string             `json:"locale,omitempty"`
	ExecutionTimeMilliseconds int64              `json:"executionTimeMilliseconds,omitempty"`
	ScannedRecords            int64              `json:"scannedRecords,omitempty"`
	ScannedBytes              int64              `json:"scannedBytes,omitempty"`
	ScannedDataPoints         int64              `json:"scannedDataPoints,omitempty"`
	Sampled                   bool               `json:"sampled,omitempty"`
	Notifications             []Notification     `json:"notifications,omitempty"`
	AnalysisTimeframe         *AnalysisTimeframe `json:"analysisTimeframe,omitempty"`
	Contributions             *Contributions     `json:"contributions,omitempty"`
}

// Contributions represents the bucket contributions for a query.
type Contributions struct {
	Buckets []BucketContribution `json:"buckets,omitempty"`
}

// BucketContribution represents a single bucket's contribution to query results.
type BucketContribution struct {
	Name                string  `json:"name"`
	Table               string  `json:"table"`
	ScannedBytes        int64   `json:"scannedBytes"`
	MatchedRecordsRatio float64 `json:"matchedRecordsRatio"`
}

// Notification represents a notification/warning from query execution.
type Notification struct {
	Severity         string   `json:"severity,omitempty"`
	NotificationType string   `json:"notificationType,omitempty"`
	Message          string   `json:"message,omitempty"`
	MessageFormat    string   `json:"messageFormat,omitempty"`
	Arguments        []string `json:"arguments,omitempty"`
}

// AnalysisTimeframe represents the timeframe analyzed by the query.
type AnalysisTimeframe struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// VerifyRequest represents a DQL query verification request body.
type VerifyRequest struct {
	Query                  string `json:"query"`
	GenerateCanonicalQuery bool   `json:"generateCanonicalQuery,omitempty"`
	Timezone               string `json:"timezone,omitempty"`
	Locale                 string `json:"locale,omitempty"`
}

// VerifyResponse represents a DQL query verification response.
type VerifyResponse struct {
	Valid          bool                 `json:"valid"`
	CanonicalQuery string               `json:"canonicalQuery,omitempty"`
	Notifications  []VerifyNotification `json:"notifications,omitempty"`
}

// VerifyNotification represents a notification from query verification.
type VerifyNotification struct {
	Severity         string          `json:"severity"`
	NotificationType string          `json:"notificationType"`
	Message          string          `json:"message"`
	SyntaxPosition   *SyntaxPosition `json:"syntaxPosition,omitempty"`
}

// SyntaxPosition represents the position of a syntax issue in a query.
type SyntaxPosition struct {
	Start *Position `json:"start,omitempty"`
	End   *Position `json:"end,omitempty"`
}

// Position represents a line and column position in text.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// ErrorResponse represents the structured error response from the DQL query API.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Details struct {
			ErrorType    string   `json:"errorType"`
			ErrorMessage string   `json:"errorMessage"`
			Arguments    []string `json:"arguments"`
		} `json:"details"`
	} `json:"error"`
}

// --- Constants ---

// pollRequestTimeoutMs is the server-side hold time per poll round trip in milliseconds.
const pollRequestTimeoutMs int64 = 5000

// defaultPollingPromiseSeconds caps the gap between successive polls before
// the backend auto-cancels a running query. dtctl re-polls immediately after
// each long-poll returns RUNNING, so 5s is comfortably above the actual gap.
const defaultPollingPromiseSeconds int32 = 5

const basePath = "/platform/storage/query/v1/query"

// --- API methods ---

// Execute submits a DQL query for execution. If the query completes synchronously
// the response contains the results directly. If the query is asynchronous
// (HTTP 202 or state RUNNING), the response contains a RequestToken for polling.
func (h *Handler) Execute(ctx context.Context, req ExecuteRequest) (*Response, error) {
	var result Response

	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result)
	if req.EnrichMetricMetadata {
		httpReq.SetQueryParam("enrich", enrichMetricMetadataParam)
	}
	h.applyHeaders(httpReq)

	resp, err := httpReq.Post(basePath + ":execute")
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// 200 with completed state or 202 with RUNNING are both valid
	if resp.StatusCode() == 200 || resp.StatusCode() == 202 {
		return &result, nil
	}

	if resp.IsError() {
		return nil, parseError(resp.StatusCode(), resp.Body())
	}

	return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode())
}

// Poll polls for the results of an asynchronous query. The server holds the
// connection for up to timeoutMs milliseconds before returning. When enrich is
// true, the poll requests metric-metadata enrichment so the final SUCCEEDED
// response carries displayName/description/unit — it must match the enrichment
// requested on the originating execute call.
func (h *Handler) Poll(ctx context.Context, requestToken string, timeoutMs int64, enrich bool) (*Response, error) {
	var result Response

	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetQueryParam("request-token", requestToken).
		SetQueryParam("request-timeout-milliseconds", fmt.Sprintf("%d", timeoutMs)).
		SetResult(&result)
	if enrich {
		httpReq.SetQueryParam("enrich", enrichMetricMetadataParam)
	}
	h.applyHeaders(httpReq)

	resp, err := httpReq.Get(basePath + ":poll")
	if err != nil {
		return nil, fmt.Errorf("failed to poll query: %w", err)
	}
	if resp.IsError() {
		return nil, httpclient.NewAPIError(resp.StatusCode(), resp.Status(), resp.String())
	}

	return &result, nil
}

// Cancel sends a best-effort cancellation request for a running query.
func (h *Handler) Cancel(ctx context.Context, requestToken string) error {
	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetQueryParam("request-token", requestToken)
	h.applyHeaders(httpReq)

	resp, err := httpReq.Post(basePath + ":cancel")
	if err != nil {
		return fmt.Errorf("failed to cancel query: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("cancel failed with status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// Verify validates a DQL query without executing it.
func (h *Handler) Verify(ctx context.Context, req VerifyRequest) (*VerifyResponse, error) {
	var result VerifyResponse

	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result)
	h.applyHeaders(httpReq)

	resp, err := httpReq.Post(basePath + ":verify")
	if err != nil {
		return nil, fmt.Errorf("failed to verify query: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("query verification failed: %w", err)
	}

	return &result, nil
}

// PollUpdate carries the state of a still-running query. It is delivered to the
// OnUpdate callback of ExecuteAndPollWithOptions on the initial RUNNING response
// and after each subsequent poll, letting callers render progress.
type PollUpdate struct {
	// Progress is the query's completion percentage (0-100) as reported by the
	// backend. It may stay at 0 until the backend has a meaningful estimate.
	Progress int
	// Preview is a partial result snapshot. It is non-nil only when the request
	// set EnablePreview and this poll carried preview records. Each preview is
	// whole (not a delta to prior previews), so callers replace, never append.
	Preview *Result
	// ScannedBytes and ScannedRecords are the running scan totals reported by the
	// backend for this poll. Grail populates and grows them on each poll of a
	// mass-data query; they are 0 when the backend has not reported them (e.g. an
	// early poll or a query type that does not scan Grail).
	ScannedBytes   int64
	ScannedRecords int64
}

// ExecuteAndPollOptions carries optional hooks for ExecuteAndPollWithOptions.
type ExecuteAndPollOptions struct {
	// OnUnauthorized is invoked when a poll receives HTTP 401, allowing callers
	// to refresh an expired token. It must return the new bearer token. If nil,
	// 401 errors are returned directly.
	OnUnauthorized func() (string, error)
	// OnUpdate, if set, is called with the current progress/preview whenever the
	// query is still RUNNING — once for the initial response and once per poll.
	// It is never called for a query that completes synchronously.
	OnUpdate func(PollUpdate)
}

// ExecuteAndPoll executes a DQL query and, if it returns asynchronously, polls
// until completion or context cancellation. If the context is cancelled during
// polling, a best-effort cancel is sent to the backend.
//
// The optional onUnauthorized callback is invoked when a poll receives HTTP 401,
// allowing callers to refresh an expired token. It must return the new bearer
// token. If nil, 401 errors are returned directly.
func (h *Handler) ExecuteAndPoll(ctx context.Context, req ExecuteRequest, onUnauthorized func() (string, error)) (*Response, error) {
	return h.ExecuteAndPollWithOptions(ctx, req, ExecuteAndPollOptions{OnUnauthorized: onUnauthorized})
}

// ExecuteAndPollWithOptions is ExecuteAndPoll with additional hooks (see
// ExecuteAndPollOptions), notably an OnUpdate callback for surfacing progress
// and preview results while the query is still running.
func (h *Handler) ExecuteAndPollWithOptions(ctx context.Context, req ExecuteRequest, opts ExecuteAndPollOptions) (*Response, error) {
	onUnauthorized := opts.OnUnauthorized
	// Use an independent context for the initial execute so we always get the
	// request token back even if the caller cancels mid-flight.
	execCtx, execCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer execCancel()

	// Ensure the execute request uses the server-side long-poll timeout so the
	// backend returns promptly for the poll loop.
	if req.RequestTimeoutMilliseconds == 0 {
		req.RequestTimeoutMilliseconds = pollRequestTimeoutMs
	}
	if req.PollingPromiseSeconds == 0 {
		req.PollingPromiseSeconds = defaultPollingPromiseSeconds
	}

	result, err := h.Execute(execCtx, req)
	if err != nil {
		// A 401 on the initial execute gets the same one-shot refresh the
		// poll loop has — long-lived processes outlive the first
		// bearer token, and without this every query after expiry fails
		// terminally even though a refresher is wired.
		if isUnauthorized(err) && onUnauthorized != nil {
			newToken, refreshErr := onUnauthorized()
			if refreshErr != nil {
				return nil, fmt.Errorf("execute returned 401 (%v) and token refresh failed: %w", err, refreshErr)
			}
			if newToken != "" {
				h.client.SetToken(newToken)
			}
			result, err = h.Execute(execCtx, req)
		}
		if err != nil {
			return nil, err
		}
	}

	// If caller cancelled while execute was in-flight, cancel backend query.
	if ctx.Err() != nil {
		if result.RequestToken != "" {
			_ = h.Cancel(context.Background(), result.RequestToken)
		}
		return nil, ctx.Err()
	}

	// If query completed synchronously, return. NOT_STARTED is a queued query
	// on a congested tenant — terminal-looking but in-flight; returning it here
	// would hand the caller the raw {"state":"NOT_STARTED"} envelope as a
	// success (observed live: three leaks in a row on a busy dev tenant).
	if result.State != "RUNNING" && result.State != "NOT_STARTED" {
		return result, nil
	}

	if result.RequestToken == "" {
		return nil, fmt.Errorf("query is running but no request token provided")
	}

	// Surface the initial RUNNING state (progress may already be non-zero).
	emitUpdate(opts.OnUpdate, req.EnablePreview, result)

	// Poll loop.
	pollCtx, pollCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer pollCancel()

	tokenJustRefreshed := false

	for {
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				_ = h.Cancel(context.Background(), result.RequestToken)
			}
			return nil, pollCtx.Err()
		default:
		}

		pollResult, pollErr := h.Poll(pollCtx, result.RequestToken, pollRequestTimeoutMs, req.EnrichMetricMetadata)
		if pollErr != nil {
			// On 401, try the onUnauthorized callback once per consecutive failure.
			var apiErr *httpclient.APIError
			if errors.As(pollErr, &apiErr) && apiErr.StatusCode == 401 && onUnauthorized != nil && !tokenJustRefreshed {
				newToken, refreshErr := onUnauthorized()
				if refreshErr != nil {
					return nil, fmt.Errorf("poll returned 401 and token refresh failed: %w", refreshErr)
				}
				if newToken != "" {
					h.client.SetToken(newToken)
				}
				tokenJustRefreshed = true
				continue
			}

			if ctx.Err() != nil {
				_ = h.Cancel(context.Background(), result.RequestToken)
				return nil, ctx.Err()
			}
			return nil, pollErr
		}

		tokenJustRefreshed = false // reset after success

		switch pollResult.State {
		case "SUCCEEDED":
			return pollResult, nil
		case "FAILED":
			return pollResult, fmt.Errorf("query execution failed")
		case "RUNNING", "NOT_STARTED":
			emitUpdate(opts.OnUpdate, req.EnablePreview, pollResult)
			continue
		default:
			return pollResult, nil
		}
	}
}

// GetNotifications returns notifications from the response, checking both
// top-level and result-level metadata.
func (r *Response) GetNotifications() []Notification {
	if r.Metadata != nil && r.Metadata.Grail != nil && len(r.Metadata.Grail.Notifications) > 0 {
		return r.Metadata.Grail.Notifications
	}
	if r.Result != nil && r.Result.Metadata != nil && r.Result.Metadata.Grail != nil {
		return r.Result.Metadata.Grail.Notifications
	}
	return nil
}

// GetRecords returns the result records, checking both the Result wrapper and
// the top-level Records field (backward compatibility).
func (r *Response) GetRecords() []map[string]interface{} {
	if r.Result != nil && len(r.Result.Records) > 0 {
		return r.Result.Records
	}
	return r.Records
}

// GetTypes returns the per-column DQL type mappings from the result section,
// as populated when the request set includeTypes. Returns nil when the API did
// not include type information.
func (r *Response) GetTypes() []ColumnTypes {
	if r.Result != nil {
		return r.Result.Types
	}
	return nil
}

// GetMetadata returns the Grail metadata from the response, checking both
// top-level and result-level metadata.
func (r *Response) GetMetadata() *GrailMetadata {
	if r.Result != nil && r.Result.Metadata != nil && r.Result.Metadata.Grail != nil {
		return r.Result.Metadata.Grail
	}
	if r.Metadata != nil && r.Metadata.Grail != nil {
		return r.Metadata.Grail
	}
	return nil
}

// GetMetrics returns the metrics array from the response, checking both
// result-level and top-level metadata. Returns nil when no metrics are present
// (e.g., for non-timeseries queries).
func (r *Response) GetMetrics() []MetricInfo {
	if r.Result != nil && r.Result.Metadata != nil && len(r.Result.Metadata.Metrics) > 0 {
		return r.Result.Metadata.Metrics
	}
	if r.Metadata != nil && len(r.Metadata.Metrics) > 0 {
		return r.Metadata.Metrics
	}
	return nil
}

// --- Internal helpers ---

// emitUpdate invokes the OnUpdate callback (when set) with the progress and any
// preview carried by a RUNNING response. A preview is attached only when the
// request enabled previews and the response actually carried records.
func emitUpdate(onUpdate func(PollUpdate), enablePreview bool, resp *Response) {
	if onUpdate == nil {
		return
	}
	var preview *Result
	if enablePreview && resp.Result != nil && len(resp.Result.Records) > 0 {
		preview = resp.Result
	}
	u := PollUpdate{Progress: resp.Progress, Preview: preview}
	if m := resp.GetMetadata(); m != nil {
		u.ScannedBytes = m.ScannedBytes
		u.ScannedRecords = m.ScannedRecords
	}
	onUpdate(u)
}

func (h *Handler) applyHeaders(req *resty.Request) {
	for k, v := range h.headers {
		req.SetHeader(k, v)
	}
}

// parseError parses the DQL API error response into a structured error.
// isUnauthorized reports whether an error is an HTTP 401 from either error
// shape this package produces: Execute wraps error bodies as *QueryError,
// Poll as *httpclient.APIError.
func isUnauthorized(err error) bool {
	var apiErr *httpclient.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401
	}
	var queryErr *QueryError
	if errors.As(err, &queryErr) {
		return queryErr.StatusCode == 401
	}
	return false
}

func parseError(statusCode int, body []byte) error {
	var apiErr ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return &QueryError{
			StatusCode: statusCode,
			Message:    apiErr.Error.Message,
			ErrorType:  apiErr.Error.Details.ErrorType,
			Detail:     apiErr.Error.Details.ErrorMessage,
			Arguments:  apiErr.Error.Details.Arguments,
		}
	}
	// Keep the fallback typed: callers branch on QueryError.StatusCode (the
	// 401-refresh path), and Error() renders the same message either way.
	return &QueryError{StatusCode: statusCode, Message: string(body)}
}

// QueryError is a structured error from the DQL Query API.
type QueryError struct {
	StatusCode int
	Message    string
	ErrorType  string
	Detail     string
	Arguments  []string
}

func (e *QueryError) Error() string {
	if e.ErrorType != "" {
		// The top-level message often just repeats the error type
		// (e.g. "UNKNOWN_COMMAND"); details.errorMessage and the arguments
		// carry the actionable part (offending token, position), so include
		// whatever adds information.
		msg := e.Message
		if e.Detail != "" && e.Detail != msg {
			if msg == e.ErrorType || msg == "" {
				msg = e.Detail
			} else {
				msg += " — " + e.Detail
			}
		}
		if len(e.Arguments) > 0 && !strings.Contains(msg, e.Arguments[0]) {
			msg += " [" + strings.Join(e.Arguments, ", ") + "]"
		}
		return fmt.Sprintf("query failed (%s): %s", e.ErrorType, msg)
	}
	return fmt.Sprintf("query failed with status %d: %s", e.StatusCode, e.Message)
}
