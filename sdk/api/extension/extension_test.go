package extension

import (
	"context"
	"encoding/json"
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
	mux.HandleFunc("/platform/extensions/v2/extensions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Simulate API constraint: page-size must not be combined with next-page-key
		if r.URL.Query().Get("page-size") != "" && r.URL.Query().Get("next-page-key") != "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"code":400,"message":"Constraints violated."}}`)
			return
		}

		resp := ExtensionList{
			Items: []Extension{
				{ExtensionName: "com.dynatrace.extension.host", Version: "1.0.0"},
				{ExtensionName: "com.dynatrace.extension.jmx", Version: "2.0.0"},
			},
			TotalCount: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("got %d extensions, want 2", len(result.Items))
	}
	if result.Items[0].ExtensionName != "com.dynatrace.extension.host" {
		t.Errorf("ExtensionName = %q, want %q", result.Items[0].ExtensionName, "com.dynatrace.extension.host")
	}
}

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/extensions/v2/extensions/com.dynatrace.extension.host", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := ExtensionVersionList{
			Items: []ExtensionVersion{
				{Version: "1.0.0", ExtensionName: "com.dynatrace.extension.host", Active: true},
				{Version: "0.9.0", ExtensionName: "com.dynatrace.extension.host"},
			},
			TotalCount: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Get(context.Background(), "com.dynatrace.extension.host")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("got %d versions, want 2", len(result.Items))
	}
	if !result.Items[0].Active {
		t.Error("expected first version to be active")
	}
}

func TestGetVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/extensions/v2/extensions/com.dynatrace.extension.host/1.0.0", func(w http.ResponseWriter, r *http.Request) {
		resp := ExtensionDetails{
			ExtensionName: "com.dynatrace.extension.host",
			Version:       "1.0.0",
			Author:        ExtensionAuthor{Name: "Dynatrace"},
			DataSources:   []string{"snmp"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.GetVersion(context.Background(), "com.dynatrace.extension.host", "1.0.0")
	if err != nil {
		t.Fatalf("GetVersion() error: %v", err)
	}
	if result.ExtensionName != "com.dynatrace.extension.host" {
		t.Errorf("ExtensionName = %q, want %q", result.ExtensionName, "com.dynatrace.extension.host")
	}
	if result.Author.Name != "Dynatrace" {
		t.Errorf("Author.Name = %q, want %q", result.Author.Name, "Dynatrace")
	}
}

func TestDeleteMonitoringConfiguration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/extensions/v2/extensions/com.dynatrace.extension.host/monitoring-configurations/config-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.DeleteMonitoringConfiguration(context.Background(), "com.dynatrace.extension.host", "config-1")
	if err != nil {
		t.Fatalf("DeleteMonitoringConfiguration() error: %v", err)
	}
}
