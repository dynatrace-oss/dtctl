package apply

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWorkflowApplyResultJSON(t *testing.T) {
	result := &WorkflowApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "workflow",
			ID:           "wf-123",
			Name:         "My Workflow",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["action"] != "created" {
		t.Errorf("expected action=created, got %v", parsed["action"])
	}
	if parsed["resourceType"] != "workflow" {
		t.Errorf("expected type=workflow, got %v", parsed["resourceType"])
	}
	if parsed["id"] != "wf-123" {
		t.Errorf("expected id=wf-123, got %v", parsed["id"])
	}
	if parsed["name"] != "My Workflow" {
		t.Errorf("expected name=My Workflow, got %v", parsed["name"])
	}
	// Should NOT contain dashboard-specific fields
	if _, ok := parsed["tileCount"]; ok {
		t.Error("workflow result should not have tileCount")
	}
	if _, ok := parsed["url"]; ok {
		t.Error("workflow result should not have url")
	}
}

func TestDashboardApplyResultJSON(t *testing.T) {
	result := &DashboardApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "dashboard",
			ID:           "dash-123",
			Name:         "My Dashboard",
		},
		URL:       "https://env.dt.com/ui/document/v0/#/dashboards/dash-123",
		TileCount: 5,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["url"] != "https://env.dt.com/ui/document/v0/#/dashboards/dash-123" {
		t.Errorf("unexpected url: %v", parsed["url"])
	}
	if parsed["tileCount"] != float64(5) {
		t.Errorf("expected tileCount=5, got %v", parsed["tileCount"])
	}
}

func TestNotebookApplyResultJSON(t *testing.T) {
	result := &NotebookApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "notebook",
			ID:           "nb-456",
			Name:         "My Notebook",
		},
		URL:          "https://env.dt.com/ui/document/v0/#/notebooks/nb-456",
		SectionCount: 3,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["action"] != "updated" {
		t.Errorf("expected action=updated, got %v", parsed["action"])
	}
	if parsed["url"] != "https://env.dt.com/ui/document/v0/#/notebooks/nb-456" {
		t.Errorf("unexpected url: %v", parsed["url"])
	}
	if parsed["sectionCount"] != float64(3) {
		t.Errorf("expected sectionCount=3, got %v", parsed["sectionCount"])
	}
}

func TestSLOApplyResultJSON(t *testing.T) {
	result := &SLOApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "slo",
			ID:           "slo-789",
			Name:         "Availability SLO",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["action"] != "created" {
		t.Errorf("expected action=created, got %v", parsed["action"])
	}
	if parsed["resourceType"] != "slo" {
		t.Errorf("expected type=slo, got %v", parsed["resourceType"])
	}
	// SLO has no extra fields
	if _, ok := parsed["url"]; ok {
		t.Error("slo result should not have url")
	}
}

func TestSettingsApplyResultJSON(t *testing.T) {
	result := &SettingsApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "settings",
			ID:           "obj-123",
			Name:         "",
		},
		SchemaID: "builtin:openpipeline.logs.pipelines",
		Scope:    "environment",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["schemaId"] != "builtin:openpipeline.logs.pipelines" {
		t.Errorf("unexpected schemaId: %v", parsed["schemaId"])
	}
	if parsed["scope"] != "environment" {
		t.Errorf("unexpected scope: %v", parsed["scope"])
	}
	// Name should be omitted when empty (omitempty)
	if _, ok := parsed["name"]; ok {
		t.Error("empty name should be omitted from JSON")
	}
}

func TestSettingsApplyResultWithSummaryJSON(t *testing.T) {
	result := &SettingsApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "settings",
			ID:           "obj-456",
			Name:         "Alert Profile",
		},
		SchemaID: "builtin:alerting.profile",
		Scope:    "environment",
		Summary:  "Alert profile for production",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["summary"] != "Alert profile for production" {
		t.Errorf("unexpected summary: %v", parsed["summary"])
	}
}

