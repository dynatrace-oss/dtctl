package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
)

// uuidRegex matches UUID-formatted strings (the Documents API rejects these for ID during creation)
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// stderrWarn writes a note to stderr and appends it to the warnings slice.
func stderrWarn(warnings *[]string, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Note: %s\n", msg)
	if warnings != nil {
		*warnings = append(*warnings, msg)
	}
}

// isUUID checks if a string is a UUID format
func isUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// Applier handles resource apply operations
type Applier struct {
	client        *client.Client
	baseURL       string
	safetyChecker *safety.Checker
	currentUserID string
}

// NewApplier creates a new applier
func NewApplier(c *client.Client) *Applier {
	currentUserID, _ := c.CurrentUserID() // Ignore error - will be empty string
	return &Applier{
		client:        c,
		baseURL:       c.BaseURL(),
		currentUserID: currentUserID,
	}
}

// WithSafetyChecker sets the safety checker for the applier
func (a *Applier) WithSafetyChecker(checker *safety.Checker) *Applier {
	a.safetyChecker = checker
	return a
}

// checkSafety performs a safety check if a checker is configured
func (a *Applier) checkSafety(op safety.Operation, ownership safety.ResourceOwnership) error {
	if a.safetyChecker == nil {
		return nil // No checker configured, allow operation
	}
	return a.safetyChecker.CheckError(op, ownership)
}

// determineOwnership determines resource ownership given an owner ID
func (a *Applier) determineOwnership(resourceOwnerID string) safety.ResourceOwnership {
	return safety.DetermineOwnership(resourceOwnerID, a.currentUserID)
}

// ApplyOptions holds options for apply operation
type ApplyOptions struct {
	TemplateVars map[string]interface{}
	DryRun       bool
	Force        bool
	ShowDiff     bool
}

// ResourceType represents the type of resource
type ResourceType string

const (
	ResourceWorkflow              ResourceType = "workflow"
	ResourceDashboard             ResourceType = "dashboard"
	ResourceNotebook              ResourceType = "notebook"
	ResourceSLO                   ResourceType = "slo"
	ResourceBucket                ResourceType = "bucket"
	ResourceSettings              ResourceType = "settings"
	ResourceAzureConnection       ResourceType = "azure_connection"
	ResourceAzureMonitoringConfig ResourceType = "azure_monitoring_config"
	ResourceGCPConnection         ResourceType = "gcp_connection"
	ResourceGCPMonitoringConfig   ResourceType = "gcp_monitoring_config"
	ResourceExtensionConfig       ResourceType = "extension_config"
	ResourceUnknown               ResourceType = "unknown"
)

