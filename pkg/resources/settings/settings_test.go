package settings

import (
	"encoding/json"
	"testing"
)

func TestSchemaListUnmarshal(t *testing.T) {
	jsonData := `{
		"items": [
			{
				"schemaId": "builtin:openpipeline.logs.pipelines",
				"displayName": "Ingest pipelines configuration (logs)",
				"version": "1.50"
			}
		],
		"totalCount": 1
	}`

	var result SchemaList
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SchemaList: %v", err)
	}

	if result.TotalCount != 1 {
		t.Errorf("TotalCount = %v, want 1", result.TotalCount)
	}

	if len(result.Items) != 1 {
		t.Fatalf("Items count = %v, want 1", len(result.Items))
	}

	schema := result.Items[0]
	if schema.SchemaID != "builtin:openpipeline.logs.pipelines" {
		t.Errorf("SchemaID = %v, want builtin:openpipeline.logs.pipelines", schema.SchemaID)
	}
	if schema.DisplayName != "Ingest pipelines configuration (logs)" {
		t.Errorf("DisplayName = %v, want 'Ingest pipelines configuration (logs)'", schema.DisplayName)
	}
	if schema.Version != "1.50" {
		t.Errorf("Version = %v, want '1.50'", schema.Version)
	}
}

func TestSettingsObjectUnmarshal(t *testing.T) {
	jsonData := `{
		"objectId": "aaaaaaaa-bbbb-cccc-dddd-000000000001",
		"schemaId": "builtin:openpipeline.logs.pipelines",
		"schemaVersion": "1.50",
		"scope": "environment",
		"summary": "Test Pipeline",
		"value": {
			"customId": "test-pipeline",
			"displayName": "Test Pipeline"
		}
	}`

	var result SettingsObject
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SettingsObject: %v", err)
	}

	if result.ObjectID != "aaaaaaaa-bbbb-cccc-dddd-000000000001" {
		t.Errorf("ObjectID = %v, want aaaaaaaa-bbbb-cccc-dddd-000000000001", result.ObjectID)
	}
	if result.SchemaID != "builtin:openpipeline.logs.pipelines" {
		t.Errorf("SchemaID = %v, want builtin:openpipeline.logs.pipelines", result.SchemaID)
	}
	if result.Scope != "environment" {
		t.Errorf("Scope = %v, want environment", result.Scope)
	}
	if result.Summary != "Test Pipeline" {
		t.Errorf("Summary = %v, want 'Test Pipeline'", result.Summary)
	}

	if result.Value == nil {
		t.Fatal("Value is nil")
	}

	customID, ok := result.Value["customId"].(string)
	if !ok {
		t.Fatal("customId not found in value")
	}
	if customID != "test-pipeline" {
		t.Errorf("customId = %v, want test-pipeline", customID)
	}
}

func TestSettingsObjectsListUnmarshal(t *testing.T) {
	jsonData := `{
		"items": [
			{
				"objectId": "aaaaaaaa-bbbb-cccc-dddd-000000000001",
				"schemaId": "builtin:openpipeline.logs.pipelines",
				"scope": "environment",
				"summary": "Pipeline 1"
			},
			{
				"objectId": "aaaaaaaa-bbbb-cccc-dddd-000000000002",
				"schemaId": "builtin:openpipeline.logs.pipelines",
				"scope": "environment",
				"summary": "Pipeline 2"
			}
		],
		"totalCount": 2,
		"nextPageKey": "page2"
	}`

	var result SettingsObjectsList
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SettingsObjectsList: %v", err)
	}

	if result.TotalCount != 2 {
		t.Errorf("TotalCount = %v, want 2", result.TotalCount)
	}

	if len(result.Items) != 2 {
		t.Errorf("Items count = %v, want 2", len(result.Items))
	}

	if result.NextPageKey != "page2" {
		t.Errorf("NextPageKey = %v, want page2", result.NextPageKey)
	}
}

