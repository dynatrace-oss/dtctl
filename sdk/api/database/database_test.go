package database_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/api/database"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *httpclient.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return srv, c
}

func dqlResponse(records []map[string]interface{}) []byte {
	resp := map[string]interface{}{
		"state": "SUCCEEDED",
		"result": map[string]interface{}{
			"records": records,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestList_ReturnsAllVendors(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "DB_INSTANCE_MYSQL-AAA", "name": "mysql-prod", "vendor": "MySQL", "type": "DB_INSTANCE_MYSQL", "host": "mysql.example.com", "port": "3306"},
		{"id": "DB_INSTANCE_POSTGRES-BBB", "name": "pg-prod", "vendor": "PostgreSQL", "type": "DB_INSTANCE_POSTGRES", "host": "pg.example.com", "port": "5432"},
	}
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":execute") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(dqlResponse(records))
	})

	h := database.NewHandler(c)
	list, err := h.List(context.Background(), database.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Databases) != 2 {
		t.Errorf("want 2 databases, got %d", len(list.Databases))
	}
	if list.TotalCount != 2 {
		t.Errorf("want TotalCount=2, got %d", list.TotalCount)
	}
}

func TestList_VendorFilter_BuildsQueryForMatchingNodes(t *testing.T) {
	var capturedBody []byte
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = json.Marshal(map[string]interface{}{}) // reset
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			capturedBody, _ = json.Marshal(body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(dqlResponse(nil))
	})

	h := database.NewHandler(c)
	_, err := h.List(context.Background(), database.ListOptions{Vendor: "post"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("could not parse captured body: %v", err)
	}
	query, _ := reqBody["query"].(string)
	if !strings.Contains(query, "DB_INSTANCE_POSTGRES") {
		t.Errorf("expected query to contain DB_INSTANCE_POSTGRES, got: %s", query)
	}
	if strings.Contains(query, "DB_INSTANCE_MYSQL") {
		t.Errorf("expected query NOT to contain DB_INSTANCE_MYSQL, got: %s", query)
	}
}

func TestList_UnknownVendor_ReturnsEmpty(t *testing.T) {
	called := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write(dqlResponse(nil))
	})

	h := database.NewHandler(c)
	list, err := h.List(context.Background(), database.ListOptions{Vendor: "oracle"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no HTTP call for unknown vendor, but server was hit")
	}
	if len(list.Databases) != 0 {
		t.Errorf("want 0 databases, got %d", len(list.Databases))
	}
}

func TestList_MissingOptionalFields_DoesNotPanic(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "DB_INSTANCE_MSSQL-CCC", "name": "mssql-prod", "vendor": "MSSQL", "type": "DB_INSTANCE_MSSQL"},
	}
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(dqlResponse(records))
	})

	h := database.NewHandler(c)
	list, err := h.List(context.Background(), database.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list.Databases[0].Host != "" || list.Databases[0].Port != "" {
		t.Errorf("expected empty host/port for record without those fields")
	}
}