// Apply applies a resource configuration from file.
// Returns a slice of results (most resource types return a single-element slice;
// connection resources may return multiple results when applying a list).
func (a *Applier) Apply(fileData []byte, opts ApplyOptions) ([]ApplyResult, error) {
	// Convert to JSON if needed
	jsonData, err := format.ValidateAndConvert(fileData)
	if err != nil {
		return nil, fmt.Errorf("invalid file format: %w", err)
	}

	// Apply template rendering if variables provided
	if len(opts.TemplateVars) > 0 {
		rendered, err := template.RenderTemplate(string(jsonData), opts.TemplateVars)
		if err != nil {
			return nil, fmt.Errorf("template rendering failed: %w", err)
		}
		jsonData = []byte(rendered)
	}

	// Detect resource type
	resourceType, err := detectResourceType(jsonData)
	if err != nil {
		return nil, err
	}

	if opts.DryRun {
		return nil, a.dryRun(resourceType, jsonData)
	}

	// Connection resources can return multiple results
	switch resourceType {
	case ResourceAzureConnection:
		return a.applyAzureConnection(jsonData)
	case ResourceGCPConnection:
		return a.applyGCPConnection(jsonData)
	default:
		// All other resource types return a single result
	}

	// Apply single-result resource types
	var result ApplyResult
	switch resourceType {
	case ResourceWorkflow:
		result, err = a.applyWorkflow(jsonData)
	case ResourceDashboard:
		result, err = a.applyDocument(jsonData, "dashboard", opts)
	case ResourceNotebook:
		result, err = a.applyDocument(jsonData, "notebook", opts)
	case ResourceSLO:
		result, err = a.applySLO(jsonData)
	case ResourceBucket:
		result, err = a.applyBucket(jsonData)
	case ResourceSettings:
		result, err = a.applySettings(jsonData)
	case ResourceAzureMonitoringConfig:
		result, err = a.applyAzureMonitoringConfig(jsonData)
	case ResourceGCPMonitoringConfig:
		result, err = a.applyGCPMonitoringConfig(jsonData)
	case ResourceExtensionConfig:
		result, err = a.applyExtensionConfig(jsonData)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
	if err != nil {
		return nil, err
	}
	return []ApplyResult{result}, nil
}

// detectResourceType determines the resource type from JSON data
func detectResourceType(data []byte) (ResourceType, error) {
	// Check for array (Azure Connection list)
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		var rawList []map[string]interface{}
		if err := json.Unmarshal(data, &rawList); err == nil && len(rawList) > 0 {
			if schema, ok := rawList[0]["schemaId"].(string); ok && schema == azureconnection.SchemaID {
				return ResourceAzureConnection, nil
			}
			if schema, ok := rawList[0]["schemaId"].(string); ok && schema == gcpconnection.SchemaID {
				return ResourceGCPConnection, nil
			}
		}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ResourceUnknown, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Azure Connection detection (single object)
	if schema, ok := raw["schemaId"].(string); ok && schema == azureconnection.SchemaID {
		return ResourceAzureConnection, nil
	}
	if schema, ok := raw["schemaId"].(string); ok && schema == gcpconnection.SchemaID {
		return ResourceGCPConnection, nil
	}

	// Azure Monitoring Config detection
	if scope, ok := raw["scope"].(string); ok && scope == "integration-azure" {
		return ResourceAzureMonitoringConfig, nil
	}

	// GCP Monitoring Config detection
	if scope, ok := raw["scope"].(string); ok && scope == "integration-gcp" {
		return ResourceGCPMonitoringConfig, nil
	}

	// Check for explicit type field
	if typeField, ok := raw["type"].(string); ok {
		switch typeField {
		case "dashboard":
			return ResourceDashboard, nil
		case "notebook":
			return ResourceNotebook, nil
		case "extension_monitoring_config":
			return ResourceExtensionConfig, nil
		}
	}

	// Heuristic detection based on field presence
	// Workflows have "tasks" and "trigger" fields
	if _, hasTasks := raw["tasks"]; hasTasks {
		if _, hasTrigger := raw["trigger"]; hasTrigger {
			return ResourceWorkflow, nil
		}
	}

	// Documents have "metadata" or "content" at root level
	if _, hasMetadata := raw["metadata"]; hasMetadata {
		// Further distinguish between dashboard and notebook
		if typeField, ok := raw["type"].(string); ok {
			if typeField == "dashboard" {
				return ResourceDashboard, nil
			}
			if typeField == "notebook" {
				return ResourceNotebook, nil
			}
		}
		return ResourceDashboard, nil // Default to dashboard for documents
	}

	// Check for direct content format (tiles for dashboard, sections for notebook)
	if _, hasTiles := raw["tiles"]; hasTiles {
		return ResourceDashboard, nil
	}
	if _, hasSections := raw["sections"]; hasSections {
		return ResourceNotebook, nil
	}

	// Also check for "content" field which contains the actual document
	if content, hasContent := raw["content"]; hasContent {
		if contentMap, ok := content.(map[string]interface{}); ok {
			if _, hasTiles := contentMap["tiles"]; hasTiles {
				return ResourceDashboard, nil
			}
			if _, hasSections := contentMap["sections"]; hasSections {
				return ResourceNotebook, nil
			}
		}
	}

	// SLOs have "criteria" and "name" fields (and optionally customSli or sliReference)
	if _, hasCriteria := raw["criteria"]; hasCriteria {
		if _, hasName := raw["name"]; hasName {
			// Check for SLO-specific fields
			if _, hasCustomSli := raw["customSli"]; hasCustomSli {
				return ResourceSLO, nil
			}
			if _, hasSliRef := raw["sliReference"]; hasSliRef {
				return ResourceSLO, nil
			}
			// If it has criteria and name but no tasks/trigger, it's likely an SLO
			if _, hasTasks := raw["tasks"]; !hasTasks {
				return ResourceSLO, nil
			}
		}
	}

	// Buckets have "bucketName" and "table" fields
	if _, hasBucketName := raw["bucketName"]; hasBucketName {
		if _, hasTable := raw["table"]; hasTable {
			return ResourceBucket, nil
		}
	}

	// Settings objects have "schemaId"/"schemaid", "scope", and "value" fields
	// Check both camelCase (API format) and lowercase (YAML format)
	var schemaIDValue string
	hasSchemaID := false
	if v, ok := raw["schemaId"].(string); ok {
		hasSchemaID = true
		schemaIDValue = v
	} else if v, ok := raw["schemaid"].(string); ok {
		hasSchemaID = true
		schemaIDValue = v
	}

	if hasSchemaID {
		if schemaIDValue == azureconnection.SchemaID {
			// This is a single Azure Connection (credential), not a list
			return ResourceAzureConnection, nil
		}
		if schemaIDValue == gcpconnection.SchemaID {
			return ResourceGCPConnection, nil
		}
		if _, hasScope := raw["scope"]; hasScope {
			if _, hasValue := raw["value"]; hasValue {
				if scope, ok := raw["scope"].(string); ok && scope == "integration-gcp" {
					return ResourceGCPMonitoringConfig, nil
				}
				if scope, ok := raw["scope"].(string); ok && scope == "integration-azure" {
					return ResourceAzureMonitoringConfig, nil
				}
				return ResourceSettings, nil
			}
		}
	}

	return ResourceUnknown, fmt.Errorf("could not detect resource type from file content")
}

// applyWorkflow applies a workflow resource
func (a *Applier) applyWorkflow(data []byte) (ApplyResult, error) {
	// Parse to check for ID
	var wf map[string]interface{}
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow JSON: %w", err)
	}

	handler := workflow.NewHandler(a.client)

	id, hasID := wf["id"].(string)
	if !hasID || id == "" {
		// Create new workflow
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		result, err := handler.Create(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create workflow: %w", err)
		}
		return &WorkflowApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "workflow",
				ID:           result.ID,
				Name:         result.Title,
			},
		}, nil
	}

	// Check if workflow exists
	existing, err := handler.Get(id)
	if err != nil {
		// Workflow doesn't exist, create it
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		result, err := handler.Create(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create workflow: %w", err)
		}
		return &WorkflowApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "workflow",
				ID:           result.ID,
				Name:         result.Title,
			},
		}, nil
	}

	// Safety check for update operation - determine ownership from existing workflow
	ownership := a.determineOwnership(existing.Owner)
	if err := a.checkSafety(safety.OperationUpdate, ownership); err != nil {
		return nil, err
	}

	// Update existing workflow
	result, err := handler.Update(id, data)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	return &WorkflowApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "workflow",
			ID:           result.ID,
			Name:         result.Title,
		},
	}, nil
}

