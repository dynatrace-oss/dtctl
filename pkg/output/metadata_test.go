package output

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{2982690, "2.8 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMillis(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0ms"},
		{47, "47ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{1500, "1.5s"},
		{59999, "60.0s"},
		{60000, "1.0m"},
		{90000, "1.5m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatMillis(tt.input)
			if result != tt.expected {
				t.Errorf("formatMillis(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{42351, "42,351"},
		{1000000, "1,000,000"},
		{-1234, "-1,234"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello world", "hello world"},
		{"newlines", "fetch logs\n| limit 3\n| fields timestamp", "fetch logs | limit 3 | fields timestamp"},
		{"multiple spaces", "foo  bar   baz", "foo bar baz"},
		{"leading trailing", "  hello  ", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseWhitespace(tt.input)
			if result != tt.expected {
				t.Errorf("collapseWhitespace(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMetadataFooter(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		ScannedDataPoints:         0,
		Sampled:                   false,
		QueryID:                   "27c4daf9-2619-4ba1-b1ad-9e276c75a351",
		DQLVersion:                "V1_0",
		Query:                     "fetch logs | limit 3 | fields timestamp",
		CanonicalQuery:            "fetch logs\n| limit 3\n| fields timestamp",
		Timezone:                  "Z",
		Locale:                    "und",
		AnalysisTimeframe: &MetadataTimeframe{
			Start: "2026-03-09T10:16:39.973805659Z",
			End:   "2026-03-09T12:16:39.973805659Z",
		},
		Contributions: &MetadataContribs{
			Buckets: []MetadataBucket{
				{
					Name:                "custom_sen_low_logs_platform_service_shared",
					Table:               "logs",
					ScannedBytes:        2982690,
					MatchedRecordsRatio: 1.0,
				},
			},
		},
	}

	// Disable color for predictable test output
	ResetColorCache()
	SetPlainMode(true)
	defer func() {
		ResetColorCache()
	}()

	result := FormatMetadataFooter(meta, nil)
	expectations := []string{
		"--- Query Metadata ---",
		"Execution time:     47ms",
		"Scanned records:    42,351",
		"Scanned bytes:      2.8 MB",
		"Scanned data pts:   0",
		"Query ID:           27c4daf9-2619-4ba1-b1ad-9e276c75a351",
		"DQL version:        V1_0",
		"Canonical query:    fetch logs | limit 3 | fields timestamp",
		"Query:              fetch logs | limit 3 | fields timestamp",
		"Timezone:           Z",
		"Locale:             und",
		"Sampled:            no",
		"custom_sen_low_logs_platform_service_shared (logs)",
		"scanned: 2.8 MB, matched: 100.0%",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("footer missing expected content %q\nGot:\n%s", exp, result)
		}
	}
}

func TestFormatMetadataFooter_Nil(t *testing.T) {
	result := FormatMetadataFooter(nil, nil)
	if result != "" {
		t.Errorf("expected empty string for nil metadata, got %q", result)
	}
}

func TestFormatMetadataFooter_Sampled(t *testing.T) {
	meta := &QueryMetadata{
		Sampled: true,
	}

	ResetColorCache()
	SetPlainMode(true)
	defer ResetColorCache()

	result := FormatMetadataFooter(meta, nil)
	if !strings.Contains(result, "Sampled:            yes") {
		t.Errorf("expected 'Sampled: yes' in output, got:\n%s", result)
	}
}

func TestFormatMetadataCSVComments(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		ScannedDataPoints:         0,
		Sampled:                   false,
		QueryID:                   "27c4daf9-2619-4ba1-b1ad-9e276c75a351",
		DQLVersion:                "V1_0",
		Query:                     "fetch logs | limit 3 | fields timestamp",
		CanonicalQuery:            "fetch logs\n| limit 3\n| fields timestamp",
		Timezone:                  "Z",
		Locale:                    "und",
		AnalysisTimeframe: &MetadataTimeframe{
			Start: "2026-03-09T10:16:39.973805659Z",
			End:   "2026-03-09T12:16:39.973805659Z",
		},
		Contributions: &MetadataContribs{
			Buckets: []MetadataBucket{
				{
					Name:                "custom_sen_low_logs_platform_service_shared",
					Table:               "logs",
					ScannedBytes:        2982690,
					MatchedRecordsRatio: 1.0,
				},
			},
		},
	}

	result := FormatMetadataCSVComments(meta, nil)
	for _, line := range strings.Split(strings.TrimSpace(result), "\n") {
		if !strings.HasPrefix(line, "#") {
			t.Errorf("CSV comment line does not start with #: %q", line)
		}
	}

	// Verify key content
	expectations := []string{
		"# Query Metadata",
		"# execution_time_ms: 47",
		"# scanned_records: 42351",
		"# scanned_bytes: 2982690",
		"# scanned_data_points: 0",
		"# query_id: 27c4daf9-2619-4ba1-b1ad-9e276c75a351",
		"# dql_version: V1_0",
		"# canonical_query: fetch logs | limit 3 | fields timestamp",
		"# query: fetch logs | limit 3 | fields timestamp",
		"# timezone: Z",
		"# locale: und",
		"# sampled: false",
		"# analysis_start: 2026-03-09T10:16:39.973805659Z",
		"# analysis_end: 2026-03-09T12:16:39.973805659Z",
		"# contribution: custom_sen_low_logs_platform_service_shared (logs, 2982690 bytes, 100.0% matched)",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("CSV comments missing expected content %q\nGot:\n%s", exp, result)
		}
	}
}

func TestFormatMetadataCSVComments_Nil(t *testing.T) {
	result := FormatMetadataCSVComments(nil, nil)
	if result != "" {
		t.Errorf("expected empty string for nil metadata, got %q", result)
	}
}

// --- Field filtering tests ---

func TestParseMetadataFields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
		wantErr  bool
	}{
		{"empty string", "", nil, false},
		{"all", "all", []string{"all"}, false},
		{"single field", "queryId", []string{"queryId"}, false},
		{"multiple fields", "queryId,scannedRecords,scannedBytes", []string{"queryId", "scannedRecords", "scannedBytes"}, false},
		{"with spaces", " queryId , scannedRecords ", []string{"queryId", "scannedRecords"}, false},
		{"trailing comma", "queryId,", []string{"queryId"}, false},
		// Edge cases: absent field selection
		{"only commas", ",,,", nil, false},
		{"single comma", ",", nil, false},
		{"spaces and commas", " , , , ", nil, false},
		// Edge cases: misspelled field names
		{"misspelled field", "queryid", nil, true},
		{"wrong case", "QueryId", nil, true},
		{"completely wrong", "foobar", nil, true},
		{"multiple misspelled", "queryid,scannedrecords", nil, true},
		// Edge cases: mixture of valid and invalid
		{"valid and invalid", "queryId,foobar", nil, true},
		{"valid surrounded by invalid", "bad1,queryId,bad2", nil, true},
		{"valid and misspelled variant", "scannedRecords,scannedrecords", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMetadataFields(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMetadataFields(%q) expected error, got nil", tt.input)
				}
				// Error should mention "unknown metadata field"
				if !strings.Contains(err.Error(), "unknown metadata field") {
					t.Errorf("error should mention 'unknown metadata field', got: %s", err.Error())
				}
				// Error should list valid fields
				if !strings.Contains(err.Error(), "valid fields:") {
					t.Errorf("error should list valid fields, got: %s", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMetadataFields(%q) unexpected error: %v", tt.input, err)
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("ParseMetadataFields(%q) = %v, want %v", tt.input, result, tt.expected)
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ParseMetadataFields(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestParseMetadataFields_ErrorMessages(t *testing.T) {
	// Verify error messages include the specific unknown field names
	_, err := ParseMetadataFields("queryid,foobar")
	if err == nil {
		t.Fatal("expected error for misspelled fields")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "queryid") {
		t.Errorf("error should mention 'queryid', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "foobar") {
		t.Errorf("error should mention 'foobar', got: %s", errMsg)
	}
	// Should list at least one valid field
	if !strings.Contains(errMsg, "queryId") {
		t.Errorf("error should suggest valid field 'queryId', got: %s", errMsg)
	}
}

func TestIsAllFields(t *testing.T) {
	tests := []struct {
		name     string
		fields   []string
		expected bool
	}{
		{"nil", nil, false},
		{"empty", []string{}, false},
		{"all", []string{"all"}, true},
		{"specific fields", []string{"queryId", "scannedRecords"}, false},
		{"all among others", []string{"queryId", "all"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllFields(tt.fields)
			if result != tt.expected {
				t.Errorf("IsAllFields(%v) = %v, want %v", tt.fields, result, tt.expected)
			}
		})
	}
}

func TestFormatMetadataFooter_FilteredFields(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		QueryID:                   "test-id",
		DQLVersion:                "V1_0",
		Timezone:                  "Z",
		Locale:                    "und",
	}

	ResetColorCache()
	SetPlainMode(true)
	defer ResetColorCache()

	fields := []string{"executionTimeMilliseconds", "scannedRecords"}
	result := FormatMetadataFooter(meta, fields)

	// Selected fields should be present
	if !strings.Contains(result, "Execution time:     47ms") {
		t.Error("expected 'Execution time: 47ms' in filtered output")
	}
	if !strings.Contains(result, "Scanned records:    42,351") {
		t.Error("expected 'Scanned records: 42,351' in filtered output")
	}

	// Non-selected fields should be absent
	if strings.Contains(result, "Scanned bytes:") {
		t.Error("'Scanned bytes' should not appear in filtered output")
	}
	if strings.Contains(result, "Query ID:") {
		t.Error("'Query ID' should not appear in filtered output")
	}
	if strings.Contains(result, "DQL version:") {
		t.Error("'DQL version' should not appear in filtered output")
	}
	if strings.Contains(result, "Timezone:") {
		t.Error("'Timezone' should not appear in filtered output")
	}
	if strings.Contains(result, "Locale:") {
		t.Error("'Locale' should not appear in filtered output")
	}
	if strings.Contains(result, "Sampled:") {
		t.Error("'Sampled' should not appear in filtered output")
	}

	// Header should always be present
	if !strings.Contains(result, "--- Query Metadata ---") {
		t.Error("expected header in filtered output")
	}
}

func TestFormatMetadataCSVComments_FilteredFields(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		QueryID:                   "test-id",
		DQLVersion:                "V1_0",
		Timezone:                  "Z",
		Locale:                    "und",
	}

	fields := []string{"queryId", "scannedBytes"}
	result := FormatMetadataCSVComments(meta, fields)

	// Selected fields should be present
	if !strings.Contains(result, "# query_id: test-id") {
		t.Error("expected '# query_id: test-id' in filtered CSV output")
	}
	if !strings.Contains(result, "# scanned_bytes: 2982690") {
		t.Error("expected '# scanned_bytes: 2982690' in filtered CSV output")
	}

	// Non-selected fields should be absent
	if strings.Contains(result, "# execution_time_ms:") {
		t.Error("'execution_time_ms' should not appear in filtered CSV output")
	}
	if strings.Contains(result, "# scanned_records:") {
		t.Error("'scanned_records' should not appear in filtered CSV output")
	}
	if strings.Contains(result, "# timezone:") {
		t.Error("'timezone' should not appear in filtered CSV output")
	}

	// Header should always be present
	if !strings.Contains(result, "# Query Metadata") {
		t.Error("expected header in filtered CSV output")
	}

	// All lines should start with #
	for _, line := range strings.Split(strings.TrimSpace(result), "\n") {
		if !strings.HasPrefix(line, "#") {
			t.Errorf("CSV comment line does not start with #: %q", line)
		}
	}
}

func TestHasField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		fields   []string
		expected bool
	}{
		{"nil fields (show all)", "queryId", nil, true},
		{"empty fields (show all)", "queryId", []string{}, true},
		{"all keyword", "queryId", []string{"all"}, true},
		{"field present", "queryId", []string{"queryId", "scannedRecords"}, true},
		{"field absent", "timezone", []string{"queryId", "scannedRecords"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasField(tt.field, tt.fields)
			if result != tt.expected {
				t.Errorf("hasField(%q, %v) = %v, want %v", tt.field, tt.fields, result, tt.expected)
			}
		})
	}
}

// --- MetadataToMap tests ---

// TestMetadataToMap_Nil verifies nil input returns nil.
func TestMetadataToMap_Nil(t *testing.T) {
	result := MetadataToMap(nil, []string{"queryId"})
	if result != nil {
		t.Errorf("MetadataToMap(nil, ...) should return nil, got %v", result)
	}
}

// TestMetadataToMap_AllFields verifies that "all" returns the original struct
// (for omitempty to suppress genuine zeros).
func TestMetadataToMap_AllFields(t *testing.T) {
	meta := &QueryMetadata{QueryID: "test-id", ScannedRecords: 42}

	result := MetadataToMap(meta, []string{"all"})
	// Should return the original struct, not a map
	if _, ok := result.(*QueryMetadata); !ok {
		t.Errorf("MetadataToMap with 'all' should return *QueryMetadata, got %T", result)
	}
	if result.(*QueryMetadata) != meta {
		t.Error("MetadataToMap with 'all' should return the same pointer")
	}
}

// TestMetadataToMap_EmptyFields verifies nil/empty fields returns original struct.
func TestMetadataToMap_EmptyFields(t *testing.T) {
	meta := &QueryMetadata{QueryID: "test-id"}

	for _, fields := range [][]string{nil, {}} {
		result := MetadataToMap(meta, fields)
		if _, ok := result.(*QueryMetadata); !ok {
			t.Errorf("MetadataToMap with fields=%v should return *QueryMetadata, got %T", fields, result)
		}
	}
}

// TestMetadataToMap_PreservesZeroValues is the critical test for the omitempty fix.
// When a user explicitly selects fields, zero values (false, 0, "") MUST appear
// in the serialized output rather than being suppressed by omitempty.
func TestMetadataToMap_PreservesZeroValues(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 0,     // zero int64
		ScannedRecords:            0,     // zero int64
		ScannedBytes:              0,     // zero int64
		ScannedDataPoints:         0,     // zero int64
		Sampled:                   false, // zero bool
		QueryID:                   "",    // zero string
		DQLVersion:                "",    // zero string
		Query:                     "",    // zero string
		CanonicalQuery:            "",    // zero string
		Timezone:                  "",    // zero string
		Locale:                    "",    // zero string
		// AnalysisTimeframe and Contributions are nil (zero pointer)
	}

	tests := []struct {
		field     string
		checkJSON string // must appear in JSON output
	}{
		{"executionTimeMilliseconds", `"executionTimeMilliseconds":0`},
		{"scannedRecords", `"scannedRecords":0`},
		{"scannedBytes", `"scannedBytes":0`},
		{"scannedDataPoints", `"scannedDataPoints":0`},
		{"sampled", `"sampled":false`},
		{"queryId", `"queryId":""`},
		{"dqlVersion", `"dqlVersion":""`},
		{"query", `"query":""`},
		{"canonicalQuery", `"canonicalQuery":""`},
		{"timezone", `"timezone":""`},
		{"locale", `"locale":""`},
		{"analysisTimeframe", `"analysisTimeframe":null`},
		{"contributions", `"contributions":null`},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result := MetadataToMap(meta, []string{tt.field})
			m, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map[string]interface{}, got %T", result)
			}
			if len(m) != 1 {
				t.Fatalf("expected exactly 1 key, got %d: %v", len(m), m)
			}

			out, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			jsonStr := string(out)
			if !strings.Contains(jsonStr, tt.checkJSON) {
				t.Errorf("JSON should contain %s, got: %s", tt.checkJSON, jsonStr)
			}
		})
	}
}

