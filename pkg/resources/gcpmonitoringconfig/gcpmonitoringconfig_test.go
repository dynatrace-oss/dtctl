package gcpmonitoringconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func newMonitoringHandler(t *testing.T, fn http.HandlerFunc) (*Handler, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(fn)
	c, err := client.New(server.URL, "test-token")
	if err != nil {
		server.Close()
		t.Fatalf("client.New() error = %v", err)
	}
	c.HTTP().SetRetryCount(0)
	return NewHandler(c), server
}

func TestGetListFindAndSchemaHelpers(t *testing.T) {
	calls := 0
	h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == ExtensionAPI {
			_ = json.NewEncoder(w).Encode(ExtensionResponse{Items: []ExtensionItem{{Version: "1.0.1"}}})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/schema") {
			_ = json.NewEncoder(w).Encode(ExtensionSchemaResponse{Enums: map[string]SchemaEnum{
				"dynatrace.datasource.gcp:location": {Items: []SchemaEnumItem{{Value: "us-central1"}}},
				"FeatureSetsType":                   {Items: []SchemaEnumItem{{Value: "compute_engine_essential"}, {Value: "foo_autodiscovery"}}},
			}})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == BaseAPI {
			calls++
			_ = json.NewEncoder(w).Encode(ListResponse{Items: []GCPMonitoringConfig{{ObjectID: "cfg-1", Value: Value{Description: "cfg", Enabled: true, Version: "1.0.1"}}}})
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, BaseAPI+"/") {
			_ = json.NewEncoder(w).Encode(GCPMonitoringConfig{ObjectID: "cfg-1", Value: Value{Description: "cfg", Enabled: true, Version: "1.0.1"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	item, err := h.Get("cfg-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.Description != "cfg" {
		t.Fatalf("unexpected item: %+v", item)
	}

	items, err := h.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected list length: %d", len(items))
	}

	_, err = h.FindByName("cfg")
	if err != nil {
		t.Fatalf("FindByName() error = %v", err)
	}

	locs, err := h.ListAvailableLocations()
	if err != nil || len(locs) != 1 {
		t.Fatalf("ListAvailableLocations() = %#v, err=%v", locs, err)
	}

	sets, err := h.ListAvailableFeatureSets()
	if err != nil || len(sets) != 2 {
		t.Fatalf("ListAvailableFeatureSets() = %#v, err=%v", sets, err)
	}
}

func TestCreateUpdateDeleteAndErrors(t *testing.T) {
	h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(GCPMonitoringConfig{ObjectID: "cfg-1", Value: Value{Description: "created"}})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(GCPMonitoringConfig{ObjectID: "cfg-1", Value: Value{Description: "updated"}})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	created, err := h.Create([]byte(`{"value":{"description":"created"}}`))
	if err != nil || created.ObjectID != "cfg-1" {
		t.Fatalf("Create() err=%v created=%+v", err, created)
	}

	updated, err := h.Update("cfg-1", []byte(`{"value":{"description":"updated"}}`))
	if err != nil || updated.ObjectID != "cfg-1" {
		t.Fatalf("Update() err=%v updated=%+v", err, updated)
	}

	if err := h.Delete("cfg-1"); err != nil {
		t.Fatalf("Delete() err=%v", err)
	}

	hErr, serverErr := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	defer serverErr.Close()

	if _, err := hErr.Get("cfg"); err == nil || !strings.Contains(err.Error(), "failed to get gcp_monitoring_config") {
		t.Fatalf("unexpected get error: %v", err)
	}
	if _, err := hErr.List(); err == nil || !strings.Contains(err.Error(), "failed to list gcp_monitoring_configs") {
		t.Fatalf("unexpected list error: %v", err)
	}
	if _, err := hErr.Create([]byte(`{}`)); err == nil || !strings.Contains(err.Error(), "failed to create gcp_monitoring_config") {
		t.Fatalf("unexpected create error: %v", err)
	}
	if _, err := hErr.Update("cfg", []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "failed to update gcp_monitoring_config") {
		t.Fatalf("unexpected update error: %v", err)
	}
	if err := hErr.Delete("cfg"); err == nil || !strings.Contains(err.Error(), "failed to delete gcp_monitoring_config") {
		t.Fatalf("unexpected delete error: %v", err)
	}
}