// applyDocument applies a document resource (dashboard or notebook)
func (a *Applier) applyDocument(data []byte, docType string, opts ApplyOptions) (ApplyResult, error) {
	// Parse to check for ID and name
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s JSON: %w", docType, err)
	}

	// Extract and validate content - handle round-trippable format from 'get' command
	contentData, name, description, validationWarnings := extractDocumentContent(doc, docType)

	// Show validation warnings on stderr and collect for result
	var resultWarnings []string
	for _, w := range validationWarnings {
		stderrWarn(&resultWarnings, "%s", w)
	}

	// Count tiles/sections for feedback
	tileCount := countDocumentItems(contentData, docType)

	handler := document.NewHandler(a.client)

	id, hasID := doc["id"].(string)
	if !hasID || id == "" {
		// No ID provided - create new document
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		if name == "" {
			name = fmt.Sprintf("Untitled %s", docType)
		}

		result, err := handler.Create(document.CreateRequest{
			Name:        name,
			Type:        docType,
			Description: description,
			Content:     contentData,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", docType, err)
		}

		// Use name from input if result doesn't have it
		resultName := result.Name
		if resultName == "" {
			resultName = name
		}
		resultID := result.ID
		if resultID == "" {
			resultID = "(ID not returned)"
		}

		return a.buildDocumentResult(ActionCreated, docType, resultID, resultName, tileCount, resultWarnings), nil
	}

	// Check if document exists
	metadata, err := handler.GetMetadata(id)
	if err != nil {
		// Document doesn't exist, create it
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		if name == "" {
			name = fmt.Sprintf("Untitled %s", docType)
		}

		// The Documents API rejects UUID-formatted IDs during creation.
		// If the ID is a UUID (e.g., from an export), create without it and let the API generate a new ID.
		createID := id
		if isUUID(id) {
			createID = ""
			stderrWarn(&resultWarnings, "Creating new %s (UUID IDs cannot be reused across tenants)", docType)
		}

		result, err := handler.Create(document.CreateRequest{
			ID:          createID,
			Name:        name,
			Type:        docType,
			Description: description,
			Content:     contentData,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", docType, err)
		}

		// Use name from input if result doesn't have it
		resultName := result.Name
		if resultName == "" {
			resultName = name
		}
		resultID := result.ID
		if resultID == "" {
			resultID = id
		}

		return a.buildDocumentResult(ActionCreated, docType, resultID, resultName, tileCount, resultWarnings), nil
	}

	// Safety check for update operation - determine ownership from metadata
	ownership := a.determineOwnership(metadata.Owner)
	if err := a.checkSafety(safety.OperationUpdate, ownership); err != nil {
		return nil, err
	}

	// Show diff if requested
	if opts.ShowDiff {
		existingDoc, err := handler.Get(id)
		if err == nil && len(existingDoc.Content) > 0 {
			showJSONDiff(existingDoc.Content, contentData, docType)
		}
	}

	// Update the existing document (including metadata if name or description provided)
	result, err := handler.UpdateWithMetadata(id, metadata.Version, contentData, "application/json", name, description)
	if err != nil {
		return nil, fmt.Errorf("failed to apply %s: %w", docType, err)
	}

	// Use name from input/metadata if result doesn't have it
	resultName := result.Name
	if resultName == "" {
		resultName = name
	}
	if resultName == "" {
		resultName = metadata.Name
	}
	resultID := result.ID
	if resultID == "" {
		resultID = id
	}

	return a.buildDocumentResult(ActionUpdated, docType, resultID, resultName, tileCount, resultWarnings), nil
}

// buildDocumentResult constructs the appropriate document result type based on docType
func (a *Applier) buildDocumentResult(action, docType, id, name string, itemCount int, warnings []string) ApplyResult {
	if docType == "notebook" {
		return &NotebookApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       action,
				ResourceType: "notebook",
				ID:           id,
				Name:         name,
				Warnings:     warnings,
			},
			URL:          a.documentURL(docType, id),
			SectionCount: itemCount,
		}
	}
	return &DashboardApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       action,
			ResourceType: "dashboard",
			ID:           id,
			Name:         name,
			Warnings:     warnings,
		},
		URL:       a.documentURL(docType, id),
		TileCount: itemCount,
	}
}