// TestMetadataToMap_PreservesZeroValues_JSON_EndToEnd exercises the full
// JSONPrinter path with the {records, metadata} map structure that printResults()
// builds, verifying that zero values appear in the output for selected fields.
func TestMetadataToMap_PreservesZeroValues_JSON_EndToEnd(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 0,
		ScannedDataPoints:         0,
		Sampled:                   false,
		QueryID:                   "zero-value-test-id",
	}

	fields := []string{"sampled", "scannedDataPoints", "executionTimeMilliseconds", "queryId"}

	records := []map[string]interface{}{
		{"timestamp": "2026-03-09T12:00:00Z", "content": "test"},
	}
	payload := map[string]interface{}{
		"records":  records,
		"metadata": MetadataToMap(meta, fields),
	}

	var buf strings.Builder
	printer := &JSONPrinter{writer: &buf}
	if err := printer.Print(payload); err != nil {
		t.Fatalf("JSONPrinter.Print failed: %v", err)
	}
	result := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("could not parse JSON: %v", err)
	}

	metaMap, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata is not a map, got %T", parsed["metadata"])
	}

	// Zero values must be present (not suppressed by omitempty)
	if v, exists := metaMap["sampled"]; !exists {
		t.Error("'sampled' should be present in output")
	} else if v != false {
		t.Errorf("'sampled' should be false, got %v", v)
	}

	if v, exists := metaMap["scannedDataPoints"]; !exists {
		t.Error("'scannedDataPoints' should be present in output")
	} else if v != float64(0) {
		t.Errorf("'scannedDataPoints' should be 0, got %v", v)
	}

	if v, exists := metaMap["executionTimeMilliseconds"]; !exists {
		t.Error("'executionTimeMilliseconds' should be present in output")
	} else if v != float64(0) {
		t.Errorf("'executionTimeMilliseconds' should be 0, got %v", v)
	}

	// Non-zero string should also be present
	if metaMap["queryId"] != "zero-value-test-id" {
		t.Errorf("expected queryId='zero-value-test-id', got %v", metaMap["queryId"])
	}

	// Non-selected fields should NOT be present
	for _, absent := range []string{"scannedRecords", "scannedBytes", "dqlVersion", "timezone", "locale"} {
		if _, exists := metaMap[absent]; exists {
			t.Errorf("metadata should NOT contain %q", absent)
		}
	}
}

