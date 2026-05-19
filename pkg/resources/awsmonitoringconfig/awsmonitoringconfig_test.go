package awsmonitoringconfig

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func newMonitoringHandler(t *testing.T, fn http.HandlerFunc) (*Handler, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(fn)
	c, err := client.NewForTesting(server.URL, "test-token")
	if err != nil {
		server.Close()
		t.Fatalf("client.New() error = %v", err)
	}
	c.HTTP().SetRetryCount(0)
	return NewHandler(c), server
}

// TestListPaginationStitchesPages verifies List follows next-page-key across
// pages and that subsequent requests do not combine page-size with
// next-page-key (Extensions 2.0 API constraint per AGENTS.md).
func TestListPaginationStitchesPages(t *testing.T) {
	pages := []ListResponse{
		{Items: []AWSMonitoringConfig{{ObjectID: "1", Value: Value{Description: "a", Enabled: true, Version: "1.0.0"}}}, NextPageKey: "k2"},
		{Items: []AWSMonitoringConfig{{ObjectID: "2", Value: Value{Description: "b", Enabled: false, Version: "1.0.0"}}}, NextPageKey: "k3"},
		{Items: []AWSMonitoringConfig{{ObjectID: "3", Value: Value{Description: "c", Enabled: true, Version: "1.0.0"}}}},
	}
	calls := 0
	h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		// Constraint guard: page-size must not be combined with next-page-key.
		if r.URL.Query().Get("next-page-key") != "" && r.URL.Query().Get("page-size") != "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"code":400,"message":"Constraints violated."}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pages[calls])
		calls++
	})
	defer server.Close()

	items, err := h.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 page requests, got %d", calls)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d (%+v)", len(items), items)
	}
	for i, want := range []string{"a", "b", "c"} {
		if items[i].Description != want {
			t.Errorf("item[%d].Description = %q, want %q", i, items[i].Description, want)
		}
	}
}
