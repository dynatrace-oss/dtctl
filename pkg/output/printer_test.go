package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewPrinter(t *testing.T) {
	// NewPrinter should return a non-nil printer for any format
	formats := []string{"json", "yaml", "yml", "csv", "table", "wide", "chart", "sparkline", "spark", "barchart", "bar", "braille", "br", "unknown"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			p := NewPrinter(format)
			if p == nil {
				t.Errorf("NewPrinter(%q) returned nil", format)
			}
		})
	}
}

func TestNewPrinterWithWriter(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter("json", &buf)
	if p == nil {
		t.Error("NewPrinterWithWriter returned nil")
	}
}

func TestNewPrinterWithOptions_PlainMode(t *testing.T) {
	var buf bytes.Buffer

	// In plain mode, table format should be converted to json
	p := NewPrinterWithOptions("table", &buf, true)

	// Verify it's a JSON printer by printing something
	data := map[string]string{"key": "value"}
	err := p.Print(data)
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"key"`) || !strings.Contains(output, `"value"`) {
		t.Errorf("expected JSON output in plain mode, got: %s", output)
	}
}

func TestNewPrinterWithOpts_NilWriter(t *testing.T) {
	// Should not panic with nil writer (defaults to stdout)
	p := NewPrinterWithOpts(PrinterOptions{
		Format: "json",
		Writer: nil,
	})
	if p == nil {
		t.Error("NewPrinterWithOpts with nil writer returned nil")
	}
}

func TestNewPrinterWithOpts_ChartDimensions(t *testing.T) {
	var buf bytes.Buffer

	tests := []struct {
		name   string
		opts   PrinterOptions
		format string
	}{
		{
			name: "chart with custom dimensions",
			opts: PrinterOptions{
				Format: "chart",
				Writer: &buf,
				Width:  100,
				Height: 20,
			},
		},
		{
			name: "sparkline with custom width",
			opts: PrinterOptions{
				Format: "sparkline",
				Writer: &buf,
				Width:  80,
			},
		},
		{
			name: "barchart with custom width",
			opts: PrinterOptions{
				Format: "barchart",
				Writer: &buf,
				Width:  100,
			},
		},
		{
			name: "braille with custom dimensions",
			opts: PrinterOptions{
				Format: "braille",
				Writer: &buf,
				Width:  80,
				Height: 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPrinterWithOpts(tt.opts)
			if p == nil {
				t.Error("NewPrinterWithOpts returned nil")
			}
		})
	}
}

func TestJSONPrinter_Print(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{writer: &buf}

	tests := []struct {
		name     string
		input    interface{}
		contains []string
	}{
		{
			name:     "simple map",
			input:    map[string]string{"name": "test"},
			contains: []string{`"name"`, `"test"`},
		},
		{
			name:     "nested struct",
			input:    struct{ ID int }{ID: 123},
			contains: []string{`"ID"`, `123`},
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			contains: []string{`1`, `2`, `3`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			err := p.Print(tt.input)
			if err != nil {
				t.Fatalf("Print failed: %v", err)
			}

			output := buf.String()
			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q, got: %s", want, output)
				}
			}
		})
	}
}

func TestJSONPrinter_PrintList(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{writer: &buf}

	input := []map[string]string{
		{"name": "item1"},
		{"name": "item2"},
	}

	err := p.PrintList(input)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "item1") || !strings.Contains(output, "item2") {
		t.Errorf("output missing items, got: %s", output)
	}
}

func TestYAMLPrinter_Print(t *testing.T) {
	var buf bytes.Buffer
	p := &YAMLPrinter{writer: &buf}

	input := map[string]string{"name": "test", "value": "hello"}

	err := p.Print(input)
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()
	// YAML output should have key: value format
	if !strings.Contains(output, "name:") || !strings.Contains(output, "test") {
		t.Errorf("expected YAML output, got: %s", output)
	}
}

func TestYAMLPrinter_PrintList(t *testing.T) {
	var buf bytes.Buffer
	p := &YAMLPrinter{writer: &buf}

	input := []string{"item1", "item2", "item3"}

	err := p.PrintList(input)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "item1") {
		t.Errorf("output missing items, got: %s", output)
	}
}

func TestPrinterFormats(t *testing.T) {
	// Test that all format aliases work correctly
	tests := []struct {
		format       string
		expectFormat string
	}{
		{"json", "json"},
		{"yaml", "yaml"},
		{"yml", "yaml"},
		{"csv", "csv"},
		{"table", "table"},
		{"wide", "wide"},
		{"chart", "chart"},
		{"sparkline", "sparkline"},
		{"spark", "sparkline"},
		{"barchart", "barchart"},
		{"bar", "barchart"},
		{"braille", "braille"},
		{"br", "braille"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinterWithWriter(tt.format, &buf)
			if p == nil {
				t.Errorf("NewPrinterWithWriter(%q) returned nil", tt.format)
			}
		})
	}
}
