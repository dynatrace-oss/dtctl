package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONLPrinter_PrintList(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONLPrinter{writer: &buf}

	records := []map[string]interface{}{
		{"host": "web-01", "status": 200},
		{"host": "web-02", "status": 500, "nested": map[string]interface{}{"a": 1}},
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), buf.String())
	}

	// Each line must be independently valid, compact JSON (no indentation).
	for i, line := range lines {
		if strings.Contains(line, "\n") {
			t.Errorf("line %d contains a newline", i)
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line %d not valid JSON: %v (%q)", i, err, line)
		}
	}

	// Nested values are preserved on the second line.
	var second map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal line 2: %v", err)
	}
	if _, ok := second["nested"].(map[string]interface{}); !ok {
		t.Errorf("nested object not preserved: %#v", second["nested"])
	}
}

func TestJSONLPrinter_Empty(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONLPrinter{writer: &buf}
	if err := p.PrintList([]map[string]interface{}{}); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty slice, got %q", buf.String())
	}
}

func TestJSONLPrinter_Print(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONLPrinter{writer: &buf}
	if err := p.Print(map[string]interface{}{"a": 1}); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if got := buf.String(); got != "{\"a\":1}\n" {
		t.Errorf("got %q, want a single compact JSON line", got)
	}
}

func TestJSONLPrinter_NonSlice(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONLPrinter{writer: &buf}
	if err := p.PrintList(42); err == nil {
		t.Error("expected error for non-slice input")
	}
}
