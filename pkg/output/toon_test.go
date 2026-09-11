package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestToonPrinter_ImplementsPrinterInterface(t *testing.T) {
	var _ Printer = (*ToonPrinter)(nil)
}

func TestToonPrinter_Print_SimpleMap(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	data := map[string]string{"id": "abc-123", "title": "My Workflow"}
	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "abc-123") {
		t.Errorf("output missing id value, got: %s", out)
	}
	if !strings.Contains(out, "My Workflow") {
		t.Errorf("output missing title value, got: %s", out)
	}
}

func TestToonPrinter_PrintList_SliceOfMaps(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	data := []map[string]string{
		{"id": "1", "name": "Alice"},
		{"id": "2", "name": "Bob"},
	}
	if err := p.PrintList(data); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("output missing expected values, got: %s", out)
	}
}

func TestToonPrinter_Print_Struct(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	type item struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	data := item{ID: "abc", Name: "Test Item"}
	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	// JSON field names should be used (not Go field names)
	if !strings.Contains(out, "id") {
		t.Errorf("expected json tag 'id' in output, got: %s", out)
	}
	if !strings.Contains(out, "name") {
		t.Errorf("expected json tag 'name' in output, got: %s", out)
	}
	if !strings.Contains(out, "abc") {
		t.Errorf("output missing id value, got: %s", out)
	}
}

func TestToonPrinter_PrintList_Structs(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	type user struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}

	data := []user{
		{ID: 1, Name: "Alice", Role: "admin"},
		{ID: 2, Name: "Bob", Role: "user"},
	}
	if err := p.PrintList(data); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	out := buf.String()
	// TOON should produce a compact tabular representation for uniform arrays
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("output missing expected values, got: %s", out)
	}
	if !strings.Contains(out, "admin") || !strings.Contains(out, "user") {
		t.Errorf("output missing role values, got: %s", out)
	}
}

func TestToonPrinter_Print_Nil(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	if err := p.Print(nil); err != nil {
		t.Fatalf("Print(nil) failed: %v", err)
	}
}

func TestToonPrinter_PrintList_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	if err := p.PrintList([]string{}); err != nil {
		t.Fatalf("PrintList(empty) failed: %v", err)
	}
}

func TestToonPrinter_Print_NestedStruct(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	type address struct {
		City    string `json:"city"`
		Country string `json:"country"`
	}
	type person struct {
		Name    string  `json:"name"`
		Address address `json:"address"`
	}

	data := person{
		Name:    "Alice",
		Address: address{City: "Vienna", Country: "AT"},
	}

	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Alice") {
		t.Errorf("output missing name, got: %s", out)
	}
	if !strings.Contains(out, "Vienna") {
		t.Errorf("output missing city, got: %s", out)
	}
}

func TestToonPrinter_OutputIsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	p := &ToonPrinter{writer: &buf}

	data := map[string]string{"key": "value"}
	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	// TOON output should NOT be valid JSON
	var v interface{}
	if err := json.Unmarshal([]byte(out), &v); err == nil {
		// It parsed as valid JSON — that's unexpected for TOON format
		// (unless it's a trivial scalar, which map isn't)
		t.Errorf("expected TOON output (not JSON), got: %s", out)
	}
}

func TestToGeneric_PreservesJSONTags(t *testing.T) {
	type item struct {
		MyField string `json:"my_field"`
		Other   int    `json:"other_name"`
	}

	data := item{MyField: "hello", Other: 42}
	generic, err := toGeneric(data)
	if err != nil {
		t.Fatalf("toGeneric failed: %v", err)
	}

	m, ok := generic.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", generic)
	}

	if _, exists := m["my_field"]; !exists {
		t.Error("expected key 'my_field' from json tag")
	}
	if _, exists := m["other_name"]; !exists {
		t.Error("expected key 'other_name' from json tag")
	}
	// Go field names should NOT be present
	if _, exists := m["MyField"]; exists {
		t.Error("unexpected Go field name 'MyField' in generic representation")
	}
}