// TestMetadataToMap_PreservesZeroValues_YAML_EndToEnd exercises the full
// YAMLPrinter path with zero values in selected fields.
func TestMetadataToMap_PreservesZeroValues_YAML_EndToEnd(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 0,
		ScannedDataPoints:         0,
		Sampled:                   false,
		QueryID:                   "zero-yaml-test-id",
	}

	fields := []string{"sampled", "scannedDataPoints", "queryId"}

	records := []map[string]interface{}{
		{"timestamp": "2026-03-09T12:00:00Z", "content": "test"},
	}
	payload := map[string]interface{}{
		"records":  records,
		"metadata": MetadataToMap(meta, fields),
	}

	var buf strings.Builder
	printer := &YAMLPrinter{writer: &buf}
	if err := printer.Print(payload); err != nil {
		t.Fatalf("YAMLPrinter.Print failed: %v", err)
	}
	result := buf.String()

	// Zero values must appear in YAML
	if !strings.Contains(result, "sampled: false") {
		t.Errorf("YAML output should contain 'sampled: false', got:\n%s", result)
	}
	if !strings.Contains(result, "scannedDataPoints: 0") {
		t.Errorf("YAML output should contain 'scannedDataPoints: 0', got:\n%s", result)
	}
	if !strings.Contains(result, "queryId: zero-yaml-test-id") {
		t.Errorf("YAML output should contain 'queryId: zero-yaml-test-id', got:\n%s", result)
	}

	// Non-selected fields must NOT appear
	for _, absent := range []string{"executionTimeMilliseconds", "scannedRecords", "scannedBytes", "dqlVersion"} {
		if strings.Contains(result, absent) {
			t.Errorf("YAML output should NOT contain %q, got:\n%s", absent, result)
		}
	}
}

