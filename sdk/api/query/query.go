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
	Query                        string             `json:"query"`
	RequestTimeoutMilliseconds   int64              `json:"requestTimeoutMilliseconds,omitempty"`
	MaxResultRecords             int64              `json:"maxResultRecords,omitempty"`
	MaxResultBytes               int64              `json:"maxResultBytes,omitempty"`
	DefaultScanLimitGbytes       float64            `json:"defaultScanLimitGbytes,omitempty"`
	DefaultSamplingRatio         float64            `json:"defaultSamplingRatio,omitempty"`
	FetchTimeoutSeconds          int32              `json:"fetchTimeoutSeconds,omitempty"`
	EnablePreview                bool               `json:"enablePreview,omitempty"`
	EnforceQueryConsumptionLimit bool               `json:"enforceQueryConsumptionLimit,omitempty"`
	IncludeTypes                 *bool              `json:"includeTypes,omitempty"`
	IncludeContributions         *bool              `json:"includeContributions,omitempty"`
	DefaultTimeframeStart        string             `json:"defaultTimeframeStart,omitempty"`
	DefaultTimeframeEnd          string             `json:"defaultTimeframeEnd,omitempty"`
	Locale                       string             `json:"locale,omitempty"`
	Timezone                     string             `json:"timezone,omitempty"`
	FilterSegments               []FilterSegmentRef `json:"filterSegments,omitempty"`
}

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
	Metadata *Metadata                `json:"metadata,omitempty"`
}

// Metadata represents the metadata section of a DQL response.
type Metadata struct {
	Grail *GrailMetadata `json:"grail,omitempty"`
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
// connection for up to timeoutMs milliseconds before returning.
func (h *Handler) Poll(ctx context.Context, requestToken string, timeoutMs int64) (*Response, error) {
	var result Response

	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetQueryParam("request-token", requestToken).
		SetQueryParam("request-timeout-milliseconds", fmt.Sprintf("%d", timeoutMs)).
		SetResult(&result)
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

// ExecuteAndPoll executes a DQL query and, if it returns asynchronously, polls
// until completion or context cancellation. If the context is cancelled during
// polling, a best-effort cancel is sent to the backend.
//
// The optional onUnauthorized callback is invoked when a poll receives HTTP 401,
// allowing callers to refresh an expired token. It must return the new bearer
// token. If nil, 401 errors are returned directly.
func (h *Handler) ExecuteAndPoll(ctx context.Context, req ExecuteRequest, onUnauthorized func() (string, error)) (*Response, error) {
	// Use an independent context for the initial execute so we always get the
	// request token back even if the caller cancels mid-flight.
	execCtx, execCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer execCancel()

	// Ensure the execute request uses the server-side long-poll timeout so the
	// backend returns promptly for the poll loop.
	if req.RequestTimeoutMilliseconds == 0 {
		req.RequestTimeoutMilliseconds = pollRequestTimeoutMs
	}

	result, err := h.Execute(execCtx, req)
	if err != nil {
		return nil, err
	}

	// If caller cancelled while execute was in-flight, cancel backend query.
	if ctx.Err() != nil {
		if result.RequestToken != "" {
			_ = h.Cancel(context.Background(), result.RequestToken)
		}
		return nil, ctx.Err()
	}

	// If query completed synchronously, return.
	if result.State != "RUNNING" {
		return result, nil
	}

	if result.RequestToken == "" {
		return nil, fmt.Errorf("query is running but no request token provided")
	}

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

		pollResult, pollErr := h.Poll(pollCtx, result.RequestToken, pollRequestTimeoutMs)
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
		case "RUNNING":
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

// --- Internal helpers ---

func (h *Handler) applyHeaders(req *resty.Request) {
	for k, v := range h.headers {
		req.SetHeader(k, v)
	}
}

// parseError parses the DQL API error response into a structured error.
func parseError(statusCode int, body []byte) error {
	var apiErr ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return &QueryError{
			StatusCode: statusCode,
			Message:    apiErr.Error.Message,
			ErrorType:  apiErr.Error.Details.ErrorType,
			Arguments:  apiErr.Error.Details.Arguments,
		}
	}
	return fmt.Errorf("query failed with status %d: %s", statusCode, string(body))
}

// QueryError is a structured error from the DQL Query API.
type QueryError struct {
	StatusCode int
	Message    string
	ErrorType  string
	Arguments  []string
}

func (e *QueryError) Error() string {
	if e.ErrorType != "" {
		return fmt.Sprintf("query failed (%s): %s", e.ErrorType, e.Message)
	}
	return fmt.Sprintf("query failed with status %d: %s", e.StatusCode, e.Message)
}
