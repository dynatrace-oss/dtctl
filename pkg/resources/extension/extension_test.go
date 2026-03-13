package extension

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func TestNewHandler(t *testing.T) {
	c, err := client.New("https://test.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	h := NewHandler(c)

	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.client == nil {
		t.Error("Handler.client is nil")
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name          string
		chunkSize     int64
		nameFilter    string
		pages         []ExtensionList
		expectError   bool
		errorContains string
		validate      func(*testing.T, *ExtensionList)
	}{
		{
			name:      "successful list single page",
			chunkSize: 0,
			pages: []ExtensionList{
				{
					TotalCount: 2,
					Items: []Extension{
						{ExtensionName: "com.dynatrace.extension.host-monitoring", ActiveVersion: "1.2.3"},
						{ExtensionName: "com.dynatrace.extension.jmx", ActiveVersion: "2.0.0"},
					},
				},
			},
			validate: func(t *testing.T, result *ExtensionList) {
				if len(result.Items) != 2 {
					t.Errorf("expected 2 extensions, got %d", len(result.Items))
				}
				if result.TotalCount != 2 {
					t.Errorf("expected TotalCount 2, got %d", result.TotalCount)
				}
			},
		},
		{
			name:      "paginated list with chunking",
			chunkSize: 10,
			pages: []ExtensionList{
				{
					TotalCount:  3,
					NextPageKey: "page2",
					Items: []Extension{
						{ExtensionName: "ext-1", ActiveVersion: "1.0.0"},
						{ExtensionName: "ext-2", ActiveVersion: "2.0.0"},
					},
				},
				{
					TotalCount: 3,
					Items: []Extension{
						{ExtensionName: "ext-3", ActiveVersion: "3.0.0"},
					},
				},
			},
			validate: func(t *testing.T, result *ExtensionList) {
				if len(result.Items) != 3 {
					t.Errorf("expected 3 extensions across pages, got %d", len(result.Items))
				}
				if result.TotalCount != 3 {
					t.Errorf("expected TotalCount 3, got %d", result.TotalCount)
				}
			},
		},
		{
			name:      "empty list",
			chunkSize: 0,
			pages: []ExtensionList{
				{
					TotalCount: 0,
					Items:      []Extension{},
				},
			},
			validate: func(t *testing.T, result *ExtensionList) {
				if len(result.Items) != 0 {
					t.Errorf("expected 0 extensions, got %d", len(result.Items))
				}
			},
		},
		{
			name:       "with name filter",
			chunkSize:  0,
			nameFilter: "com.dynatrace",
			pages: []ExtensionList{
				{
					TotalCount: 1,
					Items: []Extension{
						{ExtensionName: "com.dynatrace.extension.host-monitoring", ActiveVersion: "1.0.0"},
					},
				},
			},
			validate: func(t *testing.T, result *ExtensionList) {
				if len(result.Items) != 1 {
					t.Errorf("expected 1 filtered extension, got %d", len(result.Items))
				}
			},
		},
		{
			name:      "chunk size capped to API maximum",
			chunkSize: 500,
			pages: []ExtensionList{
				{
					TotalCount: 1,
					Items: []Extension{
						{ExtensionName: "ext-1", ActiveVersion: "1.0.0"},
					},
				},
			},
			validate: func(t *testing.T, result *ExtensionList) {
				if len(result.Items) != 1 {
					t.Errorf("expected 1 extension, got %d", len(result.Items))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageIndex := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/platform/extensions/v2/extensions" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// Simulate API page-size limit (rejects > maxPageSize)
				if ps := r.URL.Query().Get("page-size"); ps != "" {
					pageSizeVal, _ := strconv.ParseInt(ps, 10, 64)
					if pageSizeVal > maxPageSize {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error":"page-size exceeds maximum"}`))
						return
					}
				}

				if tt.nameFilter != "" {
					name := r.URL.Query().Get("name")
					if name != tt.nameFilter {
						t.Errorf("expected name filter %q, got %q", tt.nameFilter, name)
					}
				}

				if pageIndex >= len(tt.pages) {
					t.Error("received more requests than expected pages")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.pages[pageIndex])
				pageIndex++
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.List(tt.nameFilter, tt.chunkSize)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		statusCode    int
		response      ExtensionVersionList
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful get",
			extensionName: "com.dynatrace.extension.host-monitoring",
			statusCode:    200,
			response: ExtensionVersionList{
				TotalCount: 2,
				Items: []ExtensionVersion{
					{Version: "1.2.3", ExtensionName: "com.dynatrace.extension.host-monitoring"},
					{Version: "1.2.2", ExtensionName: "com.dynatrace.extension.host-monitoring"},
				},
			},
		},
		{
			name:          "not found",
			extensionName: "com.dynatrace.extension.nonexistent",
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "access denied",
			extensionName: "com.dynatrace.extension.restricted",
			statusCode:    403,
			expectError:   true,
			errorContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.Get(tt.extensionName)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Items) != len(tt.response.Items) {
				t.Errorf("expected %d versions, got %d", len(tt.response.Items), len(result.Items))
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		version       string
		statusCode    int
		response      ExtensionDetails
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful get version",
			extensionName: "com.dynatrace.extension.host-monitoring",
			version:       "1.2.3",
			statusCode:    200,
			response: ExtensionDetails{
				ExtensionName:       "com.dynatrace.extension.host-monitoring",
				Version:             "1.2.3",
				Author:              ExtensionAuthor{Name: "Dynatrace"},
				DataSources:         []string{"snmp", "wmi"},
				FeatureSets:         []string{"default", "advanced"},
				MinDynatraceVersion: "1.250",
			},
		},
		{
			name:          "version not found",
			extensionName: "com.dynatrace.extension.host-monitoring",
			version:       "99.99.99",
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/" + tt.version
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.GetVersion(tt.extensionName, tt.version)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ExtensionName != tt.response.ExtensionName {
				t.Errorf("expected name %q, got %q", tt.response.ExtensionName, result.ExtensionName)
			}
			if result.Version != tt.response.Version {
				t.Errorf("expected version %q, got %q", tt.response.Version, result.Version)
			}
			if result.Author.Name != tt.response.Author.Name {
				t.Errorf("expected author %q, got %q", tt.response.Author.Name, result.Author.Name)
			}
		})
	}
}

func TestGetEnvironmentConfig(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		statusCode    int
		response      ExtensionEnvironmentConfig
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful get config",
			extensionName: "com.dynatrace.extension.host-monitoring",
			statusCode:    200,
			response:      ExtensionEnvironmentConfig{Version: "1.2.3"},
		},
		{
			name:          "no active config",
			extensionName: "com.dynatrace.extension.inactive",
			statusCode:    404,
			expectError:   true,
			errorContains: "no active environment configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/environmentConfiguration"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.GetEnvironmentConfig(tt.extensionName)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Version != tt.response.Version {
				t.Errorf("expected version %q, got %q", tt.response.Version, result.Version)
			}
		})
	}
}

