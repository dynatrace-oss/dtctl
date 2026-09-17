package output

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// wholeValueJSON is the encoding streamJSONArray has to match byte for byte:
// json.Encoder with the same indent, handed the whole value at once.
func wholeValueJSON(t *testing.T, v interface{}, prefix, indent string) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("whole-value encode failed: %v", err)
	}
	return buf.String()
}

type namedSlice []int

type marshalerSlice []int

func (m marshalerSlice) MarshalJSON() ([]byte, error) { return []byte(`"custom"`), nil }

type rec struct {
	Name   string                 `json:"name"`
	Count  int                    `json:"count"`
	Nested map[string]interface{} `json:"nested,omitempty"`
	Tags   []string               `json:"tags,omitempty"`
}

// TestStreamJSONArray_MatchesWholeValueEncoding is the contract that lets the
// spill measurement use the streaming path: the byte count (and the bytes) must
// be exactly what the JSON printer would have emitted.
func TestStreamJSONArray_MatchesWholeValueEncoding(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
	}{
		{"empty slice", []map[string]interface{}{}},
		{"single flat record", []map[string]interface{}{{"host": "web-01", "status": float64(200)}}},
		{"multiple records", []map[string]interface{}{
			{"host": "web-01", "status": float64(200)},
			{"host": "web-02", "status": float64(500)},
		}},
		{"nested maps and arrays", []map[string]interface{}{
			{"a": map[string]interface{}{"b": map[string]interface{}{"c": []interface{}{1.0, 2.0, "x"}}}},
			{"a": nil, "empty_map": map[string]interface{}{}, "empty_arr": []interface{}{}},
		}},
		{"scalars", []interface{}{1.0, "two", true, nil}},
		{"strings needing escapes", []string{"a\"b", "tab\there", "nl\nhere", "unicode: ü €", "html: <b>&</b>"}},
		{"structs", []rec{
			{Name: "one", Count: 1, Nested: map[string]interface{}{"k": "v"}, Tags: []string{"a", "b"}},
			{Name: "two", Count: 2},
		}},
		{"pointer to slice", &[]map[string]interface{}{{"x": 1.0}}},
		{"array (not slice)", [2]int{1, 2}},
		{"named slice type", namedSlice{1, 2, 3}},
		{"slice of slices", [][]int{{1, 2}, {}, {3}}},
		{"slice of nil maps", []map[string]interface{}{nil, {"a": 1.0}}},
		{"deeply nested", []interface{}{map[string]interface{}{
			"l1": map[string]interface{}{"l2": map[string]interface{}{"l3": map[string]interface{}{"l4": "deep"}}},
		}}},
		{"json numbers", []interface{}{json.Number("12345678901234567890"), json.Number("1.5")}},
		{"elements with their own Marshaler", []marshalerSlice{{1}, {2}}},
		{"elements marshalling via TextMarshaler", []time.Time{
			time.Date(2026, 9, 17, 10, 30, 0, 0, time.UTC),
			time.Date(2026, 9, 17, 11, 0, 0, 0, time.UTC),
		}},
	}

	indents := []struct{ prefix, indent string }{
		{"", "  "},
		{"", "\t"},
		{"  ", "  "},
		{"", ""},
	}

	for _, c := range cases {
		for _, ind := range indents {
			var got bytes.Buffer
			handled, err := streamJSONArray(&got, c.in, ind.prefix, ind.indent)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", c.name, err)
			}
			if !handled {
				t.Fatalf("%s: expected the shape to be streamable", c.name)
			}
			want := wholeValueJSON(t, c.in, ind.prefix, ind.indent)
			if got.String() != want {
				t.Errorf("%s (prefix=%q indent=%q):\n got: %q\nwant: %q",
					c.name, ind.prefix, ind.indent, got.String(), want)
			}
		}
	}
}

// TestStreamJSONArray_UnhandledShapes checks the shapes that do not encode as a
// bracketed element list are declined with the writer untouched, so the caller
// falls back to the whole-value encoder instead of emitting something wrong.
func TestStreamJSONArray_UnhandledShapes(t *testing.T) {
	var nilSlice []int
	var nilPtr *[]int
	cases := []struct {
		name string
		in   interface{}
	}{
		{"nil slice encodes as null", nilSlice},
		{"nil pointer", nilPtr},
		{"nil interface", nil},
		{"map", map[string]interface{}{"a": 1}},
		{"string", "not a slice"},
		{"struct", rec{Name: "x"}},
		{"byte slice encodes as base64", []byte("abc")},
		{"slice with its own Marshaler", marshalerSlice{1, 2}},
		{"raw message", json.RawMessage(`[1,2]`)},
	}
	for _, c := range cases {
		var got bytes.Buffer
		handled, err := streamJSONArray(&got, c.in, "", "  ")
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
		}
		if handled {
			t.Errorf("%s: expected handled=false, got streamed output %q", c.name, got.String())
		}
		if got.Len() != 0 {
			t.Errorf("%s: writer must be untouched, got %q", c.name, got.String())
		}
	}
}