// extractDocumentContent extracts the content from a document, handling various input formats
// Returns: contentData, name, description, warnings
func extractDocumentContent(doc map[string]interface{}, docType string) ([]byte, string, string, []string) {
	var warnings []string
	name, _ := doc["name"].(string)
	description, _ := doc["description"].(string)

	// Check if this is a "get" output format with nested content
	if content, hasContent := doc["content"]; hasContent {
		contentMap, isMap := content.(map[string]interface{})
		if isMap {
			// Check for double-nested content (common mistake)
			if innerContent, hasInner := contentMap["content"]; hasInner {
				warnings = append(warnings, "detected double-nested content (.content.content) - using inner content")
				contentMap = innerContent.(map[string]interface{})
			}

			// Validate structure based on document type
			if docType == "dashboard" {
				if _, hasTiles := contentMap["tiles"]; !hasTiles {
					warnings = append(warnings, "dashboard content has no 'tiles' field - dashboard may be empty")
				}
				if _, hasVersion := contentMap["version"]; !hasVersion {
					warnings = append(warnings, "dashboard content has no 'version' field")
				}
			} else if docType == "notebook" {
				if _, hasSections := contentMap["sections"]; !hasSections {
					warnings = append(warnings, "notebook content has no 'sections' field - notebook may be empty")
				}
			}

			contentData, _ := json.Marshal(contentMap)
			return contentData, name, description, warnings
		}
	}

	// No content field - the whole doc might be the content (direct format)
	// Check if it looks like dashboard/notebook content
	if docType == "dashboard" {
		if _, hasTiles := doc["tiles"]; hasTiles {
			// This is direct content format
			contentData, _ := json.Marshal(doc)
			return contentData, name, description, warnings
		}
		warnings = append(warnings, "document has no 'content' or 'tiles' field - structure may be incorrect")
	} else if docType == "notebook" {
		if _, hasSections := doc["sections"]; hasSections {
			// This is direct content format
			contentData, _ := json.Marshal(doc)
			return contentData, name, description, warnings
		}
		warnings = append(warnings, "document has no 'content' or 'sections' field - structure may be incorrect")
	}

	// Fall back to using the whole document as content
	contentData, _ := json.Marshal(doc)
	return contentData, name, description, warnings
}

