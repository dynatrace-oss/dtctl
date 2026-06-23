package analyzer

import (
	"encoding/json"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// rawByteSeqRE matches the regression signature: a YAML sequence that is really
// a []byte / json.RawMessage rendered element-by-element as integers ("- 123").
var rawByteSeqRE = regexp.MustCompile(`(?m)(?:^[ \t]*-[ \t]+\d+[ \t]*\n){8,}`)

// assertYAMLMatchesJSON marshals v to both JSON and YAML and asserts the YAML
// round-trips to the same structure as the JSON — i.e. YAML output honors the
// JSON tags (json:"-" exclusions, omitempty, camelCase) instead of falling back
// to reflection. It also guards against the raw-byte-sequence regression.
func assertYAMLMatchesJSON(t *testing.T, v any) {
	t.Helper()

	jsonBytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	yamlBytes, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	if rawByteSeqRE.MatchString(string(yamlBytes)) {
		t.Fatalf("value rendered a field as a raw byte sequence (regression):\n%s", yamlBytes)
	}

	var fromJSON, fromYAML any
	if err := json.Unmarshal(jsonBytes, &fromJSON); err != nil {
		t.Fatalf("json round-trip error = %v", err)
	}
	if err := yaml.Unmarshal(yamlBytes, &fromYAML); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}

	// Re-normalize both through JSON so numeric types (yaml int vs json float64)
	// compare equal; we care about structural/key parity, not Go types.
	normJSON, _ := json.Marshal(fromJSON)
	normYAML, _ := json.Marshal(fromYAML)
	if string(normJSON) != string(normYAML) {
		t.Errorf("YAML output does not match JSON output:\n JSON: %s\n YAML: %s", normJSON, normYAML)
	}
}

func TestAnalyzer_MarshalYAML_MatchesJSON(t *testing.T) {
	a := Analyzer{
		Name:         "dt.statistics.GenericForecastAnalyzer",
		DisplayName:  "Generic Forecast",
		Type:         "DAVIS",
		CategoryName: "Forecast", // display-only (json:"-"), must not leak into YAML
	}
	assertYAMLMatchesJSON(t, a)

	yamlBytes, _ := yaml.Marshal(a)
	if rawByteSeqRE.MatchString(string(yamlBytes)) {
		t.Fatalf("unexpected byte sequence:\n%s", yamlBytes)
	}
	var m map[string]any
	if err := yaml.Unmarshal(yamlBytes, &m); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}
	if _, leaked := m["categoryname"]; leaked {
		t.Errorf("display-only CategoryName leaked into YAML: %v", m)
	}
	if _, ok := m["displayName"]; !ok {
		t.Errorf("expected camelCase displayName key in YAML, got: %v", m)
	}
}

// TestAnalyzerDefinition_MarshalYAML_StructuredRawMessage is the key regression
// guard: Input/Output/AnalyzerCall are json.RawMessage ([]byte), which without a
// MarshalYAML render as a list of raw byte values (the dashboard-content bug).
func TestAnalyzerDefinition_MarshalYAML_StructuredRawMessage(t *testing.T) {
	d := AnalyzerDefinition{
		Name:         "dt.statistics.GenericForecastAnalyzer",
		DisplayName:  "Generic Forecast",
		Type:         "DAVIS",
		CategoryName: "Forecast",
		Input:        json.RawMessage(`{"fields":[{"name":"timeSeriesData","type":"timeseries"}]}`),
		Output:       json.RawMessage(`{"fields":[{"name":"forecast","type":"timeseries"}]}`),
	}

	assertYAMLMatchesJSON(t, d)

	yamlBytes, _ := yaml.Marshal(d)
	var m map[string]any
	if err := yaml.Unmarshal(yamlBytes, &m); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}
	input, ok := m["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected input to be a structured mapping, got %T:\n%s", m["input"], yamlBytes)
	}
	if _, ok := input["fields"]; !ok {
		t.Errorf("expected input.fields in YAML, got: %v", input)
	}
}

func TestExecuteResult_MarshalYAML_MatchesJSON(t *testing.T) {
	r := ExecuteResult{
		RequestToken: "tok-123",
		Result:       &AnalyzerResult{ResultID: "res-1", ResultStatus: "OK", ExecutionStatus: "COMPLETED"},
		// Flattened display-only fields (json:"-"); must not leak into YAML.
		ResultID:        "res-1",
		ResultStatus:    "OK",
		ExecutionStatus: "COMPLETED",
	}

	assertYAMLMatchesJSON(t, r)

	yamlBytes, _ := yaml.Marshal(r)
	var m map[string]any
	if err := yaml.Unmarshal(yamlBytes, &m); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}
	for _, leaked := range []string{"resultid", "resultstatus", "executionstatus"} {
		if _, ok := m[leaked]; ok {
			t.Errorf("display-only field %q leaked into YAML: %v", leaked, m)
		}
	}
}
