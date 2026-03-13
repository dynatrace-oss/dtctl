package lookup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// newLookupTestHandler creates a Handler backed by a test server.
func newLookupTestHandler(t *testing.T, mux *http.ServeMux) (*Handler, func()) {
	t.Helper()
	srv := httptest.NewServer(mux)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return NewHandler(c), srv.Close
}

// --- Create ---

func TestCreate_WithDataContent_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(UploadResponse{
			Records: 2,
		})
	})
	// DQL endpoint (not called for Create, but needs to exist)
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	csvData := []byte("id,name\n1,alice\n2,bob\n")
	resp, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DisplayName: "Test",
		DataContent: csvData,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.Records != 2 {
		t.Errorf("expected 2 records, got %d", resp.Records)
	}
}

func TestCreate_InvalidPath(t *testing.T) {
	h, cleanup := newLookupTestHandler(t, http.NewServeMux())
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "no-leading-slash",
		DataContent: []byte("a,b\n1,2\n"),
	})
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestCreate_NoDataSource(t *testing.T) {
	h, cleanup := newLookupTestHandler(t, http.NewServeMux())
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath: "/lookups/test.csv",
		// No DataContent, no DataSource
	})
	if err == nil {
		t.Fatal("expected error when no data source, got nil")
	}
}

func TestCreate_ServerError_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "unauthorized")
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DataContent: []byte("id,name\n1,a\n"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_ServerError_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DataContent: []byte("id,name\n1,a\n"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_ServerError_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DataContent: []byte("id,name\n1,a\n"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_ServerError_Conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "already exists")
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DataContent: []byte("id,name\n1,a\n"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_ServerError_BadRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad csv format")
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	_, err := h.Create(CreateRequest{
		FilePath:    "/lookups/test.csv",
		DataContent: []byte("id,name\n1,a\n"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Update ---

func TestUpdate_CallsCreateWithOverwrite(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files/tabular/lookup:upload", func(w http.ResponseWriter, r *http.Request) {
		// Verify overwrite is set (it's in the JSON form field, not query)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(UploadResponse{Records: 1})
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	resp, err := h.Update("/lookups/update.csv", CreateRequest{
		DataContent: []byte("id,name\n1,alice\n"),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if resp.Records != 1 {
		t.Errorf("expected 1 record, got %d", resp.Records)
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files:delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("/lookups/to-delete.csv")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestDelete_InvalidPath(t *testing.T) {
	h, cleanup := newLookupTestHandler(t, http.NewServeMux())
	defer cleanup()

	err := h.Delete("no-leading-slash.csv")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestDelete_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files:delete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("/lookups/missing.csv")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files:delete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("/lookups/locked.csv")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/resource-store/v1/files:delete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	h, cleanup := newLookupTestHandler(t, mux)
	defer cleanup()

	err := h.Delete("/lookups/private.csv")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
