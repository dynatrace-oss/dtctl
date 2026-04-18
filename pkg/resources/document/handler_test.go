package document

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func newDocTestHandler(t *testing.T, mux *http.ServeMux) (*Handler, func()) {
	t.Helper()
	srv := httptest.NewServer(mux)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return NewHandler(c), srv.Close
}

// --- NewHandler ---

func TestNewHandler(t *testing.T) {
	c, err := client.NewForTesting("https://test.example.invalid", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h := NewHandler(c)
	if h == nil || h.client == nil {
		t.Fatal("NewHandler returned nil")
	}
}

// --- List ---

func TestList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentList{
			Documents: []DocumentMetadata{
				{ID: "doc-1", Name: "My Dashboard", Type: "dashboard"},
				{ID: "doc-2", Name: "My Notebook", Type: "notebook"},
			},
			TotalCount: 2,
		})
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	result, err := h.List(DocumentFilters{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Documents) != 2 {
		t.Errorf("expected 2 documents, got %d", len(result.Documents))
	}
}

func TestList_WithFilters(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		// Verify filter is passed
		filter := r.URL.Query().Get("filter")
		if filter == "" {
			t.Error("expected filter query param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentList{Documents: []DocumentMetadata{{ID: "doc-1", Type: "dashboard"}}, TotalCount: 1})
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	_, err := h.List(DocumentFilters{Type: "dashboard"})
	if err != nil {
		t.Fatalf("List() with filter error = %v", err)
	}
}

func TestList_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	_, err := h.List(DocumentFilters{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestList_Pagination(t *testing.T) {
	pageIndex := 0
	pages := []DocumentList{
		{
			Documents: []DocumentMetadata{
				{ID: "doc-1", Name: "Dashboard 1", Type: "dashboard"},
				{ID: "doc-2", Name: "Dashboard 2", Type: "dashboard"},
			},
			TotalCount:  3,
			NextPageKey: "page2",
		},
		{
			Documents: []DocumentMetadata{
				{ID: "doc-3", Name: "Dashboard 3", Type: "dashboard"},
			},
			TotalCount: 3,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		// Verify filter is sent on every request (page tokens do NOT preserve it)
		expectedFilter := "type=='dashboard'"
		if r.URL.Query().Get("filter") != expectedFilter {
			t.Errorf("expected filter %q on every request, got %q", expectedFilter, r.URL.Query().Get("filter"))
		}

		// Verify page-size is sent on every request (Document API accepts it with page-key)
		if r.URL.Query().Get("page-size") == "" {
			t.Error("page-size must be sent on every request")
		}

		if pageIndex >= len(pages) {
			t.Error("received more requests than expected pages")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages[pageIndex])
		pageIndex++
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	result, err := h.List(DocumentFilters{ChunkSize: 10, Type: "dashboard"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Documents) != 3 {
		t.Errorf("expected 3 documents across pages, got %d", len(result.Documents))
	}
	if result.TotalCount != 3 {
		t.Errorf("expected TotalCount 3, got %d", result.TotalCount)
	}
}

// --- GetMetadata ---

func TestGetMetadata_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-123/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentMetadata{
			ID:   "doc-123",
			Name: "My Dashboard",
			Type: "dashboard",
		})
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	meta, err := h.GetMetadata("doc-123")
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if meta.ID != "doc-123" {
		t.Errorf("expected ID 'doc-123', got %q", meta.ID)
	}
}

func TestGetMetadata_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/missing/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	_, err := h.GetMetadata("missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetMetadata_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/locked/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	_, err := h.GetMetadata("locked")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-del", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Query().Get("optimistic-locking-version") == "" {
			t.Error("expected optimistic-locking-version query param")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("doc-del", 3)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/gone", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("gone", 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_Conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/stale", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("stale", 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Create ---

func TestCreate_MissingName(t *testing.T) {
	h, cleanup := newDocTestHandler(t, http.NewServeMux())
	defer cleanup()

	_, err := h.Create(CreateRequest{Type: "dashboard", Content: []byte("{}")})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCreate_MissingType(t *testing.T) {
	h, cleanup := newDocTestHandler(t, http.NewServeMux())
	defer cleanup()

	_, err := h.Create(CreateRequest{Name: "My Doc", Content: []byte("{}")})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestCreate_MissingContent(t *testing.T) {
	h, cleanup := newDocTestHandler(t, http.NewServeMux())
	defer cleanup()

	_, err := h.Create(CreateRequest{Name: "My Doc", Type: "dashboard"})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestCreate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "multipart/form-data; boundary=boundary")
		// Return a multipart response with metadata and content parts
		boundary := "test-boundary"
		w.Header().Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n{\"id\":\"new-doc-1\",\"name\":\"My Dashboard\",\"type\":\"dashboard\",\"version\":1}\r\n--%s--\r\n", boundary, boundary)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	doc, err := h.Create(CreateRequest{
		Name:    "My Dashboard",
		Type:    "dashboard",
		Content: []byte(`{"tiles":[]}`),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
}

func TestCreate_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{Name: "Doc", Type: "dashboard", Content: []byte("{}")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- EnvironmentShare ---

func TestCreateEnvironmentShare(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body CreateEnvironmentShareRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.DocumentID != "doc-1" || body.Access != "read" {
			t.Errorf("unexpected body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EnvironmentShare{ID: "share-1", DocumentID: "doc-1", Access: []string{"read"}})
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	got, err := h.CreateEnvironmentShare(CreateEnvironmentShareRequest{DocumentID: "doc-1", Access: "read"})
	if err != nil {
		t.Fatalf("CreateEnvironmentShare: %v", err)
	}
	if got.ID != "share-1" || len(got.Access) != 1 || got.Access[0] != "read" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestListEnvironmentShares_FiltersByDocumentID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if filter != "documentId=='doc-1'" {
			t.Errorf("unexpected filter: %q", filter)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EnvironmentShareList{
			Shares:     []EnvironmentShare{{ID: "s1", DocumentID: "doc-1", Access: []string{"read"}}},
			TotalCount: 1,
		})
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	got, err := h.ListEnvironmentShares("doc-1")
	if err != nil {
		t.Fatalf("ListEnvironmentShares: %v", err)
	}
	if len(got.Shares) != 1 || got.Shares[0].ID != "s1" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestEnsureEnvironmentShare_AlreadyExists_NoOp(t *testing.T) {
	createCalls := 0
	patchCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(EnvironmentShareList{
				Shares:     []EnvironmentShare{{ID: "s1", DocumentID: "doc-1", Access: []string{"read"}}},
				TotalCount: 1,
			})
			return
		}
		if r.Method == http.MethodPost {
			createCalls++
			w.WriteHeader(http.StatusCreated)
		}
	})
	// EnsureEnvironmentShare also flips isPrivate=false; mock metadata + PATCH.
	mux.HandleFunc("/platform/document/v1/documents/doc-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentMetadata{ID: "doc-1", Name: "doc", Type: "notebook", Version: 3, IsPrivate: true})
	})
	mux.HandleFunc("/platform/document/v1/documents/doc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patchCalls++
			w.WriteHeader(http.StatusOK)
		}
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	got, err := h.EnsureEnvironmentShare("doc-1", "read")
	if err != nil {
		t.Fatalf("EnsureEnvironmentShare: %v", err)
	}
	if got.ID != "s1" {
		t.Errorf("expected existing share returned, got %+v", got)
	}
	if createCalls != 0 {
		t.Errorf("expected no create calls, got %d", createCalls)
	}
	if patchCalls != 1 {
		t.Errorf("expected exactly 1 isPrivate PATCH, got %d", patchCalls)
	}
}

func TestEnsureEnvironmentShare_CreatesWhenAbsent(t *testing.T) {
	postCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(EnvironmentShareList{Shares: nil, TotalCount: 0})
			return
		}
		if r.Method == http.MethodPost {
			postCalls++
			json.NewEncoder(w).Encode(EnvironmentShare{ID: "s-new", DocumentID: "doc-1", Access: []string{"read"}})
		}
	})
	mux.HandleFunc("/platform/document/v1/documents/doc-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentMetadata{ID: "doc-1", Name: "doc", Type: "notebook", Version: 1, IsPrivate: true})
	})
	mux.HandleFunc("/platform/document/v1/documents/doc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
		}
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	got, err := h.EnsureEnvironmentShare("doc-1", "read")
	if err != nil {
		t.Fatalf("EnsureEnvironmentShare: %v", err)
	}
	if got.ID != "s-new" {
		t.Errorf("unexpected result: %+v", got)
	}
	if postCalls != 1 {
		t.Errorf("expected exactly 1 create call, got %d", postCalls)
	}
}

func TestEnsureEnvironmentShare_ReplacesDifferentAccess(t *testing.T) {
	var deletedID string
	postCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(EnvironmentShareList{
				Shares:     []EnvironmentShare{{ID: "s-old", DocumentID: "doc-1", Access: []string{"read"}}},
				TotalCount: 1,
			})
			return
		}
		if r.Method == http.MethodPost {
			postCalls++
			json.NewEncoder(w).Encode(EnvironmentShare{ID: "s-new", DocumentID: "doc-1", Access: []string{"read", "write"}})
		}
	})
	mux.HandleFunc("/platform/document/v1/environment-shares/s-old", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedID = "s-old"
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/platform/document/v1/documents/doc-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DocumentMetadata{ID: "doc-1", Name: "doc", Type: "notebook", Version: 1, IsPrivate: true})
	})
	mux.HandleFunc("/platform/document/v1/documents/doc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
		}
	})
	h, cleanup := newDocTestHandler(t, mux)
	defer cleanup()

	got, err := h.EnsureEnvironmentShare("doc-1", "read-write")
	if err != nil {
		t.Fatalf("EnsureEnvironmentShare: %v", err)
	}
	if got.ID != "s-new" || !got.HasAccess("read-write") {
		t.Errorf("unexpected result: %+v", got)
	}
	if deletedID != "s-old" {
		t.Error("expected old share to be deleted")
	}
	if postCalls != 1 {
		t.Errorf("expected 1 create call, got %d", postCalls)
	}
}

// --- documentListItemToDocument / ConvertToDocuments ---

func TestConvertToDocuments(t *testing.T) {
	list := &DocumentList{
		Documents: []DocumentMetadata{
			{ID: "d1", Name: "Dashboard 1", Type: "dashboard", Version: 1},
			{ID: "d2", Name: "Notebook 2", Type: "notebook", Version: 2},
		},
		TotalCount: 2,
	}
	docs := ConvertToDocuments(list)
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	if docs[0].ID != "d1" || docs[1].ID != "d2" {
		t.Errorf("unexpected documents: %v", docs)
	}
}