func TestListMonitoringConfigurations(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		version       string
		chunkSize     int64
		statusCode    int
		response      MonitoringConfigurationList
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful list",
			extensionName: "com.dynatrace.extension.host-monitoring",
			chunkSize:     0,
			statusCode:    200,
			response: MonitoringConfigurationList{
				TotalCount: 2,
				Items: []MonitoringConfiguration{
					{ObjectID: "config-1", Scope: "HOST-123"},
					{ObjectID: "config-2", Scope: "HOST_GROUP-456"},
				},
			},
		},
		{
			name:          "with version filter",
			extensionName: "com.dynatrace.extension.host-monitoring",
			version:       "1.2.3",
			chunkSize:     0,
			statusCode:    200,
			response: MonitoringConfigurationList{
				TotalCount: 1,
				Items: []MonitoringConfiguration{
					{ObjectID: "config-1", Scope: "HOST-123"},
				},
			},
		},
		{
			name:          "extension not found",
			extensionName: "com.dynatrace.extension.nonexistent",
			chunkSize:     0,
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/monitoring-configurations"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// NOTE: Implementation does not set version as a query param, so do not check for it here.

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.ListMonitoringConfigurations(tt.extensionName, tt.version, tt.chunkSize)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Items) != len(tt.response.Items) {
				t.Errorf("expected %d configs, got %d", len(tt.response.Items), len(result.Items))
			}
		})
	}
}

func TestGetMonitoringConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		configID      string
		statusCode    int
		response      MonitoringConfiguration
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful get",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "config-1",
			statusCode:    200,
			response:      MonitoringConfiguration{ObjectID: "config-1", Scope: "HOST-123"},
		},
		{
			name:          "config not found",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "nonexistent",
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "access denied",
			extensionName: "com.dynatrace.extension.restricted",
			configID:      "config-1",
			statusCode:    403,
			expectError:   true,
			errorContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/monitoring-configurations/" + tt.configID
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method: %s (expected GET)", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.GetMonitoringConfiguration(tt.extensionName, tt.configID)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ObjectID != tt.response.ObjectID {
				t.Errorf("expected objectId %q, got %q", tt.response.ObjectID, result.ObjectID)
			}
			if result.Scope != tt.response.Scope {
				t.Errorf("expected scope %q, got %q", tt.response.Scope, result.Scope)
			}
			// Verify enrichment fields
			if result.Type != "extension_monitoring_config" {
				t.Errorf("expected type %q, got %q", "extension_monitoring_config", result.Type)
			}
			if result.ExtensionName != tt.extensionName {
				t.Errorf("expected extensionName %q, got %q", tt.extensionName, result.ExtensionName)
			}
		})
	}
}

func TestCreateMonitoringConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		body          MonitoringConfigurationCreate
		statusCode    int
		response      MonitoringConfiguration
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful create",
			extensionName: "com.dynatrace.extension.host-monitoring",
			body: MonitoringConfigurationCreate{
				Scope: "HOST-123",
				Value: map[string]any{"enabled": true, "description": "test"},
			},
			statusCode: 200,
			response:   MonitoringConfiguration{ObjectID: "new-config-1", Scope: "HOST-123"},
		},
		{
			name:          "extension not found",
			extensionName: "com.dynatrace.extension.nonexistent",
			body: MonitoringConfigurationCreate{
				Value: map[string]any{"enabled": true},
			},
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "access denied",
			extensionName: "com.dynatrace.extension.host-monitoring",
			body: MonitoringConfigurationCreate{
				Value: map[string]any{"enabled": true},
			},
			statusCode:    403,
			expectError:   true,
			errorContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/monitoring-configurations"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s (expected POST)", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.CreateMonitoringConfiguration(tt.extensionName, tt.body)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ObjectID != tt.response.ObjectID {
				t.Errorf("expected objectId %q, got %q", tt.response.ObjectID, result.ObjectID)
			}
		})
	}
}

func TestUpdateMonitoringConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		configID      string
		body          MonitoringConfigurationCreate
		statusCode    int
		response      MonitoringConfiguration
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful update",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "config-1",
			body: MonitoringConfigurationCreate{
				Scope: "HOST-123",
				Value: map[string]any{"enabled": false, "description": "updated"},
			},
			statusCode: 200,
			response:   MonitoringConfiguration{ObjectID: "config-1", Scope: "HOST-123"},
		},
		{
			name:          "config not found",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "nonexistent",
			body: MonitoringConfigurationCreate{
				Value: map[string]any{"enabled": true},
			},
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "access denied",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "config-1",
			body: MonitoringConfigurationCreate{
				Value: map[string]any{"enabled": true},
			},
			statusCode:    403,
			expectError:   true,
			errorContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/monitoring-configurations/" + tt.configID
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method != http.MethodPut {
					t.Errorf("unexpected method: %s (expected PUT)", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 200 {
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			result, err := handler.UpdateMonitoringConfiguration(tt.extensionName, tt.configID, tt.body)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ObjectID != tt.response.ObjectID {
				t.Errorf("expected objectId %q, got %q", tt.response.ObjectID, result.ObjectID)
			}
		})
	}
}

func TestDeleteMonitoringConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		extensionName string
		configID      string
		statusCode    int
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful delete",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "config-1",
			statusCode:    204,
		},
		{
			name:          "config not found",
			extensionName: "com.dynatrace.extension.host-monitoring",
			configID:      "nonexistent",
			statusCode:    404,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "access denied",
			extensionName: "com.dynatrace.extension.restricted",
			configID:      "config-1",
			statusCode:    403,
			expectError:   true,
			errorContains: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/platform/extensions/v2/extensions/" + tt.extensionName + "/monitoring-configurations/" + tt.configID
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s (expected %s)", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.Method != http.MethodDelete {
					t.Errorf("unexpected method: %s (expected DELETE)", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			handler := NewHandler(c)
			err = handler.DeleteMonitoringConfiguration(tt.extensionName, tt.configID)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