func TestBucketApplyResultJSON(t *testing.T) {
	result := &BucketApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "bucket",
			ID:           "my-bucket",
			Name:         "my-bucket",
			Warnings:     []string{"Bucket creation can take up to 1 minute to complete"},
		},
		Status: "creating",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["status"] != "creating" {
		t.Errorf("unexpected status: %v", parsed["status"])
	}
	warnings, ok := parsed["warnings"].([]interface{})
	if !ok {
		t.Fatal("warnings should be an array")
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0] != "Bucket creation can take up to 1 minute to complete" {
		t.Errorf("unexpected warning: %v", warnings[0])
	}
}

func TestConnectionApplyResultJSON(t *testing.T) {
	result := &ConnectionApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "gcp-connection",
			ID:           "conn-123",
			Name:         "GCP Production",
		},
		SchemaID: "builtin:hyperscaler-authentication.connections.gcp",
		Scope:    "environment",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["schemaId"] != "builtin:hyperscaler-authentication.connections.gcp" {
		t.Errorf("unexpected schemaId: %v", parsed["schemaId"])
	}
	if parsed["id"] != "conn-123" {
		t.Errorf("unexpected id: %v", parsed["id"])
	}
}

func TestMonitoringConfigApplyResultJSON(t *testing.T) {
	result := &MonitoringConfigApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "gcp-monitoring-config",
			ID:           "mc-789",
			Name:         "GCP Monitoring",
		},
		Scope: "integration-gcp",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["resourceType"] != "gcp-monitoring-config" {
		t.Errorf("unexpected resourceType: %v", parsed["resourceType"])
	}
	if parsed["scope"] != "integration-gcp" {
		t.Errorf("unexpected scope: %v", parsed["scope"])
	}
	// MonitoringConfigApplyResult no longer has SchemaID (API doesn't provide it)
	if _, ok := parsed["schemaId"]; ok {
		t.Error("monitoring config result should not have schemaId")
	}
}

func TestOmitEmptyFields(t *testing.T) {
	// Workflow has no extra fields — verify dashboard-specific fields are absent
	result := &WorkflowApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "workflow",
			ID:           "wf-1",
			Name:         "Test",
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	for _, field := range []string{"url", "tileCount", "schemaId", "scope", "objectId", "status", "warnings"} {
		if strings.Contains(s, field) {
			t.Errorf("workflow JSON should not contain %q, got: %s", field, s)
		}
	}
}

func TestDashboardOmitEmptyOptionalFields(t *testing.T) {
	// Dashboard with no URL and zero TileCount — URL should be omitted, TileCount should be omitted
	result := &DashboardApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "dashboard",
			ID:           "dash-1",
			Name:         "Test",
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	if strings.Contains(s, "url") {
		t.Errorf("empty url should be omitted, got: %s", s)
	}
	if strings.Contains(s, "tileCount") {
		t.Errorf("zero tileCount should be omitted, got: %s", s)
	}
}

func TestWarningsOmittedWhenEmpty(t *testing.T) {
	result := &BucketApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "bucket",
			ID:           "b-1",
			Name:         "test-bucket",
		},
		Status: "active",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), "warnings") {
		t.Errorf("nil warnings should be omitted from JSON, got: %s", string(data))
	}
}

func TestSettingsSummaryOmittedWhenEmpty(t *testing.T) {
	result := &SettingsApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "settings",
			ID:           "s-1",
			Name:         "",
		},
		SchemaID: "builtin:alerting.profile",
		Scope:    "environment",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), "summary") {
		t.Errorf("empty summary should be omitted from JSON, got: %s", string(data))
	}
}

