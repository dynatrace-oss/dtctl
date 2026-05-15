package document

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func newTestClient(t *testing.T, handler http.Handler) *httpclient.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return c
}

func TestList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := DocumentList{
			Documents: []DocumentMetadata{
				{ID: "doc-1", Name: "Dashboard 1", Type: "dashboard", Version: 1},
				{ID: "doc-2", Name: "Notebook 1", Type: "notebook", Version: 2},
			},
			TotalCount: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List(DocumentFilters{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result.Documents) != 2 {
		t.Errorf("got %d documents, want 2", len(result.Documents))
	}
	if result.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", result.TotalCount)
	}
}

func TestList_Paginated(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		pageKey := r.URL.Query().Get("page-key")

		// Document API: page-size IS allowed with page-key
		var resp DocumentList
		if pageKey == "" {
			// First page
			if ps := r.URL.Query().Get("page-size"); ps != "2" {
				t.Errorf("first page: page-size = %q, want %q", ps, "2")
			}
			resp = DocumentList{
				Documents: []DocumentMetadata{
					{ID: "doc-1", Name: "Doc 1", Version: 1},
					{ID: "doc-2", Name: "Doc 2", Version: 1},
				},
				TotalCount:  3,
				NextPageKey: "page2token",
			}
		} else if pageKey == "page2token" {
			// Second page — page-size should still be present (Document API style)
			if ps := r.URL.Query().Get("page-size"); ps != "2" {
				t.Errorf("second page: page-size = %q, want %q (Document API sends page-size on every request)", ps, "2")
			}
			resp = DocumentList{
				Documents: []DocumentMetadata{
					{ID: "doc-3", Name: "Doc 3", Version: 1},
				},
				TotalCount: 3,
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"message":"unknown page key"}}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List(DocumentFilters{ChunkSize: 2})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result.Documents) != 3 {
		t.Errorf("got %d documents, want 3", len(result.Documents))
	}
	if callCount != 2 {
		t.Errorf("API called %d times, want 2", callCount)
	}
}

func TestGetMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-123/metadata", func(w http.ResponseWriter, r *http.Request) {
		resp := DocumentMetadata{
			ID:      "doc-123",
			Name:    "My Dashboard",
			Type:    "dashboard",
			Version: 3,
			Owner:   "user-1",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.GetMetadata("doc-123")
	if err != nil {
		t.Fatalf("GetMetadata() error: %v", err)
	}
	if result.ID != "doc-123" {
		t.Errorf("ID = %q, want %q", result.ID, "doc-123")
	}
	if result.Name != "My Dashboard" {
		t.Errorf("Name = %q, want %q", result.Name, "My Dashboard")
	}
}

func TestDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if v := r.URL.Query().Get("optimistic-locking-version"); v != "5" {
			t.Errorf("optimistic-locking-version = %q, want %q", v, "5")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.Delete("doc-123", 5)
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestCreateEnvironmentShare(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := EnvironmentShare{
			ID:         "share-1",
			DocumentID: "doc-123",
			Access:     []string{"read"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.CreateEnvironmentShare(CreateEnvironmentShareRequest{
		DocumentID: "doc-123",
		Access:     "read",
	})
	if err != nil {
		t.Fatalf("CreateEnvironmentShare() error: %v", err)
	}
	if result.ID != "share-1" {
		t.Errorf("ID = %q, want %q", result.ID, "share-1")
	}
}

func TestCreateEnvironmentShare_Conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `{"error":{"code":409,"message":"share already exists"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.CreateEnvironmentShare(CreateEnvironmentShareRequest{
		DocumentID: "doc-123",
		Access:     "read",
	})
	if err == nil {
		t.Fatal("CreateEnvironmentShare() expected error for 409")
	}
	if !errors.Is(err, ErrShareConflict) {
		t.Errorf("expected ErrShareConflict, got: %v", err)
	}
}

func TestListEnvironmentShares(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := EnvironmentShareList{
			Shares: []EnvironmentShare{
				{ID: "share-1", DocumentID: "doc-123", Access: []string{"read"}},
			},
			TotalCount: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ListEnvironmentShares("doc-123")
	if err != nil {
		t.Fatalf("ListEnvironmentShares() error: %v", err)
	}
	if len(result.Shares) != 1 {
		t.Errorf("got %d shares, want 1", len(result.Shares))
	}
}

func TestDeleteEnvironmentShare(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.DeleteEnvironmentShare("share-1")
	if err != nil {
		t.Fatalf("DeleteEnvironmentShare() error: %v", err)
	}
}
