package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestJQPrinter_JsonFormat(t *testing.T) {
	var buf bytes.Buffer
	base := NewPrinterWithWriter("json", &buf)
	p := NewJQPrinter(base, ".name")

	in := map[string]interface{}{"name": "alpha", "id": 42}
	if err := p.Print(in); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected filtered value in output, got: %s", out)
	}
	if strings.Contains(out, "id") {
		t.Fatalf("expected id field to be filtered out, got: %s", out)
	}
}

func TestJQPrinter_TableFormat(t *testing.T) {
	var buf bytes.Buffer
	base := NewPrinterWithWriter("table", &buf)
	p := NewJQPrinter(base, ".[] | {name: .name}")

	in := []map[string]interface{}{
		{"name": "alpha", "id": "1"},
		{"name": "beta", "id": "2"},
	}
	if err := p.PrintList(in); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected NAME header in table output, got: %s", out)
	}
	if strings.Contains(out, "ID") {
		t.Fatalf("expected ID column to be filtered out, got: %s", out)
	}
}

func TestJQPrinter_InvalidFilter(t *testing.T) {
	var buf bytes.Buffer
	base := NewPrinterWithWriter("json", &buf)
	p := NewJQPrinter(base, ".[")

	err := p.Print(map[string]interface{}{"name": "alpha"})
	if err == nil {
		t.Fatal("expected invalid jq filter error")
	}
	if !strings.Contains(err.Error(), "invalid --jq filter") {
		t.Fatalf("expected invalid filter error, got: %v", err)
	}
}
