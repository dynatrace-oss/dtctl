package apply

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
)

// applyAzureConnection applies Azure connection (credential)
func (a *Applier) applyAzureConnection(data []byte) ([]ApplyResult, error) {
	// Azure connection input might be a single object or a list of setting objects
	var items []map[string]interface{}

	// Try parsing as array first
	err := json.Unmarshal(data, &items)
	if err != nil {
		// Not an array, try parsing as single object
		var item map[string]interface{}
		if errSingle := json.Unmarshal(data, &item); errSingle != nil {
			return nil, fmt.Errorf("failed to parse Azure connection JSON: %w", errSingle)
		}
		items = []map[string]interface{}{item}
	}

	handler := azureconnection.NewHandler(a.client)

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
			return nil, fmt.Errorf("azure connection missing 'value' field")
		}

		// Convert valueMap to Value struct
		valueJSON, err := json.Marshal(valueMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}

		var value azureconnection.Value
		if err := json.Unmarshal(valueJSON, &value); err != nil {
			return nil, fmt.Errorf("failed to unmarshal value: %w", err)
		}

		// Auto-lookup for Federated Credentials if ObjectID is missing
		if objectID == "" && value.Type == "federatedIdentityCredential" {
			existing, err := handler.FindByNameAndType(value.Name, value.Type)
			if err != nil {
				// Log warning but proceed to try create
				stderrWarn(&resultWarnings, "Failed to lookup existing connection: %v", err)
			} else if existing != nil {
				objectID = existing.ObjectID
				stderrWarn(&resultWarnings, "Found existing Federated Credential connection %q (ID: %s), switching to update mode", value.Name, objectID)
			}
		}

		// Read optional issuer override — lets callers specify a custom token issuer
		// in the YAML without requiring auto-detection from the host name.
		issuerOverride, _ := item["issuer"].(string)

		if objectID == "" {
			// Create
			req := azureconnection.AzureConnectionCreate{
				SchemaID: schemaID,
				Scope:    scope,
				Value:    value,
			}
			res, err := handler.Create(req)
			if err != nil {
				return nil, fmt.Errorf("failed to create Azure connection: %w", err)
			}

			// Check for federated identity to print instructions
			if value.Type == "federatedIdentityCredential" {
				printFederatedInstructions(a.baseURL, res.ObjectID, issuerOverride, &resultWarnings)
			}

			results = append(results, &ConnectionApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionCreated,
					ResourceType: "azure_connection",
					ID:           res.ObjectID,
					Name:         value.Name,
				},
				SchemaID: schemaID,
				Scope:    scope,
			})
		} else {
			// Update
			_, err := handler.Update(objectID, value)
			if err != nil {
				errMsg := err.Error()

				// Catch generic validation error that happens when Azure side is not ready/configured
				// "was unable to be validated with validator .../azureConfiguration"
				if strings.Contains(errMsg, "azureConfiguration") && strings.Contains(errMsg, "unable to be validated") {
					// Check if we have incomplete configuration (missing app/directory ID)
					if value.Type == "federatedIdentityCredential" {
						fedCred := value.FederatedIdentityCredential
						if fedCred == nil || fedCred.ApplicationID == "" || fedCred.DirectoryID == "" {
							printFederatedCompleteInstructions(a.baseURL, objectID, value.Name, issuerOverride)
							return nil, fmt.Errorf("azure connection requires additional configuration: %w", err)
						}
					}
				}

				// Check for Federated Identity error (AADSTS70025 or AADSTS700213)
				if strings.Contains(errMsg, "AADSTS70025") || strings.Contains(errMsg, "AADSTS700213") {
					if value.FederatedIdentityCredential != nil && value.FederatedIdentityCredential.ApplicationID != "" {
						printFederatedErrorSnippet(a.baseURL, objectID, value.FederatedIdentityCredential.ApplicationID, issuerOverride)
						return nil, fmt.Errorf("azure connection requires federation setup on Azure side: %w", err)
					}
				}
				return nil, fmt.Errorf("failed to update Azure connection %s: %w", objectID, err)
			}

			results = append(results, &ConnectionApplyResult{
				ApplyResultBase: ApplyResultBase{
					Action:       ActionUpdated,
					ResourceType: "azure_connection",
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

// applyAzureMonitoringConfig applies Azure monitoring configuration
func (a *Applier) applyAzureMonitoringConfig(data []byte) (ApplyResult, error) {
	handler := azuremonitoringconfig.NewHandler(a.client)

	// Unmarshal to struct to handle casing properly via json tags
	var config azuremonitoringconfig.AzureMonitoringConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse Azure monitoring config JSON: %w", err)
	}

	objectID := config.ObjectID

	if config.Value.Version == "" && config.Version != "" {
		config.Value.Version = config.Version
	}

	var warnings []string

	// Lookup by name if ID is missing (Feature 1: naming convention lookup)
	if objectID == "" && config.Value.Description != "" {
		existing, err := handler.FindByName(config.Value.Description)
		if err == nil && existing != nil {
			stderrWarn(&warnings, "Found existing Azure monitoring config %q with ID: %s", config.Value.Description, existing.ObjectID)
			objectID = existing.ObjectID
			config.ObjectID = objectID // Set ID for update
		}
	}

	if objectID == "" {
		if config.Value.Version == "" {
			latestVersion, err := handler.GetLatestVersion()
			if err != nil {
				return nil, fmt.Errorf("failed to determine extension version for azure_monitoring_config: %w", err)
			}
			config.Value.Version = latestVersion
			config.Version = latestVersion
			stderrWarn(&warnings, "Using latest extension version: %s", latestVersion)
		}

		// New creation
		cleanData, err := json.Marshal(config)
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
				ResourceType: "azure_monitoring_config",
				ID:           res.ObjectID,
				Name:         config.Value.Description,
				Warnings:     warnings,
			},
			Scope: config.Scope,
		}, nil
	}

	// Update existing

	// Feature 2: If version is missing in YAML, preserve existing version
	if config.Value.Version == "" {
		existing, err := handler.Get(objectID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch existing config to preserve version: %w", err)
		} else {
			stderrWarn(&warnings, "Preserving existing version: %s", existing.Value.Version)
			config.Value.Version = existing.Value.Version
			config.Version = existing.Value.Version
		}
	}

	cleanData, err := json.Marshal(config)
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
			ResourceType: "azure_monitoring_config",
			ID:           res.ObjectID,
			Name:         config.Value.Description,
			Warnings:     warnings,
		},
		Scope: config.Scope,
	}, nil
}

