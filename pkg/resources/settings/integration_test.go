package settings

import (
	"encoding/json"
	"testing"
)

// TestSettingsObjectDecodingIntegration tests that SettingsObject automatically decodes objectID
func TestSettingsObjectDecodingIntegration(t *testing.T) {
	// Simulate API response with real objectID
	apiResponse := `{
		"objectId": "vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQALQVBQTElDQVRJT04AEDVDOUI5QkIxQjQ1NDY4NTUAJGU0YzY3NDJmLTQ3ZjktM2IxNC04MzQ4LTU5Y2JlMzJmNzk4ML7vVN4V2t6t",
		"schemaId": "builtin:rum.web.name",
		"scope": "APPLICATION-5C9B9BB1B4546855",
		"summary": "Test setting"
	}`

	var obj SettingsObject
	err := json.Unmarshal([]byte(apiResponse), &obj)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Before decoding, UID, ScopeType, and ScopeID should be empty
	if obj.UID != "" {
		t.Errorf("UID should be empty before decoding, got: %s", obj.UID)
	}

	// Decode the objectID
	obj.decodeObjectID()

	// After decoding, UID, ScopeType, and ScopeID should be populated
	expectedUID := "e4c6742f-47f9-3b14-8348-59cbe32f7980"
	if obj.UID != expectedUID {
		t.Errorf("UID = %q, want %q", obj.UID, expectedUID)
	}

	expectedScopeType := "APPLICATION"
	if obj.ScopeType != expectedScopeType {
		t.Errorf("ScopeType = %q, want %q", obj.ScopeType, expectedScopeType)
	}

	expectedScopeID := "5C9B9BB1B4546855"
	if obj.ScopeID != expectedScopeID {
		t.Errorf("ScopeID = %q, want %q", obj.ScopeID, expectedScopeID)
	}

	// Original fields should be preserved
	if obj.ObjectID == "" {
		t.Error("ObjectID should not be empty")
	}
	if obj.SchemaID != "builtin:rum.web.name" {
		t.Errorf("SchemaID = %q, want %q", obj.SchemaID, "builtin:rum.web.name")
	}
}

// TestSettingsObjectDecodingWithInvalidID tests graceful handling of invalid objectIDs
func TestSettingsObjectDecodingWithInvalidID(t *testing.T) {
	tests := []struct {
		name     string
		objectID string
	}{
		{
			name:     "invalid base64",
			objectID: "not-valid-base64!!!",
		},
		{
			name:     "too short",
			objectID: "YWJj", // "abc" in base64
		},
		{
			name:     "empty",
			objectID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := SettingsObject{
				ObjectID: tt.objectID,
				SchemaID: "builtin:test",
			}

			// Should not panic
			obj.decodeObjectID()

			// UID, ScopeType, and ScopeID should remain empty for invalid IDs
			if obj.UID != "" {
				t.Errorf("UID should be empty for invalid objectID, got: %s", obj.UID)
			}
			if obj.ScopeType != "" {
				t.Errorf("ScopeType should be empty for invalid objectID, got: %s", obj.ScopeType)
			}
			if obj.ScopeID != "" {
				t.Errorf("ScopeID should be empty for invalid objectID, got: %s", obj.ScopeID)
			}
		})
	}
}