// countDocumentItems counts tiles (for dashboards) or sections (for notebooks)
func countDocumentItems(contentData []byte, docType string) int {
	var content map[string]interface{}
	if err := json.Unmarshal(contentData, &content); err != nil {
		return 0
	}

	if docType == "dashboard" {
		if tiles, ok := content["tiles"].([]interface{}); ok {
			return len(tiles)
		}
	} else if docType == "notebook" {
		if sections, ok := content["sections"].([]interface{}); ok {
			return len(sections)
		}
	}
	return 0
}

// itemName returns the item name for a document type (tiles for dashboards, sections for notebooks)
func itemName(docType string) string {
	if docType == "dashboard" {
		return "tiles"
	}
	return "sections"
}

// showJSONDiff displays a simple diff between two JSON documents
func showJSONDiff(oldData, newData []byte, resourceType string) {
	// Pretty-print both for comparison
	var oldPretty, newPretty bytes.Buffer
	if err := json.Indent(&oldPretty, oldData, "", "  "); err != nil {
		return
	}
	if err := json.Indent(&newPretty, newData, "", "  "); err != nil {
		return
	}

	oldLines := strings.Split(oldPretty.String(), "\n")
	newLines := strings.Split(newPretty.String(), "\n")

	fmt.Fprintf(os.Stderr, "\n--- existing %s\n+++ new %s\n", resourceType, resourceType)

	// Simple line-by-line diff
	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	changes := 0
	for i := 0; i < maxLines; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if oldLine != "" {
				fmt.Fprintf(os.Stderr, "- %s\n", oldLine)
			}
			if newLine != "" {
				fmt.Fprintf(os.Stderr, "+ %s\n", newLine)
			}
			changes++
		}
	}

	if changes == 0 {
		fmt.Fprintln(os.Stderr, "(no changes)")
	}
	fmt.Fprintln(os.Stderr)
}

