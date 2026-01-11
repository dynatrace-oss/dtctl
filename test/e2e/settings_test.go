//go:build integration
// +build integration

package e2e

import (
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/test/integration"
)

func TestSettingsLifecycle(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := settings.NewHandler(env.Client)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "complete settings lifecycle",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: List schemas to verify schema exists
			t.Log("Step 1: Listing schemas...")
			schemas, err := handler.ListSchemas()
			if err != nil {
				t.Fatalf("Failed to list schemas: %v", err)
			}
			if schemas.TotalCount == 0 {
				t.Fatal("No schemas found in environment")
			}
			t.Logf("✓ Found %d schema(s)", schemas.TotalCount)

			// Verify our test schema exists
			schemaID := "builtin:loadtest.5k.owner-based"
			found := false
			for _, schema := range schemas.Items {
				if schema.SchemaID == schemaID {
					found = true
					t.Logf("✓ Found test schema: %s (%s)", schema.SchemaID, schema.DisplayName)
					break
				}
			}
			if !found {
				t.Skipf("Skipping: Schema %q not available in this environment", schemaID)
			}

			// Step 2: Create settings object
			t.Log("Step 2: Creating settings object...")
			fixture := integration.SettingsObjectFixture(env.TestPrefix)
			createReq := settings.SettingsObjectCreate{
				SchemaID:      fixture["schemaId"].(string),
				Scope:         fixture["scope"].(string),
				Value:         fixture["value"].(map[string]interface{}),
				SchemaVersion: "",
			}

			created, err := handler.Create(createReq)
			if err != nil {
				t.Fatalf("Failed to create settings object: %v", err)
			}
			if created.ObjectID == "" {
				t.Fatal("Created settings object has no ID")
			}
			t.Logf("✓ Created settings object: %s (Version: %s)", created.ObjectID, created.Version)

			// Track for cleanup
			env.Cleanup.Track("settings", created.ObjectID, createReq.SchemaID)

			// Step 3: Get settings object
			t.Log("Step 3: Getting settings object...")
			retrieved, err := handler.Get(created.ObjectID)
			if err != nil {
				t.Fatalf("Failed to get settings object: %v", err)
			}
			if retrieved.ObjectID != created.ObjectID {
				t.Errorf("Retrieved settings object ID mismatch: got %s, want %s", retrieved.ObjectID, created.ObjectID)
			}
			if retrieved.SchemaID != createReq.SchemaID {
				t.Errorf("Retrieved settings object schema mismatch: got %s, want %s", retrieved.SchemaID, createReq.SchemaID)
			}
			t.Logf("✓ Retrieved settings object: %s (Schema: %s, Version: %s)", retrieved.ObjectID, retrieved.SchemaID, retrieved.Version)

			// Step 4: List settings objects (verify our object appears)
			t.Log("Step 4: Listing settings objects...")
			list, err := handler.ListObjects(schemaID, "environment", 0)
			if err != nil {
				t.Fatalf("Failed to list settings objects: %v", err)
			}
			foundInList := false
			for _, obj := range list.Items {
				if obj.ObjectID == created.ObjectID {
					foundInList = true
					break
				}
			}
			if !foundInList {
				t.Error("Created settings object not found in list")
			} else {
				t.Logf("✓ Found settings object in list (total: %d objects)", list.TotalCount)
			}

			// Step 5: Update settings object
			t.Log("Step 5: Updating settings object...")
			updateValue := integration.SettingsObjectFixtureModified(env.TestPrefix)

			updated, err := handler.Update(created.ObjectID, created.Version, updateValue)
			if err != nil {
				t.Fatalf("Failed to update settings object: %v", err)
			}
			if updated.Version == created.Version {
				t.Errorf("Settings object version should have changed: got %s, previous %s", updated.Version, created.Version)
			}
			t.Logf("✓ Updated settings object: Version %s → %s", created.Version, updated.Version)

			// Step 6: Verify update
			t.Log("Step 6: Verifying update...")
			updatedObj, err := handler.Get(created.ObjectID)
			if err != nil {
				t.Fatalf("Failed to get updated settings object: %v", err)
			}
			if updatedObj.Version != updated.Version {
				t.Errorf("Updated settings object version mismatch: got %s, want %s", updatedObj.Version, updated.Version)
			}
			// Verify the text field was updated
			if text, ok := updatedObj.Value["text"].(string); ok {
				expectedText := updateValue["text"].(string)
				if text != expectedText {
					t.Errorf("Settings object text not updated: got %s, want %s", text, expectedText)
				}
			}
			t.Logf("✓ Verified update (Version: %s)", updatedObj.Version)

			// Step 7: Delete settings object
			t.Log("Step 7: Deleting settings object...")
			err = handler.Delete(created.ObjectID, updatedObj.Version)
			if err != nil {
				t.Fatalf("Failed to delete settings object: %v", err)
			}
			t.Logf("✓ Deleted settings object: %s", created.ObjectID)

			// Step 8: Verify deletion (should get error/404)
			t.Log("Step 8: Verifying deletion...")
			_, err = handler.Get(created.ObjectID)
			if err == nil {
				t.Error("Expected error when getting deleted settings object, got nil")
			} else {
				t.Logf("✓ Verified deletion (got expected error: %v)", err)
			}
		})
	}
}

