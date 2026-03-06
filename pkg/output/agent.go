package output

import (
	"encoding/json"
	"io"
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
type AgentPrinter struct {
	writer io.Writer
	ctx    *ResponseContext
}

// NewAgentPrinter creates an AgentPrinter that writes envelope-wrapped JSON to writer.
func NewAgentPrinter(writer io.Writer, ctx *ResponseContext) *AgentPrinter {
	if ctx == nil {
		ctx = &ResponseContext{}
	}
	return &AgentPrinter{writer: writer, ctx: ctx}
}

// Print writes a single result wrapped in the agent envelope.
func (p *AgentPrinter) Print(data interface{}) error {
	resp := Response{
		OK:      true,
		Result:  data,
		Context: p.ctx,
	}
	enc := json.NewEncoder(p.writer)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
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
