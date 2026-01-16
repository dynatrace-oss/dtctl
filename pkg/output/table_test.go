package output

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Test struct with table tags
type TestResource struct {
	Name        string `table:"NAME"`
	ID          string `table:"ID"`
	Status      string `table:"STATUS"`
	Description string `table:"DESCRIPTION,wide"`
	Internal    string `table:"-"`
	hidden      string // unexported, should be ignored
}

// Test struct without table tags
type SimpleResource struct {
	Name   string
	Value  int
	Active bool
}

func TestGetTableFields_WithTags(t *testing.T) {
	typ := reflect.TypeOf(TestResource{})

	// Normal mode - should exclude wide-only and hidden fields
	fields := getTableFields(typ, false)
	if len(fields) != 3 {
		t.Errorf("expected 3 fields in normal mode, got %d", len(fields))
	}

	fieldNames := make(map[string]bool)
	for _, f := range fields {
		fieldNames[f.name] = true
	}

	if !fieldNames["NAME"] || !fieldNames["ID"] || !fieldNames["STATUS"] {
		t.Errorf("missing expected fields, got: %v", fieldNames)
	}
	if fieldNames["DESCRIPTION"] {
		t.Error("wide-only field should not be included in normal mode")
	}
}

func TestGetTableFields_WideMode(t *testing.T) {
	typ := reflect.TypeOf(TestResource{})

	// Wide mode - should include wide-only fields
	fields := getTableFields(typ, true)
	if len(fields) != 4 {
		t.Errorf("expected 4 fields in wide mode, got %d", len(fields))
	}

	fieldNames := make(map[string]bool)
	for _, f := range fields {
		fieldNames[f.name] = true
	}

	if !fieldNames["DESCRIPTION"] {
		t.Error("wide-only field should be included in wide mode")
	}
}

func TestGetTableFields_NoTags(t *testing.T) {
	typ := reflect.TypeOf(SimpleResource{})

	// Without tags, all exported fields should be shown
	fields := getTableFields(typ, false)
	if len(fields) != 3 {
		t.Errorf("expected 3 fields for struct without tags, got %d", len(fields))
	}
}

func TestTablePrinter_Print_Struct(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf, wide: false}

	resource := TestResource{
		Name:        "test-resource",
		ID:          "123",
		Status:      "active",
		Description: "A test resource",
		Internal:    "internal-data",
	}

	err := p.Print(resource)
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()

	// Should contain headers and values
	if !strings.Contains(output, "NAME") {
		t.Error("output missing NAME header")
	}
	if !strings.Contains(output, "test-resource") {
		t.Error("output missing name value")
	}
	// Should NOT contain wide-only field
	if strings.Contains(output, "DESCRIPTION") {
		t.Error("output should not contain wide-only DESCRIPTION in normal mode")
	}
	// Should NOT contain hidden field
	if strings.Contains(output, "internal-data") {
		t.Error("output should not contain hidden field data")
	}
}

func TestTablePrinter_Print_NonStruct(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	// Non-struct should just print the value
	err := p.Print("simple string value")
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "simple string value") {
		t.Errorf("output should contain the string value, got: %s", output)
	}
}

func TestTablePrinter_Print_Pointer(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	resource := &TestResource{
		Name:   "ptr-resource",
		ID:     "456",
		Status: "pending",
	}

	err := p.Print(resource)
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ptr-resource") {
		t.Errorf("output missing pointer resource data, got: %s", output)
	}
}

func TestTablePrinter_PrintList_Structs(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	resources := []TestResource{
		{Name: "resource1", ID: "1", Status: "active"},
		{Name: "resource2", ID: "2", Status: "pending"},
		{Name: "resource3", ID: "3", Status: "failed"},
	}

	err := p.PrintList(resources)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "resource1") || !strings.Contains(output, "resource2") || !strings.Contains(output, "resource3") {
		t.Errorf("output missing resources, got: %s", output)
	}
}

