package document

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// rawByteSeqRE matches the regression signature: a YAML/JSON sequence that is
// really a []byte rendered element-by-element as integers (e.g. "- 123").
var rawByteSeqRE = regexp.MustCompile(`(?m)(?:^[ \t]*-[ \t]+\d+[ \t]*\n){8,}`)

// dashboardContent is a small but realistic dashboard document body.
const dashboardContent = `{"tiles":{"0":{"type":"data","title":"Hosts"}},"layouts":{"0":{"x":0,"y":0}}}`

// --- Unit tests: CLI Document marshaling ---

// TestDocument_MarshalYAML_StructuredContent guards the regression where the CLI
// Document lacked a MarshalYAML and Content ([]byte) was emitted as a list of
// raw byte values (e.g. "- 123") instead of the structured dashboard body.
func TestDocument_MarshalYAML_StructuredContent(t *testing.T) {
	d := Document{
		ID:      "c8e42bc8-a9bd-433f-85c7-343017c0836a",
		Name:    "Smartscape Overview",
		Type:    "dashboard",
		Version: 784,
		Content: []byte(dashboardContent),
	}

	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	got := string(out)

	if rawByteSeqRE.MatchString(got) {
		t.Fatalf("content rendered as raw byte sequence (regression):\n%s", got)
	}

	// Round-trip the YAML back and assert the content is structured data.
	var parsed map[string]any
	if err := yaml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	content, ok := parsed["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected content to be a structured mapping, got %T:\n%s", parsed["content"], got)
	}
	if _, ok := content["tiles"]; !ok {
		t.Errorf("expected content.tiles in YAML output, got: %v", content)
	}
}

// TestDocument_MarshalJSON_StructuredContent guards that JSON output includes
// the content as a structured object rather than dropping it (the field is
// tagged json:"-", so without a custom marshaler content disappears entirely).
func TestDocument_MarshalJSON_StructuredContent(t *testing.T) {
	d := Document{
		ID:      "c8e42bc8-a9bd-433f-85c7-343017c0836a",
		Name:    "Smartscape Overview",
		Type:    "dashboard",
		Version: 784,
		Content: []byte(dashboardContent),
	}

	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var parsed struct {
		Content struct {
			Tiles map[string]any `json:"tiles"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output: %s", err, out)
	}
	if len(parsed.Content.Tiles) == 0 {
		t.Errorf("expected content.tiles in JSON output, got: %s", out)
	}
}

// TestDocument_MarshalYAML_NonJSONContent ensures non-JSON content is rendered
// as a plain string rather than a byte sequence.
func TestDocument_MarshalYAML_NonJSONContent(t *testing.T) {
	d := Document{ID: "x", Name: "raw", Type: "dashboard", Content: []byte("not json content")}

	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	if rawByteSeqRE.MatchString(string(out)) {
		t.Fatalf("non-JSON content rendered as raw byte sequence:\n%s", out)
	}
	if !strings.Contains(string(out), "not json content") {
		t.Errorf("expected raw string content in YAML output, got:\n%s", out)
	}
}

// TestDocument_MarshalYAML_EmptyContent ensures documents without content omit
// the content key entirely instead of emitting an empty/invalid value.
func TestDocument_MarshalYAML_EmptyContent(t *testing.T) {
	d := Document{ID: "x", Name: "meta-only", Type: "dashboard", Version: 1}

	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	if strings.Contains(string(out), "content:") {
		t.Errorf("expected no content key for empty content, got:\n%s", out)
	}
}

// TestSnapshot_MarshalYAML_NoLeakedDisplayFields ensures the snapshot's
// display-only CreatedBy/CreatedTime (json:"-", duplicates of ModificationInfo)
// do not leak into YAML output and that keys keep their camelCase — matching
// JSON output (used by `dtctl history ... -o yaml`).
func TestSnapshot_MarshalYAML_NoLeakedDisplayFields(t *testing.T) {
	s := Snapshot{
		SnapshotVersion: 3,
		DocumentVersion: 12,
		Description:     "before edit",
		CreatedBy:       "user-a@example.invalid",
	}

	out, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}
	for _, leaked := range []string{"createdby", "createdtime", "createdBy", "createdTime"} {
		if _, ok := m[leaked]; ok {
			t.Errorf("display-only field %q leaked into YAML: %v", leaked, m)
		}
	}
	if _, ok := m["snapshotVersion"]; !ok {
		t.Errorf("expected camelCase snapshotVersion key in YAML, got: %v", m)
	}
}

// --- Integration test: full fetch -> render path used by `get dashboard -o yaml` ---

// TestGet_RendersStructuredContent exercises the path `get dashboard <id> -o yaml`
// actually uses: handler.Get() fetches a document (multipart response with a
// content part) and the resulting CLI Document is marshaled for output. This is
// the gap the original bug fell through — the marshaling tests only covered the
// SDK type, never the CLI type returned by the handler.
func TestGet_RendersStructuredContent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		boundary := "test-boundary"
		w.Header().Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w,
			"--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n"+
				"{\"id\":\"c8e42bc8-a9bd-433f-85c7-343017c0836a\",\"name\":\"Smartscape Overview\",\"type\":\"dashboard\",\"version\":784}\r\n"+
				"--%s\r\nContent-Disposition: form-data; name=\"content\"; filename=\"content.json\"\r\nContent-Type: application/json\r\n\r\n"+
				"%s\r\n--%s--\r\n",
			boundary, boundary, dashboardContent, boundary)
	})

	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	doc, err := h.Get("c8e42bc8-a9bd-433f-85c7-343017c0836a")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// YAML output (the reported failure mode).
	yamlOut, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml.Marshal(doc) error = %v", err)
	}
	if rawByteSeqRE.MatchString(string(yamlOut)) {
		t.Fatalf("get dashboard -o yaml rendered content as raw bytes (regression):\n%s", yamlOut)
	}
	var parsedYAML map[string]any
	if err := yaml.Unmarshal(yamlOut, &parsedYAML); err != nil {
		t.Fatalf("yaml round-trip error = %v", err)
	}
	if _, ok := parsedYAML["content"].(map[string]any); !ok {
		t.Errorf("expected structured content in YAML, got %T:\n%s", parsedYAML["content"], yamlOut)
	}

	// JSON output must include content (not drop it).
	jsonOut, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal(doc) error = %v", err)
	}
	if !strings.Contains(string(jsonOut), `"content"`) {
		t.Errorf("expected content in JSON output, got: %s", jsonOut)
	}
}