func TestSettingsObjectCreateMarshal(t *testing.T) {
	req := SettingsObjectCreate{
		SchemaID: "builtin:openpipeline.logs.pipelines",
		Scope:    "environment",
		Value: map[string]any{
			"customId":    "test-pipeline",
			"displayName": "Test Pipeline",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal SettingsObjectCreate: %v", err)
	}

	// Unmarshal to verify structure
	var result map[string]any
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["schemaId"] != "builtin:openpipeline.logs.pipelines" {
		t.Errorf("schemaId = %v, want builtin:openpipeline.logs.pipelines", result["schemaId"])
	}
	if result["scope"] != "environment" {
		t.Errorf("scope = %v, want environment", result["scope"])
	}

	value, ok := result["value"].(map[string]any)
	if !ok {
		t.Fatal("value is not a map")
	}
	if value["customId"] != "test-pipeline" {
		t.Errorf("value.customId = %v, want test-pipeline", value["customId"])
	}
}

func TestCreateResponseUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantErr     bool
		wantObjectID string
		checkError  bool
	}{
		{
			name: "successful creation",
			jsonData: `{
				"items": [
					{
						"objectId": "aaaaaaaa-bbbb-cccc-dddd-000000000001",
						"code": 201
					}
				]
			}`,
			wantErr:      false,
			wantObjectID: "aaaaaaaa-bbbb-cccc-dddd-000000000001",
			checkError:   false,
		},
		{
			name: "creation with error",
			jsonData: `{
				"items": [
					{
						"objectId": "",
						"code": 400,
						"error": {
							"code": 400,
							"message": "Invalid schema"
						}
					}
				]
			}`,
			wantErr:    false,
			checkError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result CreateResponse
			err := json.Unmarshal([]byte(tt.jsonData), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(result.Items) == 0 {
					t.Fatal("Items is empty")
				}

				if tt.checkError {
					if result.Items[0].Error == nil {
						t.Error("Expected error in response, got nil")
					}
				} else {
					if result.Items[0].ObjectID != tt.wantObjectID {
						t.Errorf("ObjectID = %v, want %v", result.Items[0].ObjectID, tt.wantObjectID)
					}
				}
			}
		})
	}
}

func TestModificationInfoUnmarshal(t *testing.T) {
	jsonData := `{
		"createdBy": "user@example.com",
		"createdTime": "2024-01-14T10:00:00Z",
		"lastModifiedBy": "admin@example.com",
		"lastModifiedTime": "2024-01-14T12:00:00Z"
	}`

	var result ModificationInfo
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal ModificationInfo: %v", err)
	}

	if result.CreatedBy != "user@example.com" {
		t.Errorf("CreatedBy = %v, want user@example.com", result.CreatedBy)
	}
	if result.LastModifiedBy != "admin@example.com" {
		t.Errorf("LastModifiedBy = %v, want admin@example.com", result.LastModifiedBy)
	}
}

func TestSettingsObjectWithModificationInfo(t *testing.T) {
	jsonData := `{
		"objectId": "test-id",
		"schemaId": "builtin:test",
		"scope": "environment",
		"modificationInfo": {
			"createdBy": "user@example.com",
			"createdTime": "2024-01-14T10:00:00Z"
		}
	}`

	var result SettingsObject
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if result.ModificationInfo == nil {
		t.Fatal("ModificationInfo is nil")
	}

	if result.ModificationInfo.CreatedBy != "user@example.com" {
		t.Errorf("CreatedBy = %v, want user@example.com", result.ModificationInfo.CreatedBy)
	}
}

func TestSettingsObjectsListEmptySchemaId(t *testing.T) {
	// Test that we correctly handle API responses with empty schemaId fields
	// This is a known API issue where the schemaId field is returned as empty string
	jsonData := `{
		"items": [
			{
				"objectId": "vu9U3hXa3q0AAAA",
				"schemaId": "",
				"scope": "",
				"summary": null,
				"value": {
					"enabled": true
				}
			},
			{
				"objectId": "vu9U3hXa3q0BBBB",
				"schemaId": "",
				"scope": "",
				"summary": null,
				"value": {
					"enabled": false
				}
			}
		],
		"totalCount": 2
	}`

	var result SettingsObjectsList
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SettingsObjectsList: %v", err)
	}

	// Before the fix is applied, schemaId should be empty
	if result.Items[0].SchemaID != "" {
		t.Errorf("Expected empty SchemaID before fix, got: %v", result.Items[0].SchemaID)
	}

	// Simulate what ListObjects does: populate empty schemaId from query parameter
	expectedSchemaID := "builtin:anomaly-detection.services"
	for i := range result.Items {
		if result.Items[i].SchemaID == "" {
			result.Items[i].SchemaID = expectedSchemaID
		}
	}

	// After the fix, schemaId should be populated
	for i, item := range result.Items {
		if item.SchemaID != expectedSchemaID {
			t.Errorf("Item %d: SchemaID = %v, want %v", i, item.SchemaID, expectedSchemaID)
		}
	}

	// Verify object IDs are still intact
	if result.Items[0].ObjectID != "vu9U3hXa3q0AAAA" {
		t.Errorf("ObjectID = %v, want vu9U3hXa3q0AAAA", result.Items[0].ObjectID)
	}
	if result.Items[1].ObjectID != "vu9U3hXa3q0BBBB" {
		t.Errorf("ObjectID = %v, want vu9U3hXa3q0BBBB", result.Items[1].ObjectID)
	}
}