// documentURL returns the UI URL for a document
func (a *Applier) documentURL(docType, id string) string {
	// Build the app-based URL for the document
	// e.g., https://abc12345.apps.dynatrace.com -> https://abc12345.apps.dynatrace.com/ui/apps/dynatrace.dashboards/dashboard/<id>
	switch docType {
	case "dashboard":
		return fmt.Sprintf("%s/ui/apps/dynatrace.dashboards/dashboard/%s", a.baseURL, id)
	case "notebook":
		return fmt.Sprintf("%s/ui/apps/dynatrace.notebooks/notebook/%s", a.baseURL, id)
	default:
		return fmt.Sprintf("%s/ui/apps/dynatrace.%ss/%s/%s", a.baseURL, docType, docType, id)
	}
}

// dryRun shows what would be applied without actually applying
func (a *Applier) dryRun(resourceType ResourceType, data []byte) error {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// For documents, check if it would be create or update
	if resourceType == ResourceDashboard || resourceType == ResourceNotebook {
		return a.dryRunDocument(resourceType, doc, data)
	}

	// Extension monitoring configs have specific fields
	if resourceType == ResourceExtensionConfig {
		return a.dryRunExtensionConfig(doc)
	}

	// For other resources, show basic info
	fmt.Printf("Dry run: would apply %s resource\n", resourceType)

	id, _ := doc["id"].(string)
	name, _ := doc["name"].(string)
	if name == "" {
		name, _ = doc["title"].(string)
	}

	if id != "" {
		fmt.Printf("  ID: %s\n", id)
	}
	if name != "" {
		fmt.Printf("  Name: %s\n", name)
	}

	fmt.Println("\nResource content validated successfully")
	return nil
}

// dryRunExtensionConfig performs dry-run validation for extension monitoring configs
func (a *Applier) dryRunExtensionConfig(doc map[string]interface{}) error {
	extensionName, _ := doc["extensionName"].(string)
	objectID, _ := doc["objectId"].(string)
	scope, _ := doc["scope"].(string)

	if objectID != "" {
		fmt.Println("Dry run: would update extension monitoring configuration")
		fmt.Printf("  Config ID: %s\n", objectID)
	} else {
		fmt.Println("Dry run: would create extension monitoring configuration")
	}
	if extensionName != "" {
		fmt.Printf("  Extension: %s\n", extensionName)
	}
	if scope != "" {
		fmt.Printf("  Scope:     %s\n", scope)
	}

	fmt.Println("\nResource content validated successfully")
	return nil
}

// dryRunDocument performs dry-run validation for dashboard/notebook documents
func (a *Applier) dryRunDocument(resourceType ResourceType, doc map[string]interface{}, data []byte) error {
	docType := string(resourceType)
	id, _ := doc["id"].(string)

	// Use the same extraction/validation logic as apply
	contentData, name, _, warnings := extractDocumentContent(doc, docType)
	if name == "" {
		name = fmt.Sprintf("Untitled %s", docType)
	}

	// Count tiles/sections
	tileCount := countDocumentItems(contentData, docType)

	// Check if document exists to determine create vs update
	action := "create"
	var existingName string
	if id != "" {
		handler := document.NewHandler(a.client)
		metadata, err := handler.GetMetadata(id)
		if err == nil {
			action = "update"
			existingName = metadata.Name
		}
	}

	fmt.Printf("Dry run: would %s %s\n", action, docType)
	fmt.Printf("  Name: %s\n", name)
	if id != "" {
		fmt.Printf("  ID: %s\n", id)
	}
	if action == "update" && existingName != "" && existingName != name {
		fmt.Printf("  (existing name: %s)\n", existingName)
	}
	if tileCount > 0 {
		fmt.Printf("  %s: %d\n", capitalize(itemName(docType)), tileCount)
	}

	// Show validation warnings
	if len(warnings) > 0 {
		fmt.Println("\nValidation warnings:")
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	} else {
		fmt.Println("\nDocument structure validated successfully")
	}

	if id != "" {
		fmt.Printf("URL (after %s): %s\n", action, a.documentURL(docType, id))
	}

	return nil
}

