package apply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// TestDetectResourceType_CustomDocument covers the generic-document detection
// added for issue #390: any object carrying a non-empty custom "type" field is
// treated as a document so it can be round-tripped through apply/update.
func TestDetectResourceType_CustomDocument(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ResourceType
		wantErr  bool
	}{
		{
			name:     "custom type only",
			input:    `{"type":"acme:config","settings":{"enabled":true}}`,
			expected: ResourceDocument,
		},
		{
			name:     "custom type with content wrapper (get -o json round-trip)",
			input:    `{"id":"acme-1","name":"cfg","type":"acme:config","content":{"foo":"bar"},"version":3}`,
			expected: ResourceDocument,
		},
		{
			name:     "custom type with metadata field",
			input:    `{"metadata":{"name":"x"},"type":"acme:config"}`,
			expected: ResourceDocument,
		},
		{
			name:     "launchpad document",
			input:    `{"type":"launchpad","content":{"widgets":[]}}`,
			expected: ResourceDocument,
		},
		{
			name:     "built-in dashboard type still wins",
			input:    `{"type":"dashboard","content":{"tiles":{}}}`,
			expected: ResourceDashboard,
		},
		{
			name:     "built-in notebook type still wins",
			input:    `{"type":"notebook","content":{"sections":[]}}`,
			expected: ResourceNotebook,
		},
		{
			name:     "empty type is not a document",
			input:    `{"type":"","foo":"bar"}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
		{
			name:     "no type and no known markers still unknown",
			input:    `{"random":"field"}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isArray, err := detectResourceType([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got type %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if isArray {
				t.Errorf("expected isArray=false")
			}
			if got != tt.expected {
				t.Errorf("detectResourceType = %q, want %q", got, tt.expected)
			}
		})
	}
}

// docContentServer builds a test server exposing the documents create endpoint
// and, optionally, a metadata + update endpoint for an existing document.
func customDocMultipartCreateResponse(w http.ResponseWriter, id, name, docType string) {
	boundary := "resp-boundary"
	w.Header().Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	fmt.Fprintf(w,
		"--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n{\"id\":%q,\"name\":%q,\"type\":%q,\"version\":1}\r\n--%s--\r\n",
		boundary, id, name, docType, boundary)
}

// TestApply_CustomDocumentCreate applies a custom-typed document with no ID and
// asserts it is created as a generic DocumentApplyResult carrying the real type.
func TestApply_CustomDocumentCreate(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			customDocMultipartCreateResponse(w, "acme-new-1", "My Config", "acme:config")
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	docJSON := `{"type":"acme:config","name":"My Config","settings":{"enabled":true}}`
	results, err := a.Apply([]byte(docJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() custom document create error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res, ok := results[0].(*DocumentApplyResult)
	if !ok {
		t.Fatalf("expected *DocumentApplyResult, got %T", results[0])
	}
	if res.Action != ActionCreated {
		t.Errorf("expected action %q, got %q", ActionCreated, res.Action)
	}
	if res.ResourceType != "acme:config" {
		t.Errorf("expected resourceType %q, got %q", "acme:config", res.ResourceType)
	}
	if res.ID != "acme-new-1" {
		t.Errorf("expected id %q, got %q", "acme-new-1", res.ID)
	}
}

// TestApply_CustomDocumentUpdate updates an existing custom document by supplying
// --type and --id explicitly (payload carries only the raw content).
func TestApply_CustomDocumentUpdate(t *testing.T) {
	const id = "acme-cfg-1"
	patched := false
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/" + id + "/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      id,
				"name":    "My Config",
				"type":    "acme:config",
				"version": 4,
				"owner":   "someone-else",
			})
		},
		"/platform/document/v1/documents/" + id: func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			if v := r.URL.Query().Get("optimistic-locking-version"); v != "4" {
				t.Errorf("expected optimistic-locking-version=4, got %q", v)
			}
			patched = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      id,
				"name":    "My Config",
				"type":    "acme:config",
				"version": 5,
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	// Raw content payload with no embedded type/id — supplied via options.
	docJSON := `{"settings":{"enabled":false}}`
	results, err := a.Apply([]byte(docJSON), ApplyOptions{Type: "acme:config", OverrideID: id})
	if err != nil {
		t.Fatalf("Apply() custom document update error = %v", err)
	}
	if !patched {
		t.Error("expected PATCH to be issued")
	}
	res, ok := results[0].(*DocumentApplyResult)
	if !ok {
		t.Fatalf("expected *DocumentApplyResult, got %T", results[0])
	}
	if res.Action != ActionUpdated {
		t.Errorf("expected action %q, got %q", ActionUpdated, res.Action)
	}
	if res.ResourceType != "acme:config" {
		t.Errorf("expected resourceType %q, got %q", "acme:config", res.ResourceType)
	}
}

