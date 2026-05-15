package httpclient

import (
	"encoding/json"

	"github.com/go-resty/resty/v2"
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
		// If we can't parse JSON, include raw body as details.
		if body := resp.String(); body != "" {
			details = body
		}
	}

	return NewAPIError(resp.StatusCode(), msg, details)
}
