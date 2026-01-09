package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSVPrinter_PrintList_Maps(t *testing.T) {
	tests := []struct {
		name     string
		data     []map[string]interface{}
		expected string
	}{
		{
			name: "simple map",
			data: []map[string]interface{}{
				{"name": "John", "age": 30, "city": "NYC"},
				{"name": "Jane", "age": 25, "city": "LA"},
			},
			expected: "age,city,name\n30,NYC,John\n25,LA,Jane\n",
		},
		{
			name: "map with missing values",
			data: []map[string]interface{}{
				{"name": "John", "age": 30},
				{"name": "Jane", "city": "LA"},
			},
			expected: "age,city,name\n30,,John\n,LA,Jane\n",
		},
		{
			name: "map with special characters",
			data: []map[string]interface{}{
				{"name": "John, Jr.", "desc": "Developer \"Senior\""},
			},
			expected: "desc,name\n\"Developer \"\"Senior\"\"\",\"John, Jr.\"\n",
		},
		{
			name:     "empty slice",
			data:     []map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := &CSVPrinter{writer: &buf}

			err := printer.PrintList(tt.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if buf.String() != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, buf.String())
			}
		})
	}
}

func TestCSVPrinter_PrintList_InterfaceMaps(t *testing.T) {
	// Test with []interface{} containing maps (common DQL result format)
	data := []interface{}{
		map[string]interface{}{"name": "John", "age": 30},
		map[string]interface{}{"name": "Jane", "age": 25},
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "age,name\n30,John\n25,Jane\n"
	if buf.String() != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, buf.String())
	}
}

func TestCSVPrinter_PrintList_Structs(t *testing.T) {
	type Person struct {
		Name string `table:"NAME"`
		Age  int    `table:"AGE"`
		City string `table:"CITY,wide"`
	}

	data := []Person{
		{Name: "John", Age: 30, City: "NYC"},
		{Name: "Jane", Age: 25, City: "LA"},
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CSV should include all fields (wide mode)
	result := buf.String()
	if !strings.Contains(result, "NAME") || !strings.Contains(result, "AGE") || !strings.Contains(result, "CITY") {
		t.Errorf("expected all headers (NAME, AGE, CITY) in output:\n%s", result)
	}
	if !strings.Contains(result, "John") || !strings.Contains(result, "Jane") {
		t.Errorf("expected data rows in output:\n%s", result)
	}
}

func TestCSVPrinter_Print_SingleMap(t *testing.T) {
	data := map[string]interface{}{
		"name": "John",
		"age":  30,
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.Print(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := buf.String()
	if !strings.Contains(result, "age,name") {
		t.Errorf("expected header in output:\n%s", result)
	}
	if !strings.Contains(result, "30,John") {
		t.Errorf("expected data row in output:\n%s", result)
	}
}

func TestCSVPrinter_PrintList_ComplexValues(t *testing.T) {
	data := []map[string]interface{}{
		{
			"name":  "John",
			"tags":  []string{"go", "rust"},
			"meta":  map[string]string{"role": "dev"},
			"score": nil,
		},
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := buf.String()
	lines := strings.Split(result, "\n")

	// Should have header and data row
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d", len(lines))
	}

	// Complex values should be formatted as strings
	if !strings.Contains(result, "John") {
		t.Errorf("expected name in output:\n%s", result)
	}
}

func TestCSVPrinter_PrintList_NonSlice(t *testing.T) {
	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList("not a slice")
	if err == nil {
		t.Fatal("expected error for non-slice input")
	}
	if !strings.Contains(err.Error(), "expected slice") {
		t.Errorf("expected 'expected slice' error, got: %v", err)
	}
}
