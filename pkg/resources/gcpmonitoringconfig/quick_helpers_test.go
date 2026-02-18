package gcpmonitoringconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
)

func TestSplitCSV(t *testing.T) {
	got := SplitCSV(" a, b ,, c ")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitCSV() = %#v, want %#v", got, want)
	}
}

func TestParseOrDefaultLocationsAndFeatureSets(t *testing.T) {
	calls := 0
	h, server := newMonitoringHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls++
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(ExtensionResponse{Items: []ExtensionItem{{Version: "1.0.0"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(ExtensionSchemaResponse{Enums: map[string]SchemaEnum{
			"dynatrace.datasource.gcp:location": {Items: []SchemaEnumItem{{Value: "us-central1"}, {Value: "europe-west1"}}},
			"FeatureSetsType":                   {Items: []SchemaEnumItem{{Value: "compute_engine_essential"}, {Value: "metrics_all"}}},
		}})
	})
	defer server.Close()

	locs, err := ParseOrDefaultLocations("", h)
	if err != nil {
		t.Fatalf("ParseOrDefaultLocations() error = %v", err)
	}
	if !reflect.DeepEqual(locs, []string{"us-central1", "europe-west1"}) {
		t.Fatalf("unexpected locations: %#v", locs)
	}

	locs, err = ParseOrDefaultLocations("a,b", h)
	if err != nil {
		t.Fatalf("ParseOrDefaultLocations(csv) error = %v", err)
	}
	if !reflect.DeepEqual(locs, []string{"a", "b"}) {
		t.Fatalf("unexpected parsed locations: %#v", locs)
	}

	calls = 0
	sets, err := ParseOrDefaultFeatureSets("", h)
	if err != nil {
		t.Fatalf("ParseOrDefaultFeatureSets() error = %v", err)
	}
	if !reflect.DeepEqual(sets, []string{"compute_engine_essential"}) {
		t.Fatalf("unexpected feature sets: %#v", sets)
	}
}

func TestResolveCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Query().Get("schemaIds") != "" {
			_ = json.NewEncoder(w).Encode(gcpconnection.ListResponse{Items: []gcpconnection.GCPConnection{{
				ObjectID: "obj-1",
				Value: gcpconnection.Value{
					Name:                        "conn-a",
					Type:                        "serviceAccountImpersonation",
					ServiceAccountImpersonation: &gcpconnection.ServiceAccountImpersonation{ServiceAccountID: "sa@test"},
				},
			}}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	c.HTTP().SetRetryCount(0)
	connHandler := gcpconnection.NewHandler(c)

	cred, err := ResolveCredential("conn-a", connHandler)
	if err != nil {
		t.Fatalf("ResolveCredential() error = %v", err)
	}
	if cred.ConnectionID != "obj-1" || cred.ServiceAccount != "sa@test" {
		t.Fatalf("unexpected credential: %#v", cred)
	}

	_, err = ResolveCredential("missing", connHandler)
	if err == nil || !strings.Contains(err.Error(), "not found by name or ID") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
