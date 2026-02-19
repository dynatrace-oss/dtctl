package output

import (
	"bytes"
	"strings"
	"testing"
)

// Test embedded struct handling
type EmbeddedBase struct {
	BaseField1 string `json:"baseField1" table:"BASE1"`
	BaseField2 string `json:"baseField2" table:"BASE2"`
	HiddenBase string `json:"hiddenBase" table:"-"`
}

type EmbeddedWrapper struct {
	EmbeddedBase
	WrapperField string  `json:"wrapperField" table:"WRAPPER"`
	Score        float64 `json:"score" table:"SCORE"`
}

func TestTablePrinter_EmbeddedStructs(t *testing.T) {
	tests := []struct {
		name     string
		data     []EmbeddedWrapper
		expected []string // Expected column headers
	}{
		{
			name: "embedded struct fields shown",
			data: []EmbeddedWrapper{
				{
					EmbeddedBase: EmbeddedBase{
						BaseField1: "base1",
						BaseField2: "base2",
						HiddenBase: "hidden",
					},
					WrapperField: "wrapper",
					Score:        95.5,
				},
				{
					EmbeddedBase: EmbeddedBase{
						BaseField1: "base1-2",
						BaseField2: "base2-2",
						HiddenBase: "hidden2",
					},
					WrapperField: "wrapper2",
					Score:        100.0,
				},
			},
			expected: []string{"BASE1", "BASE2", "WRAPPER", "SCORE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := &TablePrinter{writer: &buf, wide: false}

			err := printer.PrintList(tt.data)
			if err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}

			output := buf.String()
			t.Logf("Output:\n%s", output)

			// Check that all expected headers are present
			for _, header := range tt.expected {
				if !strings.Contains(output, header) {
					t.Errorf("Expected header %q not found in output", header)
				}
			}

			// Check that hidden field is not shown
			if strings.Contains(output, "HIDDENBASE") {
				t.Errorf("Hidden field should not be in output")
			}

			// Check that data values are present
			if !strings.Contains(output, "base1") {
				t.Errorf("Expected data value 'base1' not found")
			}
			if !strings.Contains(output, "wrapper") {
				t.Errorf("Expected data value 'wrapper' not found")
			}
			if !strings.Contains(output, "95.5") {
				t.Errorf("Expected data value '95.5' not found")
			}
		})
	}
}

func TestTablePrinter_EmbeddedStructs_WideMode(t *testing.T) {
	type EmbeddedWithWide struct {
		BaseField string `json:"baseField" table:"BASE"`
		WideField string `json:"wideField" table:"WIDE,wide"`
	}

	type WrapperWithWide struct {
		EmbeddedWithWide
		NormalField string `json:"normalField" table:"NORMAL"`
	}

	data := []WrapperWithWide{
		{
			EmbeddedWithWide: EmbeddedWithWide{
				BaseField: "base",
				WideField: "wide",
			},
			NormalField: "normal",
		},
	}

	// Test normal mode - wide field should not appear
	var buf bytes.Buffer
	printer := &TablePrinter{writer: &buf, wide: false}
	err := printer.PrintList(data)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "WIDE") {
		t.Errorf("Wide field should not appear in normal mode")
	}
	if !strings.Contains(output, "BASE") {
		t.Errorf("Base field should appear in normal mode")
	}

	// Test wide mode - wide field should appear
	buf.Reset()
	printer = &TablePrinter{writer: &buf, wide: true}
	err = printer.PrintList(data)
	if err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}

	output = buf.String()
	if !strings.Contains(output, "WIDE") {
		t.Errorf("Wide field should appear in wide mode")
	}
	if !strings.Contains(output, "BASE") {
		t.Errorf("Base field should appear in wide mode")
	}
}
