package agentmode

import (
	"encoding/json"
	"io"
)

// Response is the standard agent mode envelope that wraps all CLI output.
// Success responses have OK=true with Result populated.
// Error responses have OK=false with Error populated.
type Response struct {
	OK      bool             `json:"ok"`
	Result  interface{}      `json:"result"`
	Error   *ErrorDetail     `json:"error,omitempty"`
	Context *ResponseContext `json:"context,omitempty"`
}

// ResponseContext provides operational metadata alongside the result.
type ResponseContext struct {
	Total       *int              `json:"total,omitempty"`
	HasMore     bool              `json:"has_more,omitempty"`
	Verb        string            `json:"verb,omitempty"`
	Resource    string            `json:"resource,omitempty"`
	Suggestions []string          `json:"suggestions,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	Duration    string            `json:"duration,omitempty"`
	Links       map[string]string `json:"links,omitempty"`
}

// ErrorDetail is a structured error for machine consumption.
type ErrorDetail struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Operation   string   `json:"operation,omitempty"`
	StatusCode  int      `json:"status_code,omitempty"`
	RequestID   string   `json:"request_id,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// ClassifyHTTPError maps an HTTP status code to a machine-readable error code.
func ClassifyHTTPError(statusCode int) string {
	switch statusCode {
	case 400:
		return "bad_request"
	case 401:
		return "auth_required"
	case 403:
		return "permission_denied"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 429:
		return "rate_limited"
	default:
		if statusCode >= 500 {
			return "server_error"
		}
		return "error"
	}
}

// WriteSuccess writes a success response envelope to the writer.
func WriteSuccess(w io.Writer, result interface{}, ctx *ResponseContext) error {
	resp := Response{
		OK:      true,
		Result:  result,
		Context: ctx,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// WriteError writes an error response envelope to the writer.
func WriteError(w io.Writer, detail *ErrorDetail) error {
	resp := Response{
		OK:    false,
		Error: detail,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