func TestApplyResultInterface(t *testing.T) {
	// All types satisfy ApplyResult interface (compile-time check)
	var _ ApplyResult = &WorkflowApplyResult{}
	var _ ApplyResult = &DashboardApplyResult{}
	var _ ApplyResult = &NotebookApplyResult{}
	var _ ApplyResult = &SLOApplyResult{}
	var _ ApplyResult = &BucketApplyResult{}
	var _ ApplyResult = &SettingsApplyResult{}
	var _ ApplyResult = &ConnectionApplyResult{}
	var _ ApplyResult = &MonitoringConfigApplyResult{}
}

func TestActionConstants(t *testing.T) {
	if ActionCreated != "created" {
		t.Errorf("ActionCreated = %q, want %q", ActionCreated, "created")
	}
	if ActionUpdated != "updated" {
		t.Errorf("ActionUpdated = %q, want %q", ActionUpdated, "updated")
	}
	if ActionUnchanged != "unchanged" {
		t.Errorf("ActionUnchanged = %q, want %q", ActionUnchanged, "unchanged")
	}
}

func TestYAMLSerialization(t *testing.T) {
	tests := []struct {
		name   string
		result ApplyResult
		checks map[string]interface{}
	}{
		{
			name: "workflow",
			result: &WorkflowApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionCreated,
					ResourceType: "workflow",
					ID:           "wf-1",
					Name:         "Test WF",
				},
			},
			checks: map[string]interface{}{
				"action":       "created",
				"resourceType": "workflow",
				"id":           "wf-1",
				"name":         "Test WF",
			},
		},
		{
			name: "dashboard with url",
			result: &DashboardApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionUpdated,
					ResourceType: "dashboard",
					ID:           "dash-1",
					Name:         "Test Dash",
				},
				URL:       "https://env.dt.com/dash/1",
				TileCount: 7,
			},
			checks: map[string]interface{}{
				"action":       "updated",
				"resourceType": "dashboard",
				"url":          "https://env.dt.com/dash/1",
				"tileCount":    7,
			},
		},
		{
			name: "settings",
			result: &SettingsApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionCreated,
					ResourceType: "settings",
					ID:           "s-1",
					Name:         "",
				},
				SchemaID: "builtin:alerting.profile",
				Scope:    "environment",
			},
			checks: map[string]interface{}{
				"schemaId": "builtin:alerting.profile",
				"scope":    "environment",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.result)
			if err != nil {
				t.Fatal(err)
			}

			var parsed map[string]interface{}
			if err := yaml.Unmarshal(data, &parsed); err != nil {
				t.Fatal(err)
			}

			for key, expected := range tt.checks {
				got := parsed[key]
				if got != expected {
					t.Errorf("YAML key %q = %v (%T), want %v (%T)", key, got, got, expected, expected)
				}
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := &DashboardApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionCreated,
			ResourceType: "dashboard",
			ID:           "dash-rt",
			Name:         "Round Trip Dashboard",
			Warnings:     []string{"warning 1", "warning 2"},
		},
		URL:       "https://env.dt.com/ui/document/v0/#/dashboards/dash-rt",
		TileCount: 12,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded DashboardApplyResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Action != original.Action {
		t.Errorf("Action = %q, want %q", decoded.Action, original.Action)
	}
	if decoded.ResourceType != original.ResourceType {
		t.Errorf("ResourceType = %q, want %q", decoded.ResourceType, original.ResourceType)
	}
	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.URL != original.URL {
		t.Errorf("URL = %q, want %q", decoded.URL, original.URL)
	}
	if decoded.TileCount != original.TileCount {
		t.Errorf("TileCount = %d, want %d", decoded.TileCount, original.TileCount)
	}
	if len(decoded.Warnings) != len(original.Warnings) {
		t.Errorf("Warnings len = %d, want %d", len(decoded.Warnings), len(original.Warnings))
	}
	for i, w := range decoded.Warnings {
		if w != original.Warnings[i] {
			t.Errorf("Warnings[%d] = %q, want %q", i, w, original.Warnings[i])
		}
	}
}
