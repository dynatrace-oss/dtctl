package settings

import (
	"encoding/json"
	"testing"
)

func TestSettingsObject_PopulateDisplayFields_EntityScope(t *testing.T) {
	apiResponse := `{
		"objectId": "vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQALQVBQTElDQVRJT04AEDVDOUI5QkIxQjQ1NDY4NTUAJGU0YzY3NDJmLTQ3ZjktM2IxNC04MzQ4LTU5Y2JlMzJmNzk4ML7vVN4V2t6t",
		"schemaId": "builtin:rum.web.name",
		"scope": "APPLICATION-5C9B9BB1B4546855",
		"summary": "Test setting"
	}`

	var obj SettingsObject
	if err := json.Unmarshal([]byte(apiResponse), &obj); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	obj.populateDisplayFields()

	if obj.ScopeType != "APPLICATION" {
		t.Errorf("ScopeType = %q, want APPLICATION", obj.ScopeType)
	}
	if obj.ScopeID != "5C9B9BB1B4546855" {
		t.Errorf("ScopeID = %q, want 5C9B9BB1B4546855", obj.ScopeID)
	}
	if obj.ObjectIDShort == "" {
		t.Error("ObjectIDShort should not be empty")
	}
}

func TestSettingsObject_PopulateDisplayFields_ComplexScopeID(t *testing.T) {
	// Scope IDs can themselves contain hyphens; only split on the first one
	obj := SettingsObject{
		ObjectID: "someobjectid",
		Scope:    "AZURE_MICROSOFT_RESOURCES_SUBSCRIPTIONS-azure.subscription:3557ab0e-6667-4ed9-b0af-df5ce7248b5c",
	}
	obj.populateDisplayFields()

	if obj.ScopeType != "AZURE_MICROSOFT_RESOURCES_SUBSCRIPTIONS" {
		t.Errorf("ScopeType = %q, want AZURE_MICROSOFT_RESOURCES_SUBSCRIPTIONS", obj.ScopeType)
	}
	if obj.ScopeID != "azure.subscription:3557ab0e-6667-4ed9-b0af-df5ce7248b5c" {
		t.Errorf("ScopeID = %q, want azure.subscription:3557ab0e-6667-4ed9-b0af-df5ce7248b5c", obj.ScopeID)
	}
}

func TestSettingsObject_PopulateDisplayFields_SingletonScope(t *testing.T) {
	// Singleton scopes like "environment" or "tenant" have no entity ID
	for _, scope := range []string{"environment", "tenant"} {
		obj := SettingsObject{ObjectID: "x", Scope: scope}
		obj.populateDisplayFields()

		if obj.ScopeType != scope {
			t.Errorf("scope %q: ScopeType = %q, want %q", scope, obj.ScopeType, scope)
		}
		if obj.ScopeID != "" {
			t.Errorf("scope %q: ScopeID = %q, want empty", scope, obj.ScopeID)
		}
	}
}

func TestSettingsObject_PopulateDisplayFields_EmptyScope(t *testing.T) {
	obj := SettingsObject{ObjectID: "someobjectid", Scope: ""}
	obj.populateDisplayFields()

	if obj.ScopeType != "" {
		t.Errorf("ScopeType = %q, want empty for empty scope", obj.ScopeType)
	}
	if obj.ScopeID != "" {
		t.Errorf("ScopeID = %q, want empty for empty scope", obj.ScopeID)
	}
}

func TestSettingsObject_PopulateDisplayFields_ObjectIDShortTruncation(t *testing.T) {
	long := SettingsObject{ObjectID: "vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ"}
	long.populateDisplayFields()
	if long.ObjectIDShort != "vu9U3hXa3q0AAAABABRi..." {
		t.Errorf("ObjectIDShort = %q, want truncated form", long.ObjectIDShort)
	}

	short := SettingsObject{ObjectID: "abc"}
	short.populateDisplayFields()
	if short.ObjectIDShort != "abc" {
		t.Errorf("ObjectIDShort = %q, want abc", short.ObjectIDShort)
	}
}
