package apply

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// TestApply_DocumentUpdate_PreservesLabels verifies that labels embedded in an
// exported document payload are sent through on a round-trip apply update.
func TestApply_DocumentUpdate_PreservesLabels(t *testing.T) {
	var patchLabels []string
	patched := false
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/dash-1/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-1", "name": "Dash", "type": "dashboard", "version": 3,
			})
		},
		"/platform/document/v1/documents/dash-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			patchLabels = r.MultipartForm.Value["labels"]
			patched = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-1", "name": "Dash", "type": "dashboard", "version": 4,
				"labels": []string{"team-a", "env:prod"},
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	payload := `{"id":"dash-1","type":"dashboard","labels":["team-a","env:prod"],"content":{"tiles":{},"version":"1"}}`
	results, err := a.Apply([]byte(payload), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !patched {
		t.Fatal("expected PATCH to be issued")
	}
	if want := []string{"team-a", "env:prod"}; !reflect.DeepEqual(patchLabels, want) {
		t.Errorf("labels sent on update = %v, want %v", patchLabels, want)
	}
	res, ok := results[0].(*DashboardApplyResult)
	if !ok {
		t.Fatalf("expected *DashboardApplyResult, got %T", results[0])
	}
	if res.Action != ActionUpdated {
		t.Errorf("expected updated action, got %q", res.Action)
	}
}

// TestExtractDocumentLabels covers label extraction from a parsed payload,
// including non-string skipping and the absent-key case.
func TestExtractDocumentLabels(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]interface{}
		want []string
	}{
		{"absent", map[string]interface{}{}, nil},
		{"empty", map[string]interface{}{"labels": []interface{}{}}, nil},
		{"strings", map[string]interface{}{"labels": []interface{}{"a", "b"}}, []string{"a", "b"}},
		{"skips non-strings and blanks", map[string]interface{}{"labels": []interface{}{"a", 1, "", "b"}}, []string{"a", "b"}},
		{"wrong type", map[string]interface{}{"labels": "a,b"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDocumentLabels(tt.doc); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractDocumentLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}