// printFederatedInstructions prints configuration instructions for Federated Identity Credential to stderr
func printFederatedInstructions(baseURL, objectID, issuerOverride string, warnings *[]string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		// Should not happen if client is initialized correctly, but fail gracefully
		output.PrintWarning("Could not parse base URL for instructions: %v", err)
		return
	}
	host := u.Host

	issuer := issuerOverride
	if issuer == "" {
		issuer = azureconnection.TokenIssuerForHost(host)
	}

	fmt.Fprintf(os.Stderr, "\nFurther configuration required in Azure Portal (Federated Credentials):\n")
	fmt.Fprintf(os.Stderr, "  Issuer:    %s\n", issuer)
	fmt.Fprintf(os.Stderr, "  Subject:   dt:connection-id/%s\n", objectID)
	fmt.Fprintf(os.Stderr, "  Audiences: %s/svc-id/com.dynatrace.da\n", host)

	if warnings != nil {
		*warnings = append(*warnings, "Azure federated credential requires additional portal setup")
	}
}

// printFederatedCompleteInstructions prints full configuration instructions for Federated Identity Credential to stderr
func printFederatedCompleteInstructions(baseURL, objectID, connectionName, issuerOverride string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		output.PrintWarning("Could not parse base URL for instructions: %v", err)
		return
	}
	host := u.Host

	issuer := issuerOverride
	if issuer == "" {
		issuer = azureconnection.TokenIssuerForHost(host)
	}

	fmt.Fprintf(os.Stderr, "\nTo complete the configuration, additional setup is required in the Azure Portal (Federated Credentials).\n")
	fmt.Fprintf(os.Stderr, "Details for Azure configuration:\n")
	fmt.Fprintf(os.Stderr, "  Issuer:    %s\n", issuer)
	fmt.Fprintf(os.Stderr, "  Subject:   dt:connection-id/%s\n", objectID)
	fmt.Fprintf(os.Stderr, "  Audiences: %s/svc-id/com.dynatrace.da\n", host)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Azure CLI commands:\n")
	fmt.Fprintf(os.Stderr, "1. Create Service Principal (if not created yet):\n")
	fmt.Fprintf(os.Stderr, "   az ad sp create-for-rbac --name %q --create-password false --query \"{CLIENT_ID:appId, TENANT_ID:tenant}\" --output table", connectionName)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "2. Create Federated Credential:\n")
	fmt.Fprintf(os.Stderr, "   az ad app federated-credential create --id \"<CLIENT_ID>\" --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", issuer, objectID, host)
	fmt.Fprintln(os.Stderr)
}

// printFederatedErrorSnippet prints az cli snippet for AADSTS70025 error to stderr
func printFederatedErrorSnippet(baseURL, objectID, clientID, issuerOverride string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	host := u.Host

	issuer := issuerOverride
	if issuer == "" {
		issuer = azureconnection.TokenIssuerForHost(host)
	}

	fmt.Fprintf(os.Stderr, "\nTo fix the Federated Identity error, run the following command:\n")
	// Use format validated by user: "{'key': 'value'}"
	fmt.Fprintf(os.Stderr, "az ad app federated-credential create --id %q --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", clientID, issuer, objectID, host)
	fmt.Fprintln(os.Stderr)
}
