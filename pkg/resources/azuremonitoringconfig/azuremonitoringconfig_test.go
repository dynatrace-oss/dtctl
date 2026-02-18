package azuremonitoringconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestCompareVersion(t *testing.T) {
	if got := compareVersion("1.2.10", "1.2.3"); got <= 0 {
		t.Fatalf("compareVersion expected positive, got %d", got)
	}
	if got := compareVersion("2.0", "2.0.0"); got != 0 {
		t.Fatalf("compareVersion expected 0, got %d", got)
	}
	if got := compareVersion("1.0", "1.0.1"); got >= 0 {
		t.Fatalf("compareVersion expected negative, got %d", got)
	}
}

func TestGetLatestVersion(t *testing.T) {
	t.Run("success chooses highest version", func(t *testing.T) {
		h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ExtensionResponse{Items: []ExtensionItem{{Version: "1.4.0"}, {Version: "1.12.0"}, {Version: "1.9.8"}}})
		})
		defer server.Close()

		version, err := h.GetLatestVersion()
		if err != nil {
			t.Fatalf("GetLatestVersion() error = %v", err)
		}
		if version != "1.12.0" {
			t.Fatalf("GetLatestVersion() = %q, want 1.12.0", version)
		}
	})

	t.Run("empty versions", func(t *testing.T) {
		h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ExtensionResponse{Items: []ExtensionItem{{Version: ""}}})
		})
		defer server.Close()

		_, err := h.GetLatestVersion()
		if err == nil || !strings.Contains(err.Error(), "no versions found") {
			t.Fatalf("GetLatestVersion() err = %v", err)
		}
	})
}

func TestListAvailableLocationsAndFeatureSets(t *testing.T) {
	calls := 0
	h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls++
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(ExtensionResponse{Items: []ExtensionItem{{Version: "2.0.0"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(ExtensionSchemaResponse{Enums: map[string]SchemaEnum{
			"dynatrace.datasource.azure:location": {Items: []SchemaEnumItem{{Value: "westeurope"}, {Value: ""}, {Value: "eastus"}}},
			"FeatureSetsType":                  {Items: []SchemaEnumItem{{Value: "logs_essential"}, {Value: "metrics_all"}, {Value: "alerts_essential"}}},
		}})
	})
	defer server.Close()

	locations, err := h.ListAvailableLocations()
	if err != nil {
		t.Fatalf("ListAvailableLocations() error = %v", err)
	}
	if len(locations) != 2 {
		t.Fatalf("locations unexpected: %+v", locations)
	}

	calls = 0
	featureSets, err := h.ListAvailableFeatureSets()
	if err != nil {
		t.Fatalf("ListAvailableFeatureSets() error = %v", err)
	}
	if len(featureSets) != 3 {
		t.Fatalf("feature sets unexpected: %+v", featureSets)
	}
	if !slices.IsSortedFunc(featureSets, func(a, b FeatureSet) int { return strings.Compare(a.Value, b.Value) }) {
		t.Fatalf("feature sets not sorted: %+v", featureSets)
	}
}

func TestMonitoringCrudAndFindByName(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		getCalls := 0
		h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				getCalls++
				if getCalls == 1 {
					_ = json.NewEncoder(w).Encode(AzureMonitoringConfig{ObjectID: "cfg-1", Value: Value{Description: "cfg-a", Enabled: true, Version: "v1"}})
					return
				}
				_ = json.NewEncoder(w).Encode(ListResponse{Items: []AzureMonitoringConfig{{ObjectID: "cfg-1", Value: Value{Description: "cfg-a", Enabled: true, Version: "v1"}}}})
			case http.MethodPost:
				_ = json.NewEncoder(w).Encode(AzureMonitoringConfig{ObjectID: "cfg-created", Value: Value{Description: "cfg-created"}})
			case http.MethodPut:
				_ = json.NewEncoder(w).Encode(AzureMonitoringConfig{ObjectID: "cfg-1", Value: Value{Description: "cfg-updated"}})
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})
		defer server.Close()

		got, err := h.Get("cfg-1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Description != "cfg-a" || got.Version != "v1" || !got.Enabled {
			t.Fatalf("Get() flattening failed: %+v", got)
		}

		list, err := h.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 1 || list[0].Description != "cfg-a" {
			t.Fatalf("List() unexpected result: %+v", list)
		}

		found, err := h.FindByName("cfg-a")
		if err != nil {
			t.Fatalf("FindByName() error = %v", err)
		}
		if found.ObjectID != "cfg-1" {
			t.Fatalf("FindByName() ObjectID = %q", found.ObjectID)
		}

		created, err := h.Create([]byte(`{"value":{"description":"cfg-created"}}`))
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if created.ObjectID != "cfg-created" {
			t.Fatalf("Create() ObjectID = %q", created.ObjectID)
		}

		updated, err := h.Update("cfg-1", []byte(`{"value":{"description":"cfg-updated"}}`))
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if updated.Value.Description != "cfg-updated" {
			t.Fatalf("Update() result unexpected: %+v", updated)
		}

		if err := h.Delete("cfg-1"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
	})

	t.Run("error paths", func(t *testing.T) {
		h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("boom"))
			case http.MethodPost:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("bad"))
			case http.MethodPut:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("boom"))
			case http.MethodDelete:
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("forbidden"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})
		defer server.Close()

		if _, err := h.Get("cfg-404"); err == nil || !strings.Contains(err.Error(), "failed to get azure_monitoring_config") {
			t.Fatalf("Get() expected error, got %v", err)
		}
		if _, err := h.List(); err == nil || !strings.Contains(err.Error(), "failed to list azure_monitoring_configs") {
			t.Fatalf("List() expected error, got %v", err)
		}
		if _, err := h.Create([]byte(`{}`)); err == nil || !strings.Contains(err.Error(), "failed to create azure_monitoring_config") {
			t.Fatalf("Create() expected error, got %v", err)
		}
		if _, err := h.Update("cfg-1", []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "failed to update azure_monitoring_config") {
			t.Fatalf("Update() expected error, got %v", err)
		}
		if err := h.Delete("cfg-1"); err == nil || !strings.Contains(err.Error(), "failed to delete azure_monitoring_config") {
			t.Fatalf("Delete() expected error, got %v", err)
		}
	})
}
