package output

import (
	"encoding/json"
	"fmt"
	"io"

	toon "github.com/toon-format/toon-go"
)

// Response is the agent mode envelope that wraps all CLI output.
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
// It lives in the Response envelope when ok=false.
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

// AgentPrinter wraps CLI output in a Response envelope for machine consumers.
// It implements the Printer interface.
//
// By default, the result field inside the JSON envelope is encoded as native
// JSON. Callers can switch to TOON via SetResultFormat("toon") for token
// efficiency.
type AgentPrinter struct {
	writer       io.Writer
	ctx          *ResponseContext
	resultFormat string // "json" (default) or "toon"
}

// NewAgentPrinter creates an AgentPrinter that writes envelope-wrapped JSON to writer.
// The result field defaults to JSON encoding. Use SetResultFormat("toon") to enable
// TOON encoding for token efficiency.
func NewAgentPrinter(writer io.Writer, ctx *ResponseContext) *AgentPrinter {
	if ctx == nil {
		ctx = &ResponseContext{}
	}
	return &AgentPrinter{writer: writer, ctx: ctx, resultFormat: "json"}
}

// SetResultFormat controls how the result field is encoded inside the agent
// envelope. Supported values are "toon" and "json" (default). Any other
// value is treated as "json" (i.e. the result is embedded as a native JSON
// value in the envelope).
func (p *AgentPrinter) SetResultFormat(format string) {
	switch format {
	case "toon", "json":
		p.resultFormat = format
	default:
		// Unknown format — fall back to json so the result is always
		// a valid native JSON value inside the envelope.
		p.resultFormat = "json"
	}
}

// Print writes a single result wrapped in the agent envelope.
func (p *AgentPrinter) Print(data interface{}) error {
	result, err := p.encodeResult(data)
	if err != nil {
		return err
	}
	resp := Response{
		OK:      true,
		Result:  result,
		Context: p.ctx,
	}
	enc := json.NewEncoder(p.writer)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// encodeResult encodes data according to the configured result format.
// For "toon" it returns a TOON-encoded string (which json.Encoder will
// emit as a JSON string value inside the envelope). For "json" (or any
// other format) it returns the data as-is so json.Encoder serialises it
// as a native JSON value.
//
// If TOON encoding fails, the method falls back to returning the raw data
// (which json.Encoder will serialise as native JSON) and adds a warning
// to the response context so the consumer can detect the fallback.
func (p *AgentPrinter) encodeResult(data interface{}) (interface{}, error) {
	if p.resultFormat != "toon" || data == nil {
		return data, nil
	}

	generic, err := toGeneric(data)
	if err != nil {
		p.addWarning(fmt.Sprintf("TOON encoding failed (toGeneric): %v; fell back to JSON", err))
		return data, nil // fall back to raw data on conversion error
	}

	encoded, err := toon.MarshalString(generic, toon.WithLengthMarkers(true))
	if err != nil {
		p.addWarning(fmt.Sprintf("TOON encoding failed (marshal): %v; fell back to JSON", err))
		return data, nil // fall back to raw data on marshal error
	}

	return encoded, nil
}

// addWarning appends a warning message to the response context.
func (p *AgentPrinter) addWarning(msg string) {
	p.ctx.Warnings = append(p.ctx.Warnings, msg)
}

// PrintList writes a list result wrapped in the agent envelope.
// If Total has been set via SetTotal, it is included in the context.
func (p *AgentPrinter) PrintList(data interface{}) error {
	return p.Print(data)
}

// SetTotal sets the total item count in the response context.
func (p *AgentPrinter) SetTotal(total int) {
	p.ctx.Total = &total
}

// SetResource sets the resource type in the response context.
func (p *AgentPrinter) SetResource(resource string) {
	p.ctx.Resource = resource
}

// SetSuggestions sets the follow-up suggestions in the response context.
func (p *AgentPrinter) SetSuggestions(suggestions []string) {
	p.ctx.Suggestions = suggestions
}

// SetWarnings sets non-fatal warnings in the response context.
func (p *AgentPrinter) SetWarnings(warnings []string) {
	p.ctx.Warnings = warnings
}

// SetDuration sets the operation duration in the response context.
// Only use for commands where timing is meaningful (wait, query, exec).
func (p *AgentPrinter) SetDuration(duration string) {
	p.ctx.Duration = duration
}

// SetLinks sets deep links to the Dynatrace UI in the response context.
func (p *AgentPrinter) SetLinks(links map[string]string) {
	p.ctx.Links = links
}

// SetHasMore indicates whether more results exist beyond the current page.
func (p *AgentPrinter) SetHasMore(hasMore bool) {
	p.ctx.HasMore = hasMore
}

// Context returns the response context for direct manipulation.
func (p *AgentPrinter) Context() *ResponseContext {
	return p.ctx
}

// PrintError writes an error response to the given writer in agent envelope format.
// This is a package-level function because errors are handled centrally in cmd/root.go,
// not through the printer instance.
func PrintError(writer io.Writer, detail *ErrorDetail) error {
	resp := Response{
		OK:    false,
		Error: detail,
	}
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
