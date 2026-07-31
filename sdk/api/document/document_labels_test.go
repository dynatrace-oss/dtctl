package document

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// multipartLabels parses a multipart/form-data request and returns the values
// of the repeated "labels" form field (the wire format the Documents API uses
// for labels on write).
func multipartLabels(t *testing.T, r *http.Request) []string {
	t.Helper()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	return r.MultipartForm.Value["labels"]
}

func writeMetadata(w http.ResponseWriter, id string, version int, labels []string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DocumentMetadata{
		ID:      id,
		Name:    "doc",
		Type:    "acme:config",
		Version: version,
		Labels:  labels,
	})
}

// TestUpdateDocument_SetsLabels asserts labels are sent as repeated verbatim
// form fields and the returned document reflects them.
func TestUpdateDocument_SetsLabels(t *testing.T) {
	var gotLabels []string
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if v := r.URL.Query().Get("optimistic-locking-version"); v != "4" {
			t.Errorf("optimistic-locking-version = %q, want 4", v)
		}
		gotLabels = multipartLabels(t, r)
		writeMetadata(w, "doc-1", 5, []string{"team-a", "env:prod"})
	})

	h := NewHandler(newTestClient(t, mux))
	doc, err := h.UpdateDocument(context.Background(), "doc-1", 4, UpdateRequest{
		Content: []byte(`{"k":"v"}`),
		Labels:  []string{"team-a", "env:prod"},
	})
	if err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	if want := []string{"team-a", "env:prod"}; !reflect.DeepEqual(gotLabels, want) {
		t.Errorf("wire labels = %v, want %v", gotLabels, want)
	}
	if !reflect.DeepEqual(doc.Labels, []string{"team-a", "env:prod"}) {
		t.Errorf("returned labels = %v, want [team-a env:prod]", doc.Labels)
	}
}

// TestUpdateDocument_LabelsOnly ensures a labels-only update still uses
// multipart (no content/name) and carries the labels.
func TestUpdateDocument_LabelsOnly(t *testing.T) {
	var gotLabels []string
	var contentType string
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-2", func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		gotLabels = multipartLabels(t, r)
		writeMetadata(w, "doc-2", 2, []string{"only"})
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.UpdateDocument(context.Background(), "doc-2", 1, UpdateRequest{Labels: []string{"only"}}); err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/") {
		t.Errorf("Content-Type = %q, want multipart/form-data", contentType)
	}
	if want := []string{"only"}; !reflect.DeepEqual(gotLabels, want) {
		t.Errorf("wire labels = %v, want %v", gotLabels, want)
	}
}

// TestUpdateWithMetadata_NoLabels confirms the compatibility wrapper sends name
// and description but no labels field.
func TestUpdateWithMetadata_NoLabels(t *testing.T) {
	var hasLabels bool
	var gotName string
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-3", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		_, hasLabels = r.MultipartForm.Value["labels"]
		gotName = r.MultipartForm.Value["name"][0]
		writeMetadata(w, "doc-3", 2, nil)
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.UpdateWithMetadata(context.Background(), "doc-3", 1, []byte(`{}`), "application/json", "New Name", "desc"); err != nil {
		t.Fatalf("UpdateWithMetadata: %v", err)
	}
	if hasLabels {
		t.Error("UpdateWithMetadata must not send a labels field")
	}
	if gotName != "New Name" {
		t.Errorf("name = %q, want New Name", gotName)
	}
}

// TestCreate_WithLabels asserts create honors labels via a follow-up update:
// the POST carries no labels, and a subsequent PATCH sets them.
func TestCreate_WithLabels(t *testing.T) {
	var postHadLabels bool
	var patchLabels []string
	patched := false
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		_, postHadLabels = r.MultipartForm.Value["labels"]
		writeMetadata(w, "new-1", 1, nil)
	})
	mux.HandleFunc("/platform/document/v1/documents/new-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if v := r.URL.Query().Get("optimistic-locking-version"); v != "1" {
			t.Errorf("follow-up locking version = %q, want 1", v)
		}
		patched = true
		patchLabels = multipartLabels(t, r)
		writeMetadata(w, "new-1", 2, []string{"a", "b"})
	})

	h := NewHandler(newTestClient(t, mux))
	doc, err := h.Create(context.Background(), CreateRequest{
		Name:    "doc",
		Type:    "acme:config",
		Content: []byte(`{"k":"v"}`),
		Labels:  []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if postHadLabels {
		t.Error("create POST must not carry labels (API rejects them there)")
	}
	if !patched {
		t.Fatal("expected a follow-up PATCH to set labels")
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(patchLabels, want) {
		t.Errorf("follow-up labels = %v, want %v", patchLabels, want)
	}
	if !reflect.DeepEqual(doc.Labels, []string{"a", "b"}) {
		t.Errorf("returned labels = %v, want [a b]", doc.Labels)
	}
	if doc.Version != 2 {
		t.Errorf("returned version = %d, want 2 (create + label update)", doc.Version)
	}
}

// TestCreate_NoLabels_NoFollowup ensures no PATCH happens when no labels are set.
func TestCreate_NoLabels_NoFollowup(t *testing.T) {
	patched := false
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		writeMetadata(w, "new-2", 1, nil)
	})
	mux.HandleFunc("/platform/document/v1/documents/new-2", func(w http.ResponseWriter, r *http.Request) {
		patched = true
		w.WriteHeader(http.StatusOK)
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.Create(context.Background(), CreateRequest{Name: "d", Type: "t", Content: []byte(`{}`)}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if patched {
		t.Error("create without labels must not issue a follow-up PATCH")
	}
}

// TestCreate_LabelFollowupFails surfaces a clear error naming the created
// document when the follow-up label update fails.
func TestCreate_LabelFollowupFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		writeMetadata(w, "new-3", 1, nil)
	})
	mux.HandleFunc("/platform/document/v1/documents/new-3", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad label"}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	doc, err := h.Create(context.Background(), CreateRequest{
		Name: "d", Type: "t", Content: []byte(`{}`), Labels: []string{"x"},
	})
	if err == nil {
		t.Fatal("expected error when follow-up label update fails")
	}
	if doc == nil || doc.ID != "new-3" {
		t.Errorf("expected the created document to be returned with its id, got %+v", doc)
	}
}
