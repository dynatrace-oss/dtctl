package apply

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
)

// applyGCPConnection applies GCP connection configuration
func (a *Applier) applyGCPConnection(data []byte) ([]ApplyResult, error) {
	var items []map[string]interface{}

	if err := json.Unmarshal(data, &items); err != nil {
		var item map[string]interface{}
		if errSingle := json.Unmarshal(data, &item); errSingle != nil {
			return nil, fmt.Errorf("failed to parse GCP connection JSON: %w", errSingle)
		}
		items = []map[string]interface{}{item}
	}

	handler := gcpconnection.NewHandler(a.client)

	var results []ApplyResult
	var resultWarnings []string
	for _, item := range items {
		objectID, _ := item["objectId"].(string)
		if objectID == "" {
			objectID, _ = item["objectid"].(string)
		}

		schemaID, _ := item["schemaId"].(string)
		if schemaID == "" {
			schemaID, _ = item["schemaid"].(string)
		}

		scope, _ := item["scope"].(string)
		if scope == "" {
			scope = "environment"
		}

		valueMap, ok := item["value"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("GCP connection missing 'value' field")
		}

		valueJSON, err := json.Marshal(valueMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}

		var value gcpconnection.Value
		if err := json.Unmarshal(valueJSON, &value); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value: %w", err)
		}
		if value.Type == "" {
			value.Type = "serviceAccountImpersonation"
		}

		if objectID == "" {
			existing, err := handler.FindByNameAndType(value.Name, value.Type)
			if err == nil && existing != nil {
				objectID = existing.ObjectID
			}
		}

		if objectID == "" {
			res, err := handler.Create(gcpconnection.GCPConnectionCreate{
				SchemaID: schemaID,
				Scope:    scope,
				Value:    value,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create GCP connection: %w", err)
			}

			results = append(results, &ConnectionApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionCreated,
					ResourceType: "gcp_connection",
					ID:           res.ObjectID,
					Name:         value.Name,
				},
				SchemaID: schemaID,
				Scope:    scope,
			})
		} else {
			_, err := handler.Update(objectID, value)
			if err != nil {
				return nil, fmt.Errorf("failed to update GCP connection %s: %w", objectID, err)
			}

			results = append(results, &ConnectionApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionUpdated,
					ResourceType: "gcp_connection",
					ID:           objectID,
					Name:         value.Name,
				},
				SchemaID: schemaID,
				Scope:    scope,
			})
		}
	}

	// Attach collected warnings to the last result
	if len(resultWarnings) > 0 && len(results) > 0 {
		if cr, ok := results[len(results)-1].(*ConnectionApplyResult); ok {
			cr.Warnings = resultWarnings
		}
	}

	return results, nil
}

// applyGCPMonitoringConfig applies GCP monitoring configuration
func (a *Applier) applyGCPMonitoringConfig(data []byte) (ApplyResult, error) {
	handler := gcpmonitoringconfig.NewHandler(a.client)
	document, err := parseCloudMonitoringDocument(data, "googleCloud")
	if err != nil {
		return nil, fmt.Errorf("failed to parse GCP monitoring config JSON: %w", err)
	}

	var config gcpmonitoringconfig.GCPMonitoringConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse GCP monitoring config JSON: %w", err)
	}

	objectID := config.ObjectID

	if config.Value.Version == "" && config.Version != "" {
		config.Value.Version = config.Version
	}

	var warnings []string

	if objectID == "" && config.Value.Description != "" {
		existing, err := handler.FindByName(config.Value.Description)
		if err == nil && existing != nil {
			stderrWarn(&warnings, "Found existing GCP monitoring config %q with ID: %s", config.Value.Description, existing.ObjectID)
			objectID = existing.ObjectID
			config.ObjectID = objectID
		}
	}

	if objectID == "" {
		if config.Value.Version == "" {
			latestVersion, err := handler.GetLatestVersion()
			if err != nil {
				return nil, fmt.Errorf("failed to determine extension version for gcp_monitoring_config: %w", err)
			}
			config.Value.Version = latestVersion
			config.Version = latestVersion
			stderrWarn(&warnings, "Using latest extension version: %s", latestVersion)
		}

		cleanData, err := document.marshal(config, config.Value.GoogleCloud)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal clean config: %w", err)
		}

		res, err := handler.Create(cleanData)
		if err != nil {
			return nil, err
		}
		return &MonitoringConfigApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "gcp_monitoring_config",
				ID:           res.ObjectID,
				Name:         config.Value.Description,
				Warnings:     warnings,
			},
			Scope: config.Scope,
		}, nil
	}

	if config.Value.Version == "" {
		existing, err := handler.Get(objectID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch existing config to preserve version: %w", err)
		}
		stderrWarn(&warnings, "Preserving existing version: %s", existing.Value.Version)
		config.Value.Version = existing.Value.Version
		config.Version = existing.Value.Version
	}

	cleanData, err := document.marshal(config, config.Value.GoogleCloud)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal clean config: %w", err)
	}

	res, err := handler.Update(objectID, cleanData)
	if err != nil {
		return nil, err
	}
	return &MonitoringConfigApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "gcp_monitoring_config",
			ID:           res.ObjectID,
			Name:         config.Value.Description,
			Warnings:     warnings,
		},
		Scope: config.Scope,
	}, nil
}