func TestToGeneric_OmitsEmptyFields(t *testing.T) {
	type item struct {
		Name  string `json:"name"`
		Value string `json:"value,omitempty"`
	}

	data := item{Name: "test"}
	generic, err := toGeneric(data)
	if err != nil {
		t.Fatalf("toGeneric failed: %v", err)
	}

	m, ok := generic.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", generic)
	}

	if _, exists := m["value"]; exists {
		t.Error("expected 'value' to be omitted (omitempty)")
	}
}

// TestAgentPrinter_DefaultsJSONResult verifies that AgentPrinter uses JSON
// encoding for the result field by default.
func TestAgentPrinter_DefaultsJSONResult(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)

	data := []map[string]string{
		{"id": "1", "name": "WF1"},
		{"id": "2", "name": "WF2"},
	}
	if err := p.PrintList(data); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	// The envelope must still be valid JSON
	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}

	// The result should be a native JSON array (not a TOON string)
	_, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatalf("expected result to be a JSON array, got %T", resp.Result)
	}
}

// TestAgentPrinter_SetResultFormatToon verifies that explicitly setting result
// format to "toon" makes the result field a TOON-encoded string.
func TestAgentPrinter_SetResultFormatToon(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)
	p.SetResultFormat("toon")

	data := []map[string]string{
		{"id": "1", "name": "WF1"},
		{"id": "2", "name": "WF2"},
	}
	if err := p.PrintList(data); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	if !resp.OK {
		t.Error("expected ok=true")
	}

	// The result should be a string (TOON-encoded)
	resultStr, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("expected result to be a TOON string, got %T", resp.Result)
	}

	if !strings.Contains(resultStr, "WF1") || !strings.Contains(resultStr, "WF2") {
		t.Errorf("TOON result missing expected values, got: %s", resultStr)
	}
}

// TestAgentPrinter_SetResultFormatJSON verifies that the default (JSON) result
// format produces a native JSON value in the result field.
func TestAgentPrinter_SetResultFormatJSON(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)
	p.SetResultFormat("json")

	data := map[string]string{"id": "abc-123", "title": "My Workflow"}
	if err := p.Print(data); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}

	// Result should be a native JSON object (map), not a string
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result to be a map (JSON object), got %T", resp.Result)
	}
	if result["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", result["id"])
	}
}

// TestAgentPrinter_NilDataStaysNil verifies that nil data is not TOON-encoded.
func TestAgentPrinter_NilDataStaysNil(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{})

	if err := p.Print(nil); err != nil {
		t.Fatalf("Print(nil) failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if m["result"] != nil {
		t.Errorf("expected null result for nil data, got %v", m["result"])
	}
}

// TestAgentPrinter_SetResultFormatUnsupported verifies that a format the
// envelope cannot carry (csv/yaml/table) still produces native JSON, but says
// so in context.warnings instead of downgrading silently.
func TestAgentPrinter_SetResultFormatUnsupported(t *testing.T) {
	for _, format := range []string{"csv", "yaml", "table", "wide", "parquet"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
			p := NewAgentPrinter(&buf, ctx)
			p.SetResultFormat(format)

			if err := p.PrintList([]map[string]string{{"id": "1"}}); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}

			var resp Response
			if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
				t.Fatalf("envelope is not valid JSON: %v", err)
			}
			if _, isString := resp.Result.(string); isString {
				t.Fatal("result should fall back to native JSON, not an encoded string")
			}
			if resp.Context == nil || len(resp.Context.Warnings) == 0 {
				t.Fatalf("expected a warning naming the unsupported format, got %+v", resp.Context)
			}
			if !strings.Contains(resp.Context.Warnings[0], format) {
				t.Errorf("warning should name the requested format %q, got %q", format, resp.Context.Warnings[0])
			}
		})
	}
}

// TestAgentPrinter_SetResultFormatEmptyIsSilent verifies that an unset format
// is not treated as an unsupported choice.
func TestAgentPrinter_SetResultFormatEmptyIsSilent(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ResponseContext{Verb: "get", Resource: "workflow"}
	p := NewAgentPrinter(&buf, ctx)
	p.SetResultFormat("")

	if err := p.PrintList([]map[string]string{{"id": "1"}}); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if resp.Context != nil && len(resp.Context.Warnings) > 0 {
		t.Errorf("empty format should not warn, got %v", resp.Context.Warnings)
	}
}