// TestMetadataToMap_SelectFields verifies the map contains only selected fields
// and preserves non-zero values correctly.
func TestMetadataToMap_SelectFields(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		ScannedDataPoints:         100,
		Sampled:                   true,
		QueryID:                   "test-id",
		DQLVersion:                "V1_0",
		Query:                     "fetch logs",
		CanonicalQuery:            "fetch logs | limit 3",
		Timezone:                  "Z",
		Locale:                    "und",
		AnalysisTimeframe: &MetadataTimeframe{
			Start: "2026-03-09T10:00:00Z",
			End:   "2026-03-09T12:00:00Z",
		},
		Contributions: &MetadataContribs{
			Buckets: []MetadataBucket{
				{Name: "bucket1", Table: "logs", ScannedBytes: 100, MatchedRecordsRatio: 0.5},
			},
		},
	}

	result := MetadataToMap(meta, []string{"queryId", "scannedRecords", "analysisTimeframe"})
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}

	if len(m) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(m), m)
	}
	if m["queryId"] != "test-id" {
		t.Errorf("expected queryId='test-id', got %v", m["queryId"])
	}
	if m["scannedRecords"] != int64(42351) {
		t.Errorf("expected scannedRecords=42351, got %v", m["scannedRecords"])
	}
	tf, ok := m["analysisTimeframe"].(*MetadataTimeframe)
	if !ok {
		t.Fatalf("expected *MetadataTimeframe, got %T", m["analysisTimeframe"])
	}
	if tf.Start != "2026-03-09T10:00:00Z" {
		t.Errorf("expected start time, got %q", tf.Start)
	}

	// Non-selected must be absent
	for _, absent := range []string{"executionTimeMilliseconds", "scannedBytes", "sampled", "dqlVersion"} {
		if _, exists := m[absent]; exists {
			t.Errorf("map should NOT contain %q", absent)
		}
	}
}

