package output

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMetadataFields_Minimal(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"minimal", []string{"minimal"}},
		{" minimal ", []string{"minimal"}},
		{"minimal,metrics", []string{"minimal", "metrics"}},
		{"queryId,minimal", []string{"queryId", "minimal"}},
	}
	for _, tt := range tests {
		got, err := ParseMetadataFields(tt.input)
		if err != nil {
			t.Fatalf("ParseMetadataFields(%q): %v", tt.input, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseMetadataFields(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseMetadataFields_UnknownFieldNamesMinimal(t *testing.T) {
	_, err := ParseMetadataFields("minimul")
	if err == nil {
		t.Fatal("expected an error for a misspelled selector")
	}
	if !strings.Contains(err.Error(), "minimal") {
		t.Errorf("error should name the minimal selector as a valid choice: %v", err)
	}
}

func TestIsMinimalFields(t *testing.T) {
	if IsMinimalFields(nil) || IsMinimalFields([]string{"all"}) || IsMinimalFields([]string{"queryId"}) {
		t.Error("IsMinimalFields true for a selection without minimal")
	}
	if !IsMinimalFields([]string{"metrics", "minimal"}) {
		t.Error("IsMinimalFields false for a selection containing minimal")
	}
}

func TestExpandMetadataFields(t *testing.T) {
	tf := &MetadataTimeframe{Start: "2026-01-01T00:00:00Z", End: "2026-01-01T02:00:00Z"}
	full := &QueryMetadata{
		ExecutionTimeMilliseconds: 132,
		ScannedRecords:            840499,
		ScannedBytes:              623373940,
		QueryID:                   "q-1",
		DQLVersion:                "V1_0",
		Query:                     "fetch logs",
		CanonicalQuery:            "fetch logs",
		Timezone:                  "Z",
		Locale:                    "und",
		AnalysisTimeframe:         tf,
	}

	tests := []struct {
		name          string
		meta          *QueryMetadata
		fields        []string
		defaultWindow bool
		want          []string
	}{
		{
			name:   "all passes through",
			meta:   full,
			fields: []string{"all"},
			want:   []string{"all"},
		},
		{
			name:   "explicit selection passes through",
			meta:   full,
			fields: []string{"queryId", "locale"},
			want:   []string{"queryId", "locale"},
		},
		{
			name:   "minimal keeps only cost signals, unsampled",
			meta:   full,
			fields: []string{"minimal"},
			want:   []string{"executionTimeMilliseconds", "scannedBytes"},
		},
		{
			name:   "minimal omits zero scannedBytes",
			meta:   &QueryMetadata{ExecutionTimeMilliseconds: 5},
			fields: []string{"minimal"},
			want:   []string{"executionTimeMilliseconds"},
		},
		{
			name:   "minimal includes sampled only when true",
			meta:   &QueryMetadata{ExecutionTimeMilliseconds: 5, ScannedBytes: 10, Sampled: true},
			fields: []string{"minimal"},
			want:   []string{"executionTimeMilliseconds", "scannedBytes", "sampled"},
		},
		{
			name:   "minimal includes scannedDataPoints when non-zero",
			meta:   &QueryMetadata{ExecutionTimeMilliseconds: 5, ScannedDataPoints: 1200},
			fields: []string{"minimal"},
			want:   []string{"executionTimeMilliseconds", "scannedDataPoints"},
		},
		{
			name:          "minimal includes analysisTimeframe when the default window applied",
			meta:          full,
			fields:        []string{"minimal"},
			defaultWindow: true,
			want:          []string{"executionTimeMilliseconds", "scannedBytes", "analysisTimeframe"},
		},
		{
			name:          "default window without a timeframe in the response adds nothing",
			meta:          &QueryMetadata{ExecutionTimeMilliseconds: 5},
			fields:        []string{"minimal"},
			defaultWindow: true,
			want:          []string{"executionTimeMilliseconds"},
		},
		{
			name: "minimal includes contributions when the response carries them",
			meta: &QueryMetadata{ExecutionTimeMilliseconds: 5, Contributions: &MetadataContribs{
				Buckets: []MetadataBucket{{Name: "default_logs"}},
			}},
			fields: []string{"minimal"},
			want:   []string{"executionTimeMilliseconds", "contributions"},
		},
		{
			name:   "minimal plus explicit fields, deduplicated",
			meta:   full,
			fields: []string{"minimal", "queryId", "scannedBytes"},
			want:   []string{"executionTimeMilliseconds", "scannedBytes", "queryId"},
		},
		{
			name:   "nil metadata keeps the selection",
			meta:   nil,
			fields: []string{"minimal"},
			want:   []string{"minimal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandMetadataFields(tt.meta, tt.fields, tt.defaultWindow)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandMetadataFields = %v, want %v", got, tt.want)
			}
		})
	}
}

// The expanded selection must drive every renderer, not just the envelope:
// -o csv puts metadata in a comment header, table/wide in a footer.
func TestExpandedMinimalFields_DriveCSVAndFooter(t *testing.T) {
	meta := &QueryMetadata{
		ExecutionTimeMilliseconds: 132,
		ScannedBytes:              2048,
		QueryID:                   "q-1",
		Query:                     "fetch logs",
		CanonicalQuery:            "fetch logs",
		Locale:                    "und",
		Timezone:                  "Z",
		DQLVersion:                "V1_0",
	}
	fields := ExpandMetadataFields(meta, []string{"minimal"}, false)

	csv := FormatMetadataCSVComments(meta, fields)
	for _, want := range []string{"# execution_time_ms: 132", "# scanned_bytes: 2048"} {
		if !strings.Contains(csv, want) {
			t.Errorf("csv header missing %q:\n%s", want, csv)
		}
	}
	for _, unwanted := range []string{"query_id", "query:", "canonical_query", "locale", "timezone", "dql_version", "sampled"} {
		if strings.Contains(csv, unwanted) {
			t.Errorf("csv header should not carry %q under minimal:\n%s", unwanted, csv)
		}
	}

	footer := FormatMetadataFooter(meta, fields)
	for _, unwanted := range []string{"Query ID", "Locale", "Timezone", "DQL version"} {
		if strings.Contains(footer, unwanted) {
			t.Errorf("footer should not carry %q under minimal:\n%s", unwanted, footer)
		}
	}

	m, ok := MetadataToMap(meta, fields).(map[string]interface{})
	if !ok {
		t.Fatalf("MetadataToMap returned %T, want a map", MetadataToMap(meta, fields))
	}
	if len(m) != 2 || m["executionTimeMilliseconds"] != int64(132) || m["scannedBytes"] != int64(2048) {
		t.Errorf("MetadataToMap under minimal = %v", m)
	}
}

// "all" is only valid alone, so it cannot be combined with minimal and then
// act as a full-selection sentinel inside the expanded list.
func TestParseMetadataFields_MinimalWithAllRejected(t *testing.T) {
	for _, in := range []string{"minimal,all", "all,minimal"} {
		if _, err := ParseMetadataFields(in); err == nil {
			t.Errorf("ParseMetadataFields(%q) should fail", in)
		}
	}
}

func TestMetadataOmitsFields(t *testing.T) {
	meta := &QueryMetadata{ExecutionTimeMilliseconds: 5, ScannedBytes: 10, QueryID: "q-1"}
	if !MetadataOmitsFields(meta, []string{"executionTimeMilliseconds", "scannedBytes"}) {
		t.Error("queryId is dropped, want true")
	}
	if MetadataOmitsFields(meta, []string{"executionTimeMilliseconds", "scannedBytes", "queryId"}) {
		t.Error("every non-empty field is shown, want false")
	}
	if MetadataOmitsFields(meta, []string{"all"}) || MetadataOmitsFields(nil, []string{"queryId"}) {
		t.Error("all / nil metadata omit nothing")
	}
}
