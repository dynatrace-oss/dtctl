package format

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    Format
		wantErr bool
	}{
		{
			name:  "detect JSON object",
			input: []byte(`{"key": "value"}`),
			want:  FormatJSON,
		},
		{
			name:  "detect JSON array",
			input: []byte(`["item1", "item2"]`),
			want:  FormatJSON,
		},
		{
			name: "detect YAML",
			input: []byte(`key: value
another: item`),
			want: FormatYAML,
		},
		{
			name:    "empty data",
			input:   []byte(``),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DetectFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYAMLToJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:  "simple YAML to JSON",
			input: []byte("key: value\nnumber: 42"),
		},
		{
			name:  "nested YAML to JSON",
			input: []byte("parent:\n  child: value\n  list:\n    - item1\n    - item2"),
		},
		{
			name:    "invalid YAML",
			input:   []byte("key: [unclosed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := YAMLToJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("YAMLToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Error("YAMLToJSON() returned nil without error")
			}
			// Verify output is valid JSON
			if !tt.wantErr {
				var js interface{}
				if err := json.Unmarshal(got, &js); err != nil {
					t.Errorf("YAMLToJSON() produced invalid JSON: %v", err)
				}
			}
		})
	}
}

func TestJSONToYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:  "simple JSON to YAML",
			input: []byte(`{"key":"value","number":42}`),
		},
		{
			name:  "nested JSON to YAML",
			input: []byte(`{"parent":{"child":"value","list":["item1","item2"]}}`),
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{"key": invalid}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONToYAML(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONToYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Error("JSONToYAML() returned nil without error")
			}
			// Verify output is valid YAML
			if !tt.wantErr {
				var y interface{}
				if err := yaml.Unmarshal(got, &y); err != nil {
					t.Errorf("JSONToYAML() produced invalid YAML: %v", err)
				}
			}
		})
	}
}

func TestValidateAndConvert(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:  "valid JSON",
			input: []byte(`{"key":"value"}`),
		},
		{
			name:  "valid YAML",
			input: []byte("key: value"),
		},
		{
			name:    "invalid data",
			input:   []byte(`not valid json or yaml: [{{`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAndConvert(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndConvert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Result should always be valid JSON
				var js interface{}
				if err := json.Unmarshal(got, &js); err != nil {
					t.Errorf("ValidateAndConvert() produced invalid JSON: %v", err)
				}
			}
		})
	}
}

func TestMultilineStringsInYAML(t *testing.T) {
	// Test that multiline strings use literal block style
	jsonData := []byte(`{"markdown": "# Hello\n\nWorld", "simple": "no newlines"}`)

	result, err := JSONToYAML(jsonData)
	if err != nil {
		t.Fatalf("JSONToYAML failed: %v", err)
	}

	t.Logf("Result:\n%s", string(result))

	// The multiline string should use literal block style (|)
	resultStr := string(result)
	if !strings.Contains(resultStr, "|") {
		t.Errorf("Expected literal block style (|) for multiline string, got:\n%s", resultStr)
	}

	// Should NOT contain escaped newlines
	if strings.Contains(resultStr, `\n`) {
		t.Errorf("Should not contain escaped newlines, got:\n%s", resultStr)
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that JSON -> YAML -> JSON preserves data
	original := []byte(`{"name":"test","count":42,"tags":["a","b","c"]}`)

	yaml, err := JSONToYAML(original)
	if err != nil {
		t.Fatalf("JSONToYAML() failed: %v", err)
	}

	backToJSON, err := YAMLToJSON(yaml)
	if err != nil {
		t.Fatalf("YAMLToJSON() failed: %v", err)
	}

	// Parse both JSON strings to compare content
	var originalData, finalData interface{}
	if err := json.Unmarshal(original, &originalData); err != nil {
		t.Fatalf("Failed to parse original JSON: %v", err)
	}
	if err := json.Unmarshal(backToJSON, &finalData); err != nil {
		t.Fatalf("Failed to parse final JSON: %v", err)
	}

	// Compare as JSON (order doesn't matter)
	originalJSON, _ := json.Marshal(originalData)
	finalJSON, _ := json.Marshal(finalData)
	if string(originalJSON) != string(finalJSON) {
		t.Errorf("Round trip changed data:\nOriginal: %s\nFinal: %s", originalJSON, finalJSON)
	}
}

func TestPrettyJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:    "compact JSON to pretty",
			input:   []byte(`{"name":"test","count":42,"nested":{"key":"value"}}`),
			wantErr: false,
		},
		{
			name:    "already pretty JSON",
			input:   []byte(`{"name": "test"}`),
			wantErr: false,
		},
		{
			name:    "JSON array",
			input:   []byte(`["item1","item2","item3"]`),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "empty JSON object",
			input:   []byte(`{}`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrettyJSON(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("PrettyJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify output is valid JSON
				var js interface{}
				if err := json.Unmarshal(got, &js); err != nil {
					t.Errorf("PrettyJSON() produced invalid JSON: %v", err)
				}

				// Verify it contains indentation (multiple spaces)
				if !strings.Contains(string(got), "  ") && len(tt.input) > 5 {
					t.Errorf("PrettyJSON() should contain indentation, got: %s", string(got))
				}
			}
		})
	}
}

func TestPrettyYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:    "compact YAML to pretty",
			input:   []byte("name: test\ncount: 42\nnested:\n  key: value"),
			wantErr: false,
		},
		{
			name:    "already pretty YAML",
			input:   []byte("name: test\nkey: value"),
			wantErr: false,
		},
		{
			name:    "YAML list",
			input:   []byte("- item1\n- item2\n- item3"),
			wantErr: false,
		},
		{
			name:    "invalid YAML",
			input:   []byte("key: [unclosed"),
			wantErr: true,
		},
		{
			name:    "empty YAML",
			input:   []byte("{}"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrettyYAML(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("PrettyYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify output is valid YAML
				var y interface{}
				if err := yaml.Unmarshal(got, &y); err != nil {
					t.Errorf("PrettyYAML() produced invalid YAML: %v", err)
				}

				// Verify it has proper structure (contains colons or dashes for non-empty)
				gotStr := string(got)
				if len(tt.input) > 5 && !strings.Contains(gotStr, ":") && !strings.Contains(gotStr, "-") {
					t.Errorf("PrettyYAML() should produce proper YAML structure, got: %s", gotStr)
				}
			}
		})
	}
}

func TestGetExtension(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{
			name:   "json format",
			format: "json",
			want:   ".json",
		},
		{
			name:   "JSON uppercase",
			format: "JSON",
			want:   ".json",
		},
		{
			name:   "yaml format",
			format: "yaml",
			want:   ".yaml",
		},
		{
			name:   "YAML uppercase",
			format: "YAML",
			want:   ".yaml",
		},
		{
			name:   "yml format",
			format: "yml",
			want:   ".yaml",
		},
		{
			name:   "YML uppercase",
			format: "YML",
			want:   ".yaml",
		},
		{
			name:   "unknown format",
			format: "unknown",
			want:   ".txt",
		},
		{
			name:   "empty format",
			format: "",
			want:   ".txt",
		},
		{
			name:   "mixed case yaml",
			format: "YaMl",
			want:   ".yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetExtension(tt.format)

			if got != tt.want {
				t.Errorf("GetExtension(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}