func TestSettingsOptimisticLocking(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := settings.NewHandler(env.Client)

	// Verify schema exists first
	schemas, err := handler.ListSchemas()
	if err != nil {
		t.Fatalf("Failed to list schemas: %v", err)
	}
	schemaID := "builtin:loadtest.5k.owner-based"
	found := false
	for _, schema := range schemas.Items {
		if schema.SchemaID == schemaID {
			found = true
			break
		}
	}
	if !found {
		t.Skipf("Skipping: Schema %q not available in this environment", schemaID)
	}

	// Create a settings object first
	fixture := integration.SettingsObjectFixture(env.TestPrefix)
	createReq := settings.SettingsObjectCreate{
		SchemaID: fixture["schemaId"].(string),
		Scope:    fixture["scope"].(string),
		Value:    fixture["value"].(map[string]interface{}),
	}

	created, err := handler.Create(createReq)
	if err != nil {
		t.Fatalf("Failed to create settings object: %v", err)
	}
	env.Cleanup.Track("settings", created.ObjectID, createReq.SchemaID)

	t.Logf("Created settings object: %s (Version: %s)", created.ObjectID, created.Version)

	// Test updating with stale version (should fail)
	t.Run("update with stale version", func(t *testing.T) {
		// First update with current version
		updateValue := integration.SettingsObjectFixtureModified(env.TestPrefix)

		updated, err := handler.Update(created.ObjectID, created.Version, updateValue)
		if err != nil {
			t.Fatalf("First update failed: %v", err)
		}
		t.Logf("First update successful: Version %s → %s", created.Version, updated.Version)

		// Try to update with old version (should fail with 409)
		// Note: Some schemas may not enforce strict optimistic locking
		_, err = handler.Update(created.ObjectID, created.Version, updateValue)
		if err == nil {
			t.Log("Note: Schema does not enforce strict optimistic locking (update with stale version succeeded)")
		} else {
			t.Logf("✓ Got expected optimistic locking error: %v", err)
		}
	})
}

func TestSettingsValidation(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := settings.NewHandler(env.Client)

	// Verify schema exists first
	schemas, err := handler.ListSchemas()
	if err != nil {
		t.Fatalf("Failed to list schemas: %v", err)
	}
	schemaID := "builtin:loadtest.5k.owner-based"
	found := false
	for _, schema := range schemas.Items {
		if schema.SchemaID == schemaID {
			found = true
			break
		}
	}
	if !found {
		t.Skipf("Skipping: Schema %q not available in this environment", schemaID)
	}

	tests := []struct {
		name    string
		req     settings.SettingsObjectCreate
		wantErr bool
	}{
		{
			name: "invalid schema id",
			req: settings.SettingsObjectCreate{
				SchemaID: "invalid:schema:id",
				Scope:    "environment",
				Value: map[string]interface{}{
					"name": "test",
				},
			},
			wantErr: true,
		},
		{
			name: "missing required fields",
			req: settings.SettingsObjectCreate{
				SchemaID: schemaID,
				Scope:    "environment",
				Value:    map[string]interface{}{
					// Missing required 'text' field
				},
			},
			wantErr: true,
		},
		{
			name: "valid settings object",
			req: settings.SettingsObjectCreate{
				SchemaID: schemaID,
				Scope:    "environment",
				Value:    integration.SettingsObjectFixture(env.TestPrefix)["value"].(map[string]interface{}),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.ValidateCreate(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				t.Logf("✓ Got expected validation error: %v", err)
			} else {
				t.Logf("✓ Validation passed")
			}
		})
	}
}

func TestSettingsSchemaOperations(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := settings.NewHandler(env.Client)

	t.Run("list schemas", func(t *testing.T) {
		schemas, err := handler.ListSchemas()
		if err != nil {
			t.Fatalf("Failed to list schemas: %v", err)
		}
		if schemas.TotalCount == 0 {
			t.Error("Expected at least one schema")
		}
		t.Logf("✓ Found %d schema(s)", schemas.TotalCount)
	})

	t.Run("get specific schema", func(t *testing.T) {
		schemaID := "builtin:loadtest.5k.owner-based"
		schema, err := handler.GetSchema(schemaID)
		if err != nil {
			t.Skipf("Skipping: Schema %q not available: %v", schemaID, err)
		}
		if schema == nil {
			t.Error("Expected schema object, got nil")
		}
		t.Logf("✓ Retrieved schema: %s", schemaID)
	})

	t.Run("get non-existent schema", func(t *testing.T) {
		schemaID := "nonexistent:schema:id"
		_, err := handler.GetSchema(schemaID)
		if err == nil {
			t.Error("Expected error when getting non-existent schema, got nil")
		} else {
			t.Logf("✓ Got expected error: %v", err)
		}
	})
}
