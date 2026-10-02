package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
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
		got, gotEnc := MeasureSerializedBytes(records, format, IndentedJSONLayout(0))
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
		got, _ := MeasureSerializedBytes(c.in, "json", IndentedJSONLayout(0))
		if got != int64(buf.Len()) {
			t.Errorf("%s: measured %d bytes, printer wrote %d", c.name, got, buf.Len())
		}
	}
}

// TestMeasureSerializedBytes_CompactMatchesCompactEncoding is the #570 guard:
// piped and agent output is compact JSON, so a compact measurement has to count
// exactly the bytes of the compact encoding, not of the indented one — for the
// streamed shapes and for the whole-value fallback alike.
func TestMeasureSerializedBytes_CompactMatchesCompactEncoding(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
	}{
		{"rows", []map[string]interface{}{
			{"host": "web-01", "status": float64(200), "msg": "a\"b <c>"},
			{"host": "web-02", "nested": map[string]interface{}{"k": []interface{}{1.0, "2"}}},
		}},
		{"empty", []map[string]interface{}{}},
		{"nil slice", []map[string]interface{}(nil)},
		{"single map", map[string]interface{}{"a": 1.0}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(c.in); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, enc := MeasureSerializedBytes(c.in, "json", JSONLayout{})
		if enc != "json" {
			t.Errorf("%s: encoding = %q, want json", c.name, enc)
		}
		if got != int64(buf.Len()) {
			t.Errorf("%s: measured %d bytes, compact encoding is %d", c.name, got, buf.Len())
		}
	}
}

// JSON Lines carries no array framing: n rows are n compact lines, and an empty
// result is no bytes at all, where a compact array would be "[]\n" — two bytes
// more than the lines for every non-empty result. The measurement has to be
// what JSONLPrinter writes, for every row count.
func TestMeasureSerializedBytes_JSONLinesMatchesPrinter(t *testing.T) {
	row := func(i int) map[string]interface{} {
		return map[string]interface{}{"host": fmt.Sprintf("web-%02d", i), "msg": "a\"b <c>", "n": float64(i)}
	}
	cases := map[string]interface{}{
		"0 rows":                           []map[string]interface{}{},
		"nil slice":                        []map[string]interface{}(nil),
		"1 row":                            []map[string]interface{}{row(1)},
		"3 rows":                           []map[string]interface{}{row(1), row(2), row(3)},
		"not a slice (printer refuses it)": map[string]interface{}{"a": 1.0},
	}
	for name, in := range cases {
		var buf bytes.Buffer
		p := NewPrinterWithOpts(PrinterOptions{Format: "jsonl", Writer: &buf})
		_ = p.PrintList(in)
		got, enc := MeasureSerializedBytes(in, "jsonl", JSONLinesLayout)
		if got != int64(buf.Len()) {
			t.Errorf("%s: measured %d bytes, JSONLPrinter wrote %d: %q", name, got, buf.Len(), buf.String())
		}
		if enc != "json" {
			t.Errorf("%s: encoding = %q, want json", name, enc)
		}
	}
}

// The layout is a JSON choice; the other encodings have one layout and must
// measure the same whatever it is.
func TestMeasureSerializedBytes_LayoutOnlyAffectsJSON(t *testing.T) {
	records := []map[string]interface{}{{"host": "web-01", "n": 1.0}, {"host": "web-02", "n": 2.0}}
	for _, format := range []string{"csv", "yaml", "toon"} {
		a, _ := MeasureSerializedBytes(records, format, IndentedJSONLayout(2))
		b, _ := MeasureSerializedBytes(records, format, JSONLayout{})
		if a != b {
			t.Errorf("%s: indented %d != compact %d", format, a, b)
		}
	}
	nested, _ := MeasureSerializedBytes(records, "json", IndentedJSONLayout(2))
	root, _ := MeasureSerializedBytes(records, "json", IndentedJSONLayout(0))
	compact, _ := MeasureSerializedBytes(records, "json", JSONLayout{})
	if !(compact < root && root < nested) {
		t.Errorf("json: compact %d < root-indented %d < nested-indented %d does not hold", compact, root, nested)
	}
}

// TestEnvelopeRecordsLayout_MatchesEncodedEnvelope pins the measurement to the
// bytes EncodeEnvelope actually writes for a kind:"records" result, in both of
// its layouts: compact (piped, as for an agent) and indented (a terminal), where
// result.records, result.constant and result.types sit two levels deep, so
// every line of theirs carries that depth's indentation on top of their own.
// Measured as a root-level value instead, the terminal case came out short by
// four bytes a line.
func TestEnvelopeRecordsLayout_MatchesEncodedEnvelope(t *testing.T) {
	records := []map[string]interface{}{
		{"host": "web-01", "status": float64(200), "msg": "a\"b <c>"},
		{"host": "web-02", "nested": map[string]interface{}{"k": []interface{}{1.0, "2"}}},
	}
	constant := map[string]interface{}{"dt.entity": "HOST-0", "region": "eu"}
	types := []interface{}{map[string]interface{}{"mappings": map[string]interface{}{"host": map[string]interface{}{"type": "string"}}}}

	for _, indented := range []bool{false, true} {
		layout := envelopeRecordsLayout(indented)
		var out bytes.Buffer
		resp := Response{
			OK:              true,
			EnvelopeVersion: EnvelopeVersion,
			Result:          &InlineRecords{Kind: KindRecords, Constant: constant, Records: records, Types: types},
			Context:         &ResponseContext{Verb: "query", Decided: "inline"},
		}
		if err := encodeEnvelopeTo(&out, resp, indented); err != nil {
			t.Fatal(err)
		}
		sep := ":"
		if indented {
			sep = ": "
		}
		for key, v := range map[string]interface{}{"records": records, "constant": constant, "types": types} {
			// What the value looks like laid out at its depth, without the
			// newline a top-level encoding ends in.
			var want bytes.Buffer
			enc := json.NewEncoder(&want)
			enc.SetIndent(layout.Prefix, layout.Indent)
			if err := enc.Encode(v); err != nil {
				t.Fatal(err)
			}
			text := bytes.TrimSuffix(want.Bytes(), []byte("\n"))
			if !bytes.Contains(out.Bytes(), append([]byte(`"`+key+`"`+sep), text...)) {
				t.Errorf("indented=%v: the envelope does not carry %s laid out as %+v:\n%s\nenvelope:\n%s",
					indented, key, layout, text, out.String())
				continue
			}
			got, _ := MeasureSerializedBytes(v, "json", layout)
			if got != int64(len(text))+1 {
				t.Errorf("indented=%v: %s measured %d bytes, the envelope carries %d (+1 newline)",
					indented, key, got, len(text))
			}
		}
	}

	// EnvelopeRecordsLayout makes the same call EncodeEnvelope does: a buffer
	// or a regular file is not a terminal, so both get the compact layout.
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, w := range []io.Writer{&bytes.Buffer{}, f} {
		if got := EnvelopeRecordsLayout(w); got != (JSONLayout{}) {
			t.Errorf("%T: layout = %+v, want compact", w, got)
		}
		var buf bytes.Buffer
		if err := EncodeEnvelope(&buf, Response{OK: true, Result: records}); err != nil {
			t.Fatal(err)
		}
		if strings.Count(buf.String(), "\n") != 1 {
			t.Errorf("EncodeEnvelope indented its output to a non-terminal: %q", buf.String())
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