// applySLO applies an SLO resource
func (a *Applier) applySLO(data []byte) (ApplyResult, error) {
	// Parse to check for ID
	var s map[string]interface{}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse SLO JSON: %w", err)
	}

	handler := slo.NewHandler(a.client)

	id, hasID := s["id"].(string)
	if !hasID || id == "" {
		// Create new SLO
		result, err := handler.Create(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create SLO: %w", err)
		}
		return &SLOApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "slo",
				ID:           result.ID,
				Name:         result.Name,
			},
		}, nil
	}

	// Check if SLO exists
	existing, err := handler.Get(id)
	if err != nil {
		// SLO doesn't exist, create it
		result, err := handler.Create(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create SLO: %w", err)
		}
		return &SLOApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "slo",
				ID:           result.ID,
				Name:         result.Name,
			},
		}, nil
	}

	// Update existing SLO
	if err := handler.Update(id, existing.Version, data); err != nil {
		return nil, fmt.Errorf("failed to update SLO: %w", err)
	}

	name, _ := s["name"].(string)
	return &SLOApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "slo",
			ID:           id,
			Name:         name,
		},
	}, nil
}

// applyBucket applies a bucket resource
func (a *Applier) applyBucket(data []byte) (ApplyResult, error) {
	var b bucket.BucketCreate
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("failed to parse bucket JSON: %w", err)
	}

	handler := bucket.NewHandler(a.client)

	// Check if bucket exists
	existing, err := handler.Get(b.BucketName)
	if err != nil {
		// Bucket doesn't exist, create it
		result, err := handler.Create(b)
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		var warnings []string
		stderrWarn(&warnings, "Bucket creation can take up to 1 minute to complete")
		return &BucketApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "bucket",
				ID:           result.BucketName,
				Name:         result.BucketName,
				Warnings:     warnings,
			},
			Status: result.Status,
		}, nil
	}

	// Update existing bucket
	update := bucket.BucketUpdate{
		DisplayName:   b.DisplayName,
		RetentionDays: b.RetentionDays,
	}

	if err := handler.Update(b.BucketName, existing.Version, update); err != nil {
		return nil, fmt.Errorf("failed to update bucket: %w", err)
	}

	return &BucketApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "bucket",
			ID:           b.BucketName,
			Name:         b.BucketName,
		},
		Status: existing.Status,
	}, nil
}

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
	_, err := handler.GetWithContext(objectID, schemaID, scope)
	if err != nil {
		// Doesn't exist - try to create it
		if schemaID == "" {
			return nil, fmt.Errorf("schemaId is required to create a settings object (objectId %q not found)", objectID)
		}
		if scope == "" {
			return nil, fmt.Errorf("scope is required to create a settings object (objectId %q not found)", objectID)
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
	updated, err := handler.UpdateWithContext(objectID, value, schemaID, scope)
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
				printFederatedInstructions(a.baseURL, res.ObjectID, &resultWarnings)
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
							printFederatedCompleteInstructions(a.baseURL, objectID, value.Name)
							return nil, fmt.Errorf("azure connection requires additional configuration: %w", err)
						}
					}
				}

				// Check for Federated Identity error (AADSTS70025 or AADSTS700213)
				if strings.Contains(errMsg, "AADSTS70025") || strings.Contains(errMsg, "AADSTS700213") {
					if value.FederatedIdentityCredential != nil && value.FederatedIdentityCredential.ApplicationID != "" {
						printFederatedErrorSnippet(a.baseURL, objectID, value.FederatedIdentityCredential.ApplicationID)
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
			ResourceType: "gcp_monitoring_config",
			ID:           res.ObjectID,
			Name:         config.Value.Description,
			Warnings:     warnings,
		},
		Scope: config.Scope,
	}, nil
}

