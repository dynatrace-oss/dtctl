package apply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestApply_DocumentUpdate_CreateSnapshot verifies --create-snapshot reaches the
// updateDocument call as the create-snapshot query parameter, with its description.
func TestApply_DocumentUpdate_CreateSnapshot(t *testing.T) {
	var createSnapshot string
	var description []string
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/dash-1/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-1", "name": "Dash", "type": "dashboard", "version": 3,
			})
		},
		"/platform/document/v1/documents/dash-1": func(w http.ResponseWriter, r *http.Request) {
			createSnapshot = r.URL.Query().Get("create-snapshot")
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			description = r.MultipartForm.Value["snapshotDescription"]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-1", "name": "Dash", "type": "dashboard", "version": 4,
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	payload := `{"id":"dash-1","type":"dashboard","content":{"tiles":{},"version":"1"}}`
	if _, err := a.Apply([]byte(payload), ApplyOptions{
		CreateSnapshot:      true,
		SnapshotDescription: "before Q3 rework",
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if createSnapshot != "true" {
		t.Errorf("create-snapshot = %q, want true", createSnapshot)
	}
	if len(description) != 1 || description[0] != "before Q3 rework" {
		t.Errorf("snapshotDescription = %v, want [before Q3 rework]", description)
	}
}

// TestApply_DocumentUpdate_NoSnapshotByDefault pins the opt-in contract at the
// apply layer: an ordinary apply must not create snapshots.
func TestApply_DocumentUpdate_NoSnapshotByDefault(t *testing.T) {
	var createSnapshot string
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents/dash-2/metadata": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-2", "name": "Dash", "type": "dashboard", "version": 1,
			})
		},
		"/platform/document/v1/documents/dash-2": func(w http.ResponseWriter, r *http.Request) {
			createSnapshot = r.URL.Query().Get("create-snapshot")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "dash-2", "name": "Dash", "type": "dashboard", "version": 2,
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	payload := `{"id":"dash-2","type":"dashboard","content":{"tiles":{},"version":"1"}}`
	if _, err := a.Apply([]byte(payload), ApplyOptions{}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if createSnapshot != "" {
		t.Errorf("create-snapshot = %q, want the parameter to be absent", createSnapshot)
	}
}

// TestApply_DocumentCreate_IgnoresCreateSnapshot asserts the flag is inert on the
// create path: there is no prior state to capture, and the create endpoint takes
// no snapshot parameter.
func TestApply_DocumentCreate_IgnoresCreateSnapshot(t *testing.T) {
	var createQuery string
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/document/v1/documents": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			createQuery = r.URL.RawQuery
			boundary := "resp-boundary"
			w.Header().Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
			fmt.Fprintf(w,
				"--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n{\"id\":\"acme-new-1\",\"name\":\"My Config\",\"type\":\"acme:config\",\"version\":1}\r\n--%s--\r\n",
				boundary, boundary)
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	payload := `{"type":"acme:config","name":"My Config","settings":{"enabled":true}}`
	if _, err := a.Apply([]byte(payload), ApplyOptions{CreateSnapshot: true}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if createQuery != "" {
		t.Errorf("create request query = %q, want no snapshot parameters", createQuery)
	}
}
