package apply

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// applySettings applies a settings object resource
func (a *Applier) applySettings(data []byte) (ApplyResult, error) {
	var setting map[string]interface{}
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, fmt.Errorf("failed to parse settings JSON: %w", err)
	}

	handler := settings.NewHandler(a.client)

	// Extract fields - handle both camelCase (API format) and lowercase (YAML keys)
	objectID, _ := setting["objectId"].(string)
	if objectID == "" {
		objectID, _ = setting["objectid"].(string)
	}

	schemaID, _ := setting["schemaId"].(string)
	if schemaID == "" {
		schemaID, _ = setting["schemaid"].(string)
	}

	scope, _ := setting["scope"].(string)

	value, ok := setting["value"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("settings object missing 'value' field or value is not an object")
	}

	// If no objectID, create new settings object
	if objectID == "" {
		if schemaID == "" {
			return nil, fmt.Errorf("schemaId is required to create a settings object")
		}
		if scope == "" {
			return nil, fmt.Errorf("scope is required to create a settings object")
		}

		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		req := settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		}

		result, err := handler.Create(req)
		if err != nil {
			return nil, fmt.Errorf("failed to create settings object: %w", err)
		}

		return &SettingsApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "settings",
				ID:           result.ObjectID,
			},
			SchemaID: schemaID,
			Scope:    scope,
		}, nil
	}

	// Check if settings object exists
	_, err := handler.Get(objectID)
	if err != nil {
		// Doesn't exist - try to create it
		if schemaID == "" {
			return nil, fmt.Errorf("schemaId is required to create a settings object (objectId %q not found)", objectID)
		}
		if scope == "" {
			return nil, fmt.Errorf("scope is required to create a settings object (objectId %q not found)", objectID)
		}

		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		req := settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		}

		result, err := handler.Create(req)
		if err != nil {
			return nil, fmt.Errorf("failed to create settings object: %w", err)
		}

		return &SettingsApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "settings",
				ID:           result.ObjectID,
			},
			SchemaID: schemaID,
			Scope:    scope,
		}, nil
	}

	// Update existing settings object
	if err := a.checkSafety(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
		return nil, err
	}

	updated, err := handler.Update(objectID, value)
	if err != nil {
		return nil, fmt.Errorf("failed to update settings object: %w", err)
	}

	return &SettingsApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "settings",
			ID:           updated.ObjectID,
			Name:         updated.Summary,
		},
		SchemaID: updated.SchemaID,
		Scope:    updated.Scope,
		Summary:  updated.Summary,
	}, nil
}