// TestApply_RequireExisting_NotFound asserts that update-only semantics fail
// (instead of creating) when the target document does not exist.
func TestApply_RequireExisting_NotFound(t *testing.T) {
	const id = "missing-doc"
	postCalled := false
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/" + id + "/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
		"/platform/document/v1/documents": func(w http.ResponseWriter, r *http.Request) {
			postCalled = true
			w.WriteHeader(http.StatusOK)
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	docJSON := `{"settings":{"enabled":false}}`
	_, err := a.Apply([]byte(docJSON), ApplyOptions{Type: "acme:config", OverrideID: id, RequireExisting: true})
	if err == nil {
		t.Fatal("expected error for RequireExisting on missing document, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
	if postCalled {
		t.Error("update-only apply must not fall back to creating the document")
	}
}

// TestApply_RequireExisting_NoID fails fast when no target ID can be resolved.
func TestApply_RequireExisting_NoID(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	_, err := a.Apply([]byte(`{"settings":{}}`), ApplyOptions{Type: "acme:config", RequireExisting: true})
	if err == nil {
		t.Fatal("expected error when updating without an id, got nil")
	}
	if !strings.Contains(err.Error(), "without an id") {
		t.Errorf("expected 'without an id' error, got %v", err)
	}
}

// TestApply_OptsLabels_OverridePayload verifies that labels supplied via
// ApplyOptions (the --label flag) replace labels embedded in the payload on
// update, and are surfaced on the returned DocumentApplyResult.
func TestApply_OptsLabels_OverridePayload(t *testing.T) {
	const id = "acme-cfg-2"
	var patchLabels []string
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/" + id + "/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": id, "name": "My Config", "type": "acme:config", "version": 2,
			})
		},
		"/platform/document/v1/documents/" + id: func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			patchLabels = r.MultipartForm.Value["labels"]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": id, "name": "My Config", "type": "acme:config", "version": 3,
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	// Payload carries labels [old]; --label (opts.Labels) should win.
	payload := `{"id":"` + id + `","type":"acme:config","labels":["old"],"settings":{"enabled":true}}`
	results, err := a.Apply([]byte(payload), ApplyOptions{Labels: []string{"team-a", "env:prod"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if want := []string{"team-a", "env:prod"}; !reflect.DeepEqual(patchLabels, want) {
		t.Errorf("labels sent on update = %v, want %v (opts.Labels must override payload)", patchLabels, want)
	}
	res, ok := results[0].(*DocumentApplyResult)
	if !ok {
		t.Fatalf("expected *DocumentApplyResult, got %T", results[0])
	}
	if want := []string{"team-a", "env:prod"}; !reflect.DeepEqual(res.Labels, want) {
		t.Errorf("result labels = %v, want %v", res.Labels, want)
	}
}

// TestApply_TypeFlag_RejectsArray ensures --type cannot be combined with array input.
func TestApply_TypeFlag_RejectsArray(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)
	_, err := a.Apply([]byte(`[{"type":"acme:config"}]`), ApplyOptions{Type: "acme:config"})
	if err == nil {
		t.Fatal("expected error for --type with array input, got nil")
	}
	if !strings.Contains(err.Error(), "array") {
		t.Errorf("expected array-related error, got %v", err)
	}
}
