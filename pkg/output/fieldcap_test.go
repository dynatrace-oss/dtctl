package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestClipRecordValues_ClipsLongStringsWithMarker(t *testing.T) {
	long := strings.Repeat("x", 30)
	records := []map[string]interface{}{
		{"content": long, "status": float64(500), "host": "web-01"},
		{"content": "short", "status": float64(200), "host": "web-02"},
	}

	got, fields := ClipRecordValues(records, 10)

	if want := strings.Repeat("x", 10) + "…(+20 chars)"; got[0]["content"] != want {
		t.Errorf("content = %q, want %q", got[0]["content"], want)
	}
	if got[1]["content"] != "short" || got[0]["status"] != float64(500) || got[0]["host"] != "web-01" {
		t.Errorf("values at or under the cap must be untouched: %v", got)
	}
	if !reflect.DeepEqual(fields, []string{"content"}) {
		t.Errorf("fields = %v, want [content]", fields)
	}
	// The caller's rows back the spill file, which must keep the full value.
	if records[0]["content"] != long {
		t.Errorf("input was mutated: %q", records[0]["content"])
	}
}

func TestClipRecordValues_NestedValuesNameTheNearestKey(t *testing.T) {
	records := []map[string]interface{}{
		{
			"k8s.object": map[string]interface{}{"spec": strings.Repeat("s", 20)},
			"frames":     []interface{}{strings.Repeat("f", 20), "ok"},
		},
	}

	got, fields := ClipRecordValues(records, 5)

	spec := got[0]["k8s.object"].(map[string]interface{})["spec"]
	if spec != "sssss…(+15 chars)" {
		t.Errorf("nested map value not clipped: %q", spec)
	}
	frames := got[0]["frames"].([]interface{})
	if frames[0] != "fffff…(+15 chars)" || frames[1] != "ok" {
		t.Errorf("array element not clipped: %v", frames)
	}
	// Sorted so the hint is deterministic regardless of map iteration order.
	if !reflect.DeepEqual(fields, []string{"frames", "spec"}) {
		t.Errorf("fields = %v, want [frames spec]", fields)
	}
}

func TestClipRecordValues_RuneAware(t *testing.T) {
	records := []map[string]interface{}{{"msg": "ääääää"}}
	got, _ := ClipRecordValues(records, 2)
	if got[0]["msg"] != "ää…(+4 chars)" {
		t.Errorf("msg = %q", got[0]["msg"])
	}
}

func TestClipRecordValues_ZeroDisables(t *testing.T) {
	records := []map[string]interface{}{{"content": strings.Repeat("x", 10000)}}
	got, fields := ClipRecordValues(records, 0)
	if got[0]["content"] != records[0]["content"] || fields != nil {
		t.Errorf("max 0 must be a no-op, got fields %v", fields)
	}
}

func TestClipValue_ArbitraryShape(t *testing.T) {
	in := []interface{}{map[string]interface{}{"trace": strings.Repeat("t", 8)}, strings.Repeat("u", 8)}
	got, fields := ClipValue(in, 4)
	want := []interface{}{map[string]interface{}{"trace": "tttt…(+4 chars)"}, "uuuu…(+4 chars)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !reflect.DeepEqual(fields, []string{"trace"}) {
		t.Errorf("fields = %v", fields)
	}
}

func TestAgentPrinter_MaxFieldCharsClipsAfterJQ(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{Verb: "query"})
	p.SetJQFilter(`[.records[] | select(.content | endswith("NEEDLE"))]`)
	p.SetMaxFieldChars(10)

	// The match sits past the cap: clipping before the filter ran would drop it.
	payload := map[string]interface{}{"records": []map[string]interface{}{
		{"content": strings.Repeat("a", 40) + "NEEDLE"},
		{"content": "nope"},
	}}
	if err := p.Print(payload); err != nil {
		t.Fatalf("Print: %v", err)
	}

	var resp struct {
		Result  []map[string]interface{} `json:"result"`
		Context ResponseContext          `json:"context"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if len(resp.Result) != 1 || resp.Result[0]["content"] != "aaaaaaaaaa…(+36 chars)" {
		t.Fatalf("result = %v", resp.Result)
	}
	if !resp.Context.Truncated || resp.Context.MaxFieldChars != 10 ||
		!reflect.DeepEqual(resp.Context.TruncatedFields, []string{"content"}) {
		t.Errorf("context does not report the clip: %+v", resp.Context)
	}
}

func TestAgentPrinter_MaxFieldCharsNothingClippedLeavesContextClean(t *testing.T) {
	var buf bytes.Buffer
	p := NewAgentPrinter(&buf, &ResponseContext{Verb: "query"})
	p.SetMaxFieldChars(10)
	if err := p.Print(map[string]interface{}{"a": "short"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "truncated") || strings.Contains(buf.String(), "max_field_chars") {
		t.Errorf("unclipped output must not claim truncation: %s", buf.String())
	}
}

func TestEnvelopeSize_MatchesEncodedBytes(t *testing.T) {
	resp := Response{OK: true, Result: map[string]interface{}{"records": []int{1, 2, 3}},
		Context: &ResponseContext{Verb: "query", Suggestions: []string{"# ä hint"}}}

	var buf bytes.Buffer
	if err := EncodeEnvelope(&buf, resp); err != nil {
		t.Fatal(err)
	}
	n, err := EnvelopeSize(&buf, resp)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) {
		t.Errorf("EnvelopeSize = %d, encoded %d bytes", n, buf.Len())
	}
}