// applyExtensionConfig applies an extension monitoring configuration.
// Detects create vs update by checking for an objectId field in the payload.
func (a *Applier) applyExtensionConfig(data []byte) (ApplyResult, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse extension config JSON: %w", err)
	}

	extensionName, _ := raw["extensionName"].(string)
	if extensionName == "" {
		return nil, fmt.Errorf("extensionName is required in extension config payload")
	}

	objectID, _ := raw["objectId"].(string)

	// Build the create/update body (scope + value only)
	var config extension.MonitoringConfigurationCreate
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse extension config body: %w", err)
	}

	handler := extension.NewHandler(a.client)

	if objectID == "" {
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return nil, err
		}

		result, err := handler.CreateMonitoringConfiguration(extensionName, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create extension monitoring configuration: %w", err)
		}

		return &ExtensionConfigApplyResult{
			ApplyResultBase: ApplyResultBase{
				Action:       ActionCreated,
				ResourceType: "extension_config",
				ID:           result.ObjectID,
			},
			ExtensionName: extensionName,
			Scope:         result.Scope,
		}, nil
	}

	if err := a.checkSafety(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
		return nil, err
	}

	result, err := handler.UpdateMonitoringConfiguration(extensionName, objectID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to update extension monitoring configuration: %w", err)
	}

	return &ExtensionConfigApplyResult{
		ApplyResultBase: ApplyResultBase{
			Action:       ActionUpdated,
			ResourceType: "extension_config",
			ID:           result.ObjectID,
		},
		ExtensionName: extensionName,
		Scope:         result.Scope,
	}, nil
}

// capitalize capitalizes the first letter of a string
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// printFederatedInstructions prints configuration instructions for Federated Identity Credential to stderr
func printFederatedInstructions(baseURL, objectID string, warnings *[]string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		// Should not happen if client is initialized correctly, but fail gracefully
		fmt.Fprintf(os.Stderr, "Warning: Could not parse base URL for instructions: %v\n", err)
		return
	}
	host := u.Host

	// Determine issuer based on environment heuristic
	// Default to SaaS production
	issuer := "https://token.dynatrace.com"
	if strings.Contains(host, "dev.apps.dynatracelabs.com") || strings.Contains(host, "dev.dynatracelabs.com") {
		issuer = "https://dev.token.dynatracelabs.com"
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
func printFederatedCompleteInstructions(baseURL, objectID, connectionName string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not parse base URL for instructions: %v\n", err)
		return
	}
	host := u.Host

	// Determine issuer
	issuer := "https://token.dynatrace.com"
	if strings.Contains(host, "dev.apps.dynatracelabs.com") || strings.Contains(host, "dev.dynatracelabs.com") {
		issuer = "https://dev.token.dynatracelabs.com"
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
func printFederatedErrorSnippet(baseURL, objectID, clientID string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	host := u.Host

	// Determine issuer
	issuer := "https://token.dynatrace.com"
	if strings.Contains(host, "dev.apps.dynatracelabs.com") || strings.Contains(host, "dev.dynatracelabs.com") {
		issuer = "https://dev.token.dynatracelabs.com"
	}

	fmt.Fprintf(os.Stderr, "\nTo fix the Federated Identity error, run the following command:\n")
	// Use format validated by user: "{'key': 'value'}"
	fmt.Fprintf(os.Stderr, "az ad app federated-credential create --id %q --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", clientID, issuer, objectID, host)
	fmt.Fprintln(os.Stderr)
}
