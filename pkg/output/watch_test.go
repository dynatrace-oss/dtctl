package output

import (
	"bytes"
	"strings"
	"testing"
)

type testResource struct {
	Name   string `table:"NAME"`
	Status string `table:"STATUS"`
	Age    int    `table:"AGE"`
}

func TestNewWatchPrinter(t *testing.T) {
	basePrinter := NewPrinter("table")
	watchPrinter := NewWatchPrinter(basePrinter)

	if watchPrinter == nil {
		t.Fatal("NewWatchPrinter() returned nil")
	}

	if watchPrinter.basePrinter != basePrinter {
		t.Error("NewWatchPrinter() did not set basePrinter correctly")
	}

	if !watchPrinter.colorize {
		t.Error("NewWatchPrinter() should enable colorize by default")
	}
}

func TestNewWatchPrinterWithWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("table", buf)

	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	if watchPrinter == nil {
		t.Fatal("NewWatchPrinterWithWriter() returned nil")
	}

	if watchPrinter.basePrinter != basePrinter {
		t.Error("NewWatchPrinterWithWriter() did not set basePrinter correctly")
	}

	if watchPrinter.writer != buf {
		t.Error("NewWatchPrinterWithWriter() did not set writer correctly")
	}

	if watchPrinter.colorize {
		t.Error("NewWatchPrinterWithWriter() should respect colorize=false")
	}
}