func TestTablePrinter_PrintList_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	err := p.PrintList([]TestResource{})
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No resources found") {
		t.Errorf("expected 'No resources found' message, got: %s", output)
	}
}

func TestTablePrinter_PrintList_NonSlice(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	err := p.PrintList("not a slice")
	if err == nil {
		t.Error("expected error for non-slice input")
	}
}

func TestTablePrinter_PrintList_Maps(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	maps := []map[string]interface{}{
		{"name": "item1", "count": 10},
		{"name": "item2", "count": 20},
	}

	err := p.PrintList(maps)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	// Headers should be uppercase
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "COUNT") {
		t.Errorf("output missing headers, got: %s", output)
	}
	if !strings.Contains(output, "item1") || !strings.Contains(output, "item2") {
		t.Errorf("output missing values, got: %s", output)
	}
}

func TestTablePrinter_PrintList_SimpleValues(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	values := []string{"value1", "value2", "value3"}

	err := p.PrintList(values)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "value1") {
		t.Errorf("output missing values, got: %s", output)
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "string",
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "int",
			value:    42,
			expected: "42",
		},
		{
			name:     "bool true",
			value:    true,
			expected: "true",
		},
		{
			name:     "bool false",
			value:    false,
			expected: "false",
		},
		{
			name:     "nil pointer",
			value:    (*string)(nil),
			expected: "",
		},
		{
			name:     "empty slice",
			value:    []string{},
			expected: "",
		},
		{
			name:     "slice with items",
			value:    []string{"a", "b", "c"},
			expected: "<3 items>",
		},
		{
			name:     "empty map",
			value:    map[string]string{},
			expected: "",
		},
		{
			name:     "map with items",
			value:    map[string]string{"a": "1", "b": "2"},
			expected: "<2 items>",
		},
		{
			name:     "zero time",
			value:    time.Time{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatValue(reflect.ValueOf(tt.value))
			if result != tt.expected {
				t.Errorf("formatValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatValue_Time(t *testing.T) {
	// Test non-zero time
	tm := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := formatValue(reflect.ValueOf(tm))

	if !strings.Contains(result, "2024-01-15") {
		t.Errorf("formatValue(time) = %q, expected date format", result)
	}
}

func TestFormatTableMapValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "nil",
			value:    nil,
			expected: "",
		},
		{
			name:     "string",
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "int",
			value:    123,
			expected: "123",
		},
		{
			name:     "empty map",
			value:    map[string]string{},
			expected: "",
		},
		{
			name:     "map with items",
			value:    map[string]int{"a": 1, "b": 2},
			expected: "<2 items>",
		},
		{
			name:     "small slice",
			value:    []string{"a", "b"},
			expected: "a, b",
		},
		{
			name:     "large slice",
			value:    []string{"a", "b", "c", "d", "e"},
			expected: "<5 items>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTableMapValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatTableMapValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTablePrinter_WideMode(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf, wide: true}

	resource := TestResource{
		Name:        "test-resource",
		ID:          "123",
		Status:      "active",
		Description: "A test resource with description",
	}

	err := p.Print(resource)
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	output := buf.String()

	// Wide mode should include DESCRIPTION
	if !strings.Contains(output, "DESCRIPTION") {
		t.Error("wide mode output missing DESCRIPTION header")
	}
	if !strings.Contains(output, "A test resource with description") {
		t.Error("wide mode output missing description value")
	}
}

func TestTablePrinter_PrintList_PointerSlice(t *testing.T) {
	var buf bytes.Buffer
	p := &TablePrinter{writer: &buf}

	resources := []*TestResource{
		{Name: "ptr1", ID: "1", Status: "active"},
		{Name: "ptr2", ID: "2", Status: "pending"},
	}

	err := p.PrintList(resources)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ptr1") || !strings.Contains(output, "ptr2") {
		t.Errorf("output missing pointer resources, got: %s", output)
	}
}
