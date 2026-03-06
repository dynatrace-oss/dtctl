package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestAgentPrinter_Print_WrapsInEnvelope(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)

	data := map[string]string{"id": "abc-123", "title": "My Workflow"}
	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Context == nil {
		t.Fatal("expected context to be present")
	}
	if resp.Context.Verb != "get" {
		t.Errorf("expected verb=get, got %q", resp.Context.Verb)
	}
	if resp.Context.Resource != "workflow" {
		t.Errorf("expected resource=workflow, got %q", resp.Context.Resource)
	}
	if resp.Error != nil {
		t.Error("expected no error on success response")
	}

	// Verify result contains our data
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result to be a map, got %T", resp.Result)
	}
	if result["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", result["id"])
	}
}

func TestAgentPrinter_PrintList_SetsTotal(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)

	items := []map[string]string{
		{"id": "1", "title": "WF 1"},
		{"id": "2", "title": "WF 2"},
		{"id": "3", "title": "WF 3"},
	}

	p.SetTotal(3)
	if err := p.PrintList(items); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Context.Total == nil {
		t.Fatal("expected total to be set")
	}
	if *resp.Context.Total != 3 {
		t.Errorf("expected total=3, got %d", *resp.Context.Total)
	}
}

func TestAgentPrinter_SetSuggestions(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	p.SetSuggestions([]string{
		"Run 'dtctl describe workflow abc' for details",
		"Run 'dtctl exec workflow abc' to trigger",
	})

	if err := p.Print(map[string]string{"id": "abc"}); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if len(resp.Context.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(resp.Context.Suggestions))
	}
	if resp.Context.Suggestions[0] != "Run 'dtctl describe workflow abc' for details" {
		t.Errorf("unexpected suggestion: %q", resp.Context.Suggestions[0])
	}
}

func TestAgentPrinter_SetWarnings(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	p.SetWarnings([]string{"API deprecated, will be removed in v2"})

	if err := p.Print("data"); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if len(resp.Context.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(resp.Context.Warnings))
	}
	if resp.Context.Warnings[0] != "API deprecated, will be removed in v2" {
		t.Errorf("unexpected warning: %q", resp.Context.Warnings[0])
	}
}

func TestAgentPrinter_SetHasMore(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	p.SetHasMore(true)

	if err := p.Print([]string{"item1"}); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !resp.Context.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestAgentPrinter_SetDuration(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	p.SetDuration("2.5s")

	if err := p.Print("result"); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if resp.Context.Duration != "2.5s" {
		t.Errorf("expected duration=2.5s, got %q", resp.Context.Duration)
	}
}

func TestAgentPrinter_SetLinks(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	p.SetLinks(map[string]string{
		"ui": "https://env.dynatrace.com/ui/workflows/abc",
	})

	if err := p.Print("data"); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if resp.Context.Links == nil {
		t.Fatal("expected links to be present")
	}
	if resp.Context.Links["ui"] != "https://env.dynatrace.com/ui/workflows/abc" {
		t.Errorf("unexpected link: %q", resp.Context.Links["ui"])
	}
}

func TestAgentPrinter_NilContext(t *testing.T) {
	var buf bytes.Buffer
	// NewAgentPrinter initializes ctx if nil
	p := NewAgentPrinter(&buf, nil)

	if err := p.Print("data"); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Context == nil {
		t.Error("expected context to be initialized")
	}
}

func TestAgentPrinter_ImplementsPrinterInterface(t *testing.T) {
	var _ Printer = (*AgentPrinter)(nil)
}

func TestAgentPrinter_EmptyContextFieldsOmitted(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{Verb: "get"})

	if err := p.Print("data"); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()

	// Fields with zero values should be omitted from JSON
	if containsKey(output, "total") {
		t.Error("total should be omitted when nil")
	}
	if containsKey(output, "has_more") {
		t.Error("has_more should be omitted when false")
	}
	if containsKey(output, "suggestions") {
		t.Error("suggestions should be omitted when nil")
	}
	if containsKey(output, "warnings") {
		t.Error("warnings should be omitted when nil")
	}
	if containsKey(output, "duration") {
		t.Error("duration should be omitted when empty")
	}
	if containsKey(output, "links") {
		t.Error("links should be omitted when nil")
	}
}

func TestPrintError(t *testing.T) {
	var buf bytes.Buffer
	detail := &ErrorDetail{
		Code:       "auth_required",
		Message:    "Authentication failed",
		Operation:  "get workflows",
		StatusCode: 401,
		RequestID:  "req-abc-123",
		Suggestions: []string{
			"Run 'dtctl auth login' to refresh your token",
		},
	}

	if err := PrintError(&buf, detail); err != nil {
		t.Fatalf("PrintError failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if resp.OK {
		t.Error("expected ok=false for error")
	}
	if resp.Result != nil {
		t.Error("expected no result for error")
	}
	if resp.Error == nil {
		t.Fatal("expected error detail")
	}
	if resp.Error.Code != "auth_required" {
		t.Errorf("expected code=auth_required, got %q", resp.Error.Code)
	}
	if resp.Error.StatusCode != 401 {
		t.Errorf("expected status_code=401, got %d", resp.Error.StatusCode)
	}
	if len(resp.Error.Suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(resp.Error.Suggestions))
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{400, "bad_request"},
		{401, "auth_required"},
		{403, "permission_denied"},
		{404, "not_found"},
		{409, "conflict"},
		{429, "rate_limited"},
		{500, "server_error"},
		{502, "server_error"},
		{503, "server_error"},
		{504, "server_error"},
		{418, "error"}, // Unknown 4xx
		{200, "error"}, // Success codes (shouldn't be used but return generic)
		{0, "error"},   // Zero
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := ClassifyHTTPError(tt.status)
			if got != tt.want {
				t.Errorf("ClassifyHTTPError(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestAgentPrinter_PrintList_WithoutSetTotal(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)

	items := []map[string]string{
		{"id": "1", "title": "WF 1"},
		{"id": "2", "title": "WF 2"},
	}

	// Do NOT call SetTotal — total should be omitted from output
	if err := p.PrintList(items); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Context.Total != nil {
		t.Errorf("expected total to be nil (omitted), got %d", *resp.Context.Total)
	}
}

func TestAgentPrinter_Print_ResultAlwaysPresent(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{Verb: "get"})

	// Print nil result — should still have "result" key in JSON (as null)
	if err := p.Print(nil); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if _, exists := m["result"]; !exists {
		t.Error("expected 'result' key to be present in output even when value is nil")
	}
}

// containsKey checks if a JSON output contains a specific key.
func containsKey(jsonOutput, key string) bool {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOutput), &m); err != nil {
		return false
	}
	ctx, ok := m["context"].(map[string]interface{})
	if !ok {
		return false
	}
	_, exists := ctx[key]
	return exists
}
