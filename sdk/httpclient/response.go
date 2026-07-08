package httpclient

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/dynatrace-oss/dtctl/pkg/metrics"
)

// CheckResponse inspects a resty response and returns a structured [*APIError]
// if the HTTP status code indicates an error (4xx/5xx). Returns nil for
// successful responses.
//
// It attempts to parse the Dynatrace error response body for a human-readable
// message. If parsing fails, the raw response body is included as details.
func CheckResponse(resp *resty.Response) error {
	if resp == nil || !resp.IsError() {
		return nil
	}

	msg := resp.Status()
	details := ""

	// Try to extract message from Dynatrace error envelope.
	// Common shapes: {"error":{"message":"..."}} or {"message":"..."}
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err == nil {
		if envelope.Error != nil && envelope.Error.Message != "" {
			msg = envelope.Error.Message
		} else if envelope.Message != "" {
			msg = envelope.Message
		}
	} else {
		// If we can't parse JSON, include raw body as details (truncated to 1KB).
		if body := resp.String(); body != "" {
			const maxDetails = 1024
			if len(body) > maxDetails {
				body = body[:maxDetails] + "... (truncated)"
			}
			details = body
		}
	}

	statusCode := resp.StatusCode()
	op := "http"
	if resp.Request != nil && (resp.Request.Method != "" || resp.Request.URL != "") {
		op = fmt.Sprintf("%s %s", resp.Request.Method, resp.Request.URL)
	}
	metrics.Default().RecordAPIError(op, statusCode, msg)

	return NewAPIError(statusCode, msg, details)
}