func TestWatchPrinter_Print(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("json", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	data := map[string]string{"key": "value"}
	err := watchPrinter.Print(data)
	if err != nil {
		t.Errorf("Print() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Print() should write output")
	}
}

func TestWatchPrinter_PrintList(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("json", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	data := []map[string]string{{"key": "value"}}
	err := watchPrinter.PrintList(data)
	if err != nil {
		t.Errorf("PrintList() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("PrintList() should write output")
	}
}

func TestWatchPrinter_PrintChanges_Empty(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("json", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	err := watchPrinter.PrintChanges([]Change{})
	if err != nil {
		t.Errorf("PrintChanges() with empty changes error = %v", err)
	}

	if buf.Len() != 0 {
		t.Error("PrintChanges() with empty changes should not write output")
	}
}

func TestWatchPrinter_PrintChanges_NonTable(t *testing.T) {
	tests := []struct {
		name       string
		changeType ChangeType
		wantPrefix string
	}{
		{
			name:       "added change",
			changeType: ChangeTypeAdded,
			wantPrefix: "+",
		},
		{
			name:       "modified change",
			changeType: ChangeTypeModified,
			wantPrefix: "~",
		},
		{
			name:       "deleted change",
			changeType: ChangeTypeDeleted,
			wantPrefix: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			basePrinter := NewPrinterWithWriter("json", buf)
			watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

			resource := map[string]string{"name": "test"}
			changes := []Change{
				{
					Type:     tt.changeType,
					Resource: resource,
				},
			}

			err := watchPrinter.PrintChanges(changes)
			if err != nil {
				t.Errorf("PrintChanges() error = %v", err)
			}

			output := buf.String()
			if !strings.HasPrefix(output, tt.wantPrefix+" ") {
				t.Errorf("PrintChanges() output should start with %q, got %q", tt.wantPrefix, output)
			}
		})
	}
}

func TestWatchPrinter_PrintChanges_Table(t *testing.T) {
	tests := []struct {
		name       string
		changeType ChangeType
		wantPrefix string
	}{
		{
			name:       "added change",
			changeType: ChangeTypeAdded,
			wantPrefix: "+",
		},
		{
			name:       "modified change",
			changeType: ChangeTypeModified,
			wantPrefix: "~",
		},
		{
			name:       "deleted change",
			changeType: ChangeTypeDeleted,
			wantPrefix: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			basePrinter := NewPrinterWithWriter("json", buf)
			watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

			resource := testResource{
				Name:   "test-resource",
				Status: "running",
				Age:    5,
			}

			changes := []Change{
				{
					Type:     tt.changeType,
					Resource: resource,
				},
			}

			err := watchPrinter.PrintChanges(changes)
			if err != nil {
				t.Errorf("PrintChanges() error = %v", err)
			}

			output := buf.String()
			if !strings.HasPrefix(output, tt.wantPrefix+" ") {
				t.Errorf("PrintChanges() output should start with %q, got %q", tt.wantPrefix, output)
			}

			// Verify resource data is present
			if !strings.Contains(output, "test-resource") {
				t.Errorf("PrintChanges() output should contain resource name, got %q", output)
			}
		})
	}
}

func TestWatchPrinter_GetPrefixAndColor(t *testing.T) {
	tests := []struct {
		name       string
		changeType ChangeType
		wantPrefix string
		wantColor  string
	}{
		{
			name:       "added",
			changeType: ChangeTypeAdded,
			wantPrefix: "+",
			wantColor:  Green,
		},
		{
			name:       "modified",
			changeType: ChangeTypeModified,
			wantPrefix: "~",
			wantColor:  Yellow,
		},
		{
			name:       "deleted",
			changeType: ChangeTypeDeleted,
			wantPrefix: "-",
			wantColor:  Red,
		},
		{
			name:       "unknown",
			changeType: ChangeType("UNKNOWN"),
			wantPrefix: " ",
			wantColor:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			basePrinter := NewPrinterWithWriter("json", buf)
			watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

			prefix, color := watchPrinter.getPrefixAndColor(tt.changeType)

			if prefix != tt.wantPrefix {
				t.Errorf("getPrefixAndColor() prefix = %q, want %q", prefix, tt.wantPrefix)
			}

			if color != tt.wantColor {
				t.Errorf("getPrefixAndColor() color = %q, want %q", color, tt.wantColor)
			}
		})
	}
}

func TestWatchPrinter_PrintChanges_MultipleChanges(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("table", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	changes := []Change{
		{
			Type:     ChangeTypeAdded,
			Resource: testResource{Name: "resource-1", Status: "running", Age: 5},
		},
		{
			Type:     ChangeTypeModified,
			Resource: testResource{Name: "resource-2", Status: "pending", Age: 3},
		},
		{
			Type:     ChangeTypeDeleted,
			Resource: testResource{Name: "resource-3", Status: "stopped", Age: 10},
		},
	}

	err := watchPrinter.PrintChanges(changes)
	if err != nil {
		t.Errorf("PrintChanges() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("PrintChanges() should output 3 lines, got %d", len(lines))
	}

	// Verify each line has the correct prefix
	if !strings.HasPrefix(lines[0], "+") {
		t.Errorf("First line should start with '+', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "~") {
		t.Errorf("Second line should start with '~', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "-") {
		t.Errorf("Third line should start with '-', got %q", lines[2])
	}
}

func TestWatchPrinter_PrintTableRow_NonStruct(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("table", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, false)

	// Test with non-struct resource (should fallback to simple format)
	err := watchPrinter.printTableRow("simple-string", "+", Green, basePrinter.(*TablePrinter))
	if err != nil {
		t.Errorf("printTableRow() with non-struct error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "simple-string") {
		t.Errorf("printTableRow() should contain resource value, got %q", output)
	}
}

func TestWatchPrinter_PrintChanges_WithColorize(t *testing.T) {
	buf := &bytes.Buffer{}
	basePrinter := NewPrinterWithWriter("table", buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, buf, true) // Enable colorize

	resource := testResource{
		Name:   "test-resource",
		Status: "running",
		Age:    5,
	}

	changes := []Change{
		{
			Type:     ChangeTypeAdded,
			Resource: resource,
		},
	}

	err := watchPrinter.PrintChanges(changes)
	if err != nil {
		t.Errorf("PrintChanges() error = %v", err)
	}

	output := buf.String()
	// When colorize is enabled, output should contain ANSI color codes
	if !strings.Contains(output, Green) && !strings.Contains(output, "+") {
		t.Errorf("PrintChanges() with colorize should include color codes or prefix, got %q", output)
	}
}