// TestJSONPrinter_MetadataToMap exercises the real JSONPrinter with
// MetadataToMap — the actual code path used in printResults().
func TestJSONPrinter_MetadataToMap(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 100,
		ScannedRecords:            5000,
		ScannedBytes:              1048576,
		QueryID:                   "json-map-test-id",
		DQLVersion:                "V1_0",
		Timezone:                  "Europe/Vienna",
		Locale:                    "en",
	}

	fields := []string{"queryId", "executionTimeMilliseconds"}

	records := []map[string]interface{}{
		{"timestamp": "2026-03-09T12:00:00Z", "content": "hello"},
	}
	payload := map[string]interface{}{
		"records":  records,
		"metadata": MetadataToMap(meta, fields),
	}

	var buf strings.Builder
	printer := &JSONPrinter{writer: &buf}
	if err := printer.Print(payload); err != nil {
		t.Fatalf("JSONPrinter.Print failed: %v", err)
	}
	result := buf.String()

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("could not parse JSONPrinter output: %v", err)
	}

	metaMap, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata is not a map, got %T", parsed["metadata"])
	}
	if metaMap["queryId"] != "json-map-test-id" {
		t.Errorf("expected queryId=json-map-test-id, got %v", metaMap["queryId"])
	}
	if metaMap["executionTimeMilliseconds"] != float64(100) {
		t.Errorf("expected executionTimeMilliseconds=100, got %v", metaMap["executionTimeMilliseconds"])
	}

	// Unselected fields must be absent
	for _, absent := range []string{"scannedRecords", "scannedBytes", "dqlVersion", "timezone", "locale"} {
		if _, exists := metaMap[absent]; exists {
			t.Errorf("metadata should NOT contain %q", absent)
		}
	}
}