// TestStreamJSONArray_ElementErrorPropagates documents the one behavioural
// difference from the whole-value encoder: an element that cannot be marshalled
// surfaces after earlier elements were already written.
func TestStreamJSONArray_ElementErrorPropagates(t *testing.T) {
	var got bytes.Buffer
	handled, err := streamJSONArray(&got, []interface{}{
		map[string]interface{}{"ok": 1.0},
		map[string]interface{}{"bad": math.NaN()},
	}, "", "  ")
	if !handled {
		t.Fatal("expected handled=true for a slice")
	}
	if err == nil {
		t.Fatal("expected an error for a NaN element")
	}
	if !strings.Contains(got.String(), `"ok"`) {
		t.Errorf("expected the elements before the failure to have been written, got %q", got.String())
	}
}

// TestMeasureSerializedBytes_MatchesPrinterOutput pins the measurement to the
// printer for every measured encoding: the streaming JSON path must not change
// the byte count the spill threshold is compared against, and the fallback must
// stay exact for the shapes streaming declines.
func TestMeasureSerializedBytes_MatchesPrinterOutput(t *testing.T) {
	records := []map[string]interface{}{
		{"host": "web-01", "status": float64(200), "msg": "a\"b <c>"},
		{"host": "web-02", "nested": map[string]interface{}{"k": []interface{}{1.0, "2"}}},
	}
	for _, format := range []string{"json", "jsonl", "table", "wide", "csv", "yaml", "toon"} {
		var buf bytes.Buffer
		enc := NormalizeMeasureEncoding(format)
		p := NewPrinterWithOpts(PrinterOptions{Format: enc, Writer: &buf})
		if err := p.PrintList(records); err != nil {
			t.Fatalf("%s: printer failed: %v", format, err)
		}
		got, gotEnc := MeasureSerializedBytes(records, format)
		if gotEnc != enc {
			t.Errorf("%s: encoding = %q, want %q", format, gotEnc, enc)
		}
		if got != int64(buf.Len()) {
			t.Errorf("%s: measured %d bytes, printer wrote %d", format, got, buf.Len())
		}
	}
}

// TestMeasureSerializedBytes_UnstreamableShapesStayExact covers the fallback:
// a shape streamJSONArray declines (or trips over) must still measure exactly
// what the printer emits, with no partial count left over from the attempt.
func TestMeasureSerializedBytes_UnstreamableShapesStayExact(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
	}{
		{"nil slice", []map[string]interface{}(nil)},
		{"single map", map[string]interface{}{"a": 1.0}},
		{"non-finite element mid-slice", []interface{}{
			map[string]interface{}{"ok": 1.0},
			map[string]interface{}{"bad": math.Inf(1)},
		}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		p := NewPrinterWithOpts(PrinterOptions{Format: "json", Writer: &buf})
		_ = p.PrintList(c.in) // may fail; the measurement must match either way
		got, _ := MeasureSerializedBytes(c.in, "json")
		if got != int64(buf.Len()) {
			t.Errorf("%s: measured %d bytes, printer wrote %d", c.name, got, buf.Len())
		}
	}
}

// chunkRecorder records how much each Write call carried, so a test can assert
// that a measurement was produced incrementally rather than from one buffer
// holding the whole serialised result.
type chunkRecorder struct {
	total    int64
	maxChunk int
}

func (c *chunkRecorder) Write(p []byte) (int, error) {
	if len(p) > c.maxChunk {
		c.maxChunk = len(p)
	}
	c.total += int64(len(p))
	return len(p), nil
}

// TestMeasureSerializedBytes_IsIncremental is the #467 regression guard: the
// JSON measurement must never materialise the whole result. json.Encoder with
// an indent set buffers the entire serialised value (twice — once compact, once
// indented) before writing a single byte, which made a spilling query peak
// higher than a non-spilling one. Writing in per-record chunks is what keeps
// the cost bounded, so the largest chunk must stay on the order of one record.
func TestMeasureSerializedBytes_IsIncremental(t *testing.T) {
	const rows = 2000
	records := make([]map[string]interface{}, 0, rows)
	for i := 0; i < rows; i++ {
		records = append(records, map[string]interface{}{
			"host":    "web-01",
			"status":  float64(200),
			"content": strings.Repeat("x", 200),
		})
	}

	rc := &chunkRecorder{}
	handled, err := streamJSONArray(rc, records, "", jsonIndent)
	if err != nil || !handled {
		t.Fatalf("streamJSONArray(handled=%v) failed: %v", handled, err)
	}

	// Generous bound: one record plus framing is a few hundred bytes, the whole
	// array is hundreds of kilobytes. Anything that buffers the result as a unit
	// blows past this by orders of magnitude.
	perRecord := int(rc.total) / rows
	if rc.maxChunk > 10*perRecord {
		t.Errorf("largest write was %d bytes for a %d-byte array (%d bytes/record): the measurement buffered the result instead of streaming it",
			rc.maxChunk, rc.total, perRecord)
	}
}