// TestYAMLPrinter_MetadataToMap exercises the real YAMLPrinter with
// MetadataToMap — the actual code path used in printResults().
func TestYAMLPrinter_MetadataToMap(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 200,
		ScannedRecords:            8000,
		ScannedBytes:              2097152,
		QueryID:                   "yaml-map-test-id",
		DQLVersion:                "V1_0",
		Timezone:                  "UTC",
		Locale:                    "de",
		Sampled:                   true,
		AnalysisTimeframe: &MetadataTimeframe{
			Start: "2026-03-09T10:00:00Z",
			End:   "2026-03-09T12:00:00Z",
		},
	}

	fields := []string{"queryId", "sampled", "analysisTimeframe"}

	records := []map[string]interface{}{
		{"timestamp": "2026-03-09T12:00:00Z", "content": "test entry"},
	}
	payload := map[string]interface{}{
		"records":  records,
		"metadata": MetadataToMap(meta, fields),
	}

	var buf strings.Builder
	printer := &YAMLPrinter{writer: &buf}
	if err := printer.Print(payload); err != nil {
		t.Fatalf("YAMLPrinter.Print failed: %v", err)
	}
	result := buf.String()

	// Selected fields must be present
	if !strings.Contains(result, "queryId: yaml-map-test-id") {
		t.Errorf("YAML output should contain queryId, got:\n%s", result)
	}
	if !strings.Contains(result, "sampled: true") {
		t.Errorf("YAML output should contain sampled: true, got:\n%s", result)
	}
	if !strings.Contains(result, "analysisTimeframe:") {
		t.Errorf("YAML output should contain analysisTimeframe, got:\n%s", result)
	}

	// Unselected fields must be absent
	for _, absent := range []string{
		"executionTimeMilliseconds",
		"scannedRecords",
		"scannedBytes",
		"dqlVersion",
		"timezone",
		"locale",
	} {
		if strings.Contains(result, absent) {
			t.Errorf("YAML output should NOT contain %q, got:\n%s", absent, result)
		}
	}
}

// TestValidMetadataFieldNames_Sorted verifies that ValidMetadataFieldNames
// returns all 13 fields in sorted order.
func TestValidMetadataFieldNames_Sorted(t *testing.T) {
	names := ValidMetadataFieldNames()
	if len(names) != 13 {
		t.Fatalf("expected 13 valid field names, got %d: %v", len(names), names)
	}
	// Verify sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("fields not sorted: %q comes after %q", names[i], names[i-1])
		}
	}
	// Verify every expected field is present
	expected := map[string]bool{
		"executionTimeMilliseconds": true,
		"scannedRecords":            true,
		"scannedBytes":              true,
		"scannedDataPoints":         true,
		"sampled":                   true,
		"queryId":                   true,
		"dqlVersion":                true,
		"query":                     true,
		"canonicalQuery":            true,
		"timezone":                  true,
		"locale":                    true,
		"analysisTimeframe":         true,
		"contributions":             true,
	}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected field name: %q", n)
		}
		delete(expected, n)
	}
	for missing := range expected {
		t.Errorf("missing field name: %q", missing)
	}
}
