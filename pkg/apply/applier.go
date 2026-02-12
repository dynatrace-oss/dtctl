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
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
)

// uuidRegex matches UUID-formatted strings (the Documents API rejects these for ID during creation)
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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
	ResourceWorkflow  ResourceType = "workflow"
	ResourceDashboard ResourceType = "dashboard"
	ResourceNotebook  ResourceType = "notebook"
	ResourceSLO       ResourceType = "slo"
	ResourceBucket    ResourceType = "bucket"
	ResourceSettings  ResourceType = "settings"
	ResourceAzureConnection      ResourceType = "azure_connection"
	ResourceAzureMonitoringConfig ResourceType = "azure_monitoring_config"
	ResourceUnknown   ResourceType = "unknown"
)

// Apply applies a resource configuration from file
func (a *Applier) Apply(fileData []byte, opts ApplyOptions) error {
	// Convert to JSON if needed
	jsonData, err := format.ValidateAndConvert(fileData)
	if err != nil {
		return fmt.Errorf("invalid file format: %w", err)
	}

	// Apply template rendering if variables provided
	if len(opts.TemplateVars) > 0 {
		rendered, err := template.RenderTemplate(string(jsonData), opts.TemplateVars)
		if err != nil {
			return fmt.Errorf("template rendering failed: %w", err)
		}
		jsonData = []byte(rendered)
	}

	// Detect resource type
	resourceType, err := detectResourceType(jsonData)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return a.dryRun(resourceType, jsonData)
	}

	// Apply based on resource type
	switch resourceType {
	case ResourceWorkflow:
		return a.applyWorkflow(jsonData)
	case ResourceDashboard:
		return a.applyDocument(jsonData, "dashboard", opts)
	case ResourceNotebook:
		return a.applyDocument(jsonData, "notebook", opts)
	case ResourceSLO:
		return a.applySLO(jsonData)
	case ResourceBucket:
		return a.applyBucket(jsonData)
	case ResourceSettings:
		return a.applySettings(jsonData)
	case ResourceAzureConnection:
		return a.applyAzureConnection(jsonData)
	case ResourceAzureMonitoringConfig:
		return a.applyAzureMonitoringConfig(jsonData)
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}
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

	// Azure Monitoring Config detection
	if scope, ok := raw["scope"].(string); ok && scope == "integration-azure" {
		return ResourceAzureMonitoringConfig, nil
	}

	// Check for explicit type field
	if typeField, ok := raw["type"].(string); ok {
		switch typeField {
		case "dashboard":
			return ResourceDashboard, nil
		case "notebook":
			return ResourceNotebook, nil
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
		if _, hasScope := raw["scope"]; hasScope {
			if _, hasValue := raw["value"]; hasValue {
				return ResourceSettings, nil
			}
		}
	}

	return ResourceUnknown, fmt.Errorf("could not detect resource type from file content")
}

// applyWorkflow applies a workflow resource
func (a *Applier) applyWorkflow(data []byte) error {
	// Parse to check for ID
	var wf map[string]interface{}
	if err := json.Unmarshal(data, &wf); err != nil {
		return fmt.Errorf("failed to parse workflow JSON: %w", err)
	}

	handler := workflow.NewHandler(a.client)

	id, hasID := wf["id"].(string)
	if !hasID || id == "" {
		// Create new workflow
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		result, err := handler.Create(data)
		if err != nil {
			return fmt.Errorf("failed to create workflow: %w", err)
		}
		fmt.Printf("Workflow %q (%s) created successfully\n", result.Title, result.ID)
		return nil
	}

	// Check if workflow exists
	existing, err := handler.Get(id)
	if err != nil {
		// Workflow doesn't exist, create it
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		result, err := handler.Create(data)
		if err != nil {
			return fmt.Errorf("failed to create workflow: %w", err)
		}
		fmt.Printf("Workflow %q (%s) created successfully\n", result.Title, result.ID)
		return nil
	}

	// Safety check for update operation - determine ownership from existing workflow
	ownership := a.determineOwnership(existing.Owner)
	if err := a.checkSafety(safety.OperationUpdate, ownership); err != nil {
		return err
	}

	// Update existing workflow
	result, err := handler.Update(id, data)
	if err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	fmt.Printf("Workflow %q (%s) updated successfully\n", result.Title, result.ID)
	return nil
}

// applyDocument applies a document resource (dashboard or notebook)
func (a *Applier) applyDocument(data []byte, docType string, opts ApplyOptions) error {
	// Parse to check for ID and name
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse %s JSON: %w", docType, err)
	}

	// Extract and validate content - handle round-trippable format from 'get' command
	contentData, name, description, warnings := extractDocumentContent(doc, docType)

	// Show validation warnings
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Count tiles/sections for feedback
	tileCount := countDocumentItems(contentData, docType)

	handler := document.NewHandler(a.client)

	id, hasID := doc["id"].(string)
	if !hasID || id == "" {
		// No ID provided - create new document
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
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
			return fmt.Errorf("failed to create %s: %w", docType, err)
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

		fmt.Printf("%s %q (%s) created successfully", capitalize(docType), resultName, resultID)
		if tileCount > 0 {
			fmt.Printf(" [%d %s]", tileCount, itemName(docType))
		}
		fmt.Println()
		if result.ID != "" {
			fmt.Printf("URL: %s\n", a.documentURL(docType, result.ID))
		}
		return nil
	}

	// Check if document exists
	metadata, err := handler.GetMetadata(id)
	if err != nil {
		// Document doesn't exist, create it
		// Safety check for create operation
		if err := a.checkSafety(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		if name == "" {
			name = fmt.Sprintf("Untitled %s", docType)
		}

		// The Documents API rejects UUID-formatted IDs during creation.
		// If the ID is a UUID (e.g., from an export), create without it and let the API generate a new ID.
		createID := id
		if isUUID(id) {
			createID = ""
			fmt.Fprintf(os.Stderr, "Note: Creating new %s (UUID IDs cannot be reused across tenants)\n", docType)
		}

		result, err := handler.Create(document.CreateRequest{
			ID:          createID,
			Name:        name,
			Type:        docType,
			Description: description,
			Content:     contentData,
		})
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", docType, err)
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

		fmt.Printf("%s %q (%s) created successfully", capitalize(docType), resultName, resultID)
		if tileCount > 0 {
			fmt.Printf(" [%d %s]", tileCount, itemName(docType))
		}
		fmt.Println()
		if result.ID != "" {
			fmt.Printf("URL: %s\n", a.documentURL(docType, result.ID))
		}
		return nil
	}

	// Safety check for update operation - determine ownership from metadata
	ownership := a.determineOwnership(metadata.Owner)
	if err := a.checkSafety(safety.OperationUpdate, ownership); err != nil {
		return err
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
		return fmt.Errorf("failed to apply %s: %w", docType, err)
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

	fmt.Printf("%s %q (%s) updated successfully", capitalize(docType), resultName, resultID)
	if tileCount > 0 {
		fmt.Printf(" [%d %s]", tileCount, itemName(docType))
	}
	fmt.Println()
	if resultID != "" {
		fmt.Printf("URL: %s\n", a.documentURL(docType, resultID))
	}
	return nil
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

	fmt.Printf("\n--- existing %s\n+++ new %s\n", resourceType, resourceType)

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
				fmt.Printf("- %s\n", oldLine)
			}
			if newLine != "" {
				fmt.Printf("+ %s\n", newLine)
			}
			changes++
		}
	}

	if changes == 0 {
		fmt.Println("(no changes)")
	}
	fmt.Println()
}

// documentURL returns the UI URL for a document
func (a *Applier) documentURL(docType, id string) string {
	// Convert base API URL to apps URL
	// e.g., https://abc12345.apps.dynatrace.com -> https://abc12345.apps.dynatrace.com/ui/document/<type>/<id>
	switch docType {
	case "dashboard":
		return fmt.Sprintf("%s/ui/document/v0/#/dashboards/%s", a.baseURL, id)
	case "notebook":
		return fmt.Sprintf("%s/ui/document/v0/#/notebooks/%s", a.baseURL, id)
	default:
		return fmt.Sprintf("%s/ui/document/v0/#/%ss/%s", a.baseURL, docType, id)
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
func (a *Applier) applySLO(data []byte) error {
	// Parse to check for ID
	var s map[string]interface{}
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to parse SLO JSON: %w", err)
	}

	handler := slo.NewHandler(a.client)

	id, hasID := s["id"].(string)
	if !hasID || id == "" {
		// Create new SLO
		result, err := handler.Create(data)
		if err != nil {
			return fmt.Errorf("failed to create SLO: %w", err)
		}
		fmt.Printf("SLO %q (%s) created successfully\n", result.Name, result.ID)
		return nil
	}

	// Check if SLO exists
	existing, err := handler.Get(id)
	if err != nil {
		// SLO doesn't exist, create it
		result, err := handler.Create(data)
		if err != nil {
			return fmt.Errorf("failed to create SLO: %w", err)
		}
		fmt.Printf("SLO %q (%s) created successfully\n", result.Name, result.ID)
		return nil
	}

	// Update existing SLO
	if err := handler.Update(id, existing.Version, data); err != nil {
		return fmt.Errorf("failed to update SLO: %w", err)
	}

	name, _ := s["name"].(string)
	fmt.Printf("SLO %q (%s) updated successfully\n", name, id)
	return nil
}

// applyBucket applies a bucket resource
func (a *Applier) applyBucket(data []byte) error {
	var b bucket.BucketCreate
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("failed to parse bucket JSON: %w", err)
	}

	handler := bucket.NewHandler(a.client)

	// Check if bucket exists
	existing, err := handler.Get(b.BucketName)
	if err != nil {
		// Bucket doesn't exist, create it
		result, err := handler.Create(b)
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		fmt.Printf("Bucket %q created (status: %s)\n", result.BucketName, result.Status)
		fmt.Println("Note: Bucket creation can take up to 1 minute to complete")
		return nil
	}

	// Update existing bucket
	update := bucket.BucketUpdate{
		DisplayName:   b.DisplayName,
		RetentionDays: b.RetentionDays,
	}

	if err := handler.Update(b.BucketName, existing.Version, update); err != nil {
		return fmt.Errorf("failed to update bucket: %w", err)
	}

	fmt.Printf("Bucket %q updated successfully\n", b.BucketName)
	return nil
}

// applySettings applies a settings object resource
func (a *Applier) applySettings(data []byte) error {
	var setting map[string]interface{}
	if err := json.Unmarshal(data, &setting); err != nil {
		return fmt.Errorf("failed to parse settings JSON: %w", err)
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
		return fmt.Errorf("settings object missing 'value' field or value is not an object")
	}

	// If no objectID, create new settings object
	if objectID == "" {
		if schemaID == "" {
			return fmt.Errorf("schemaId is required to create a settings object")
		}
		if scope == "" {
			return fmt.Errorf("scope is required to create a settings object")
		}

		req := settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		}

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create settings object: %w", err)
		}

		fmt.Printf("Settings object created successfully\n")
		fmt.Printf("  Schema: %s\n", schemaID)
		fmt.Printf("  Scope: %s\n", scope)
		fmt.Printf("  ObjectID: %s\n", result.ObjectID)
		return nil
	}

	// Check if settings object exists
	_, err := handler.GetWithContext(objectID, schemaID, scope)
	if err != nil {
		// Doesn't exist - try to create it
		if schemaID == "" {
			return fmt.Errorf("schemaId is required to create a settings object (objectId %q not found)", objectID)
		}
		if scope == "" {
			return fmt.Errorf("scope is required to create a settings object (objectId %q not found)", objectID)
		}

		req := settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		}

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create settings object: %w", err)
		}

		fmt.Printf("Settings object created successfully\n")
		fmt.Printf("  Schema: %s\n", schemaID)
		fmt.Printf("  Scope: %s\n", scope)
		fmt.Printf("  ObjectID: %s\n", result.ObjectID)
		return nil
	}

	// Update existing settings object
	updated, err := handler.UpdateWithContext(objectID, value, schemaID, scope)
	if err != nil {
		return fmt.Errorf("failed to update settings object: %w", err)
	}

	fmt.Printf("Settings object updated successfully\n")
	fmt.Printf("  Schema: %s\n", updated.SchemaID)
	fmt.Printf("  Scope: %s\n", updated.Scope)
	fmt.Printf("  ObjectID: %s\n", updated.ObjectID)
	if updated.Summary != "" {
		fmt.Printf("  Summary: %s\n", updated.Summary)
	}

	return nil
}

// applyAzureConnection applies Azure connection (credential)
func (a *Applier) applyAzureConnection(data []byte) error {
	// Azure connection input might be a single object or a list of setting objects
	var items []map[string]interface{}
	
	// Try parsing as array first
	err := json.Unmarshal(data, &items)
	if err != nil {
		// Not an array, try parsing as single object
		var item map[string]interface{}
		if errSingle := json.Unmarshal(data, &item); errSingle != nil {
			return fmt.Errorf("failed to parse Azure connection JSON: %w", errSingle)
		}
		items = []map[string]interface{}{item}
	}

	handler := settings.NewHandler(a.client)

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

		value, ok := item["value"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("Azure connection missing 'value' field")
		}

		if objectID == "" {
			// Create
			req := settings.SettingsObjectCreate{
				SchemaID: schemaID,
				Scope:    scope,
				Value:    value,
			}
			res, err := handler.Create(req)
			if err != nil {
				return fmt.Errorf("failed to create Azure connection: %w", err)
			}
			fmt.Printf("Azure connection created: %s\n", res.ObjectID)

			// Check for federated identity to print instructions
			if typeVal, ok := value["type"].(string); ok && typeVal == "federatedIdentityCredential" {
				printFederatedInstructions(a.baseURL, res.ObjectID)
			}
		} else {
			// Update
			_, err := handler.UpdateWithContext(objectID, value, schemaID, scope)
			if err != nil {
				// Check for Federated Identity error (AADSTS70025 or AADSTS700213)
				if strings.Contains(err.Error(), "AADSTS70025") || strings.Contains(err.Error(), "AADSTS700213") {
					if fedCred, ok := value["federatedIdentityCredential"].(map[string]interface{}); ok {
						if appID, ok := fedCred["applicationId"].(string); ok {
							printFederatedErrorSnippet(a.baseURL, objectID, appID)
						}
					}
				}
				return fmt.Errorf("failed to update Azure connection %s: %w", objectID, err)
			}
			fmt.Printf("Azure connection updated: %s\n", objectID)
		}
	}
	return nil
}

// applyAzureMonitoringConfig applies Azure monitoring configuration
func (a *Applier) applyAzureMonitoringConfig(data []byte) error {
	handler := azuremonitoringconfig.NewHandler(a.client)

	// Unmarshal to struct to handle casing properly via json tags
	var config azuremonitoringconfig.AzureMonitoringConfig
	if err := json.Unmarshal(data, &config); err != nil {
		// Fallback to map if struct unmarshal fails (e.g. slight schema mismatch we might want to pass through?)
		// But usually strict typing is better here to fix casing issues.
		// Let's try to unmarshal to map first to get ID, but for sending we need clean data.
		return fmt.Errorf("failed to parse Azure monitoring config JSON: %w", err)
	}
	
	objectID := config.ObjectID

	if objectID == "" {
		// Re-marshal to ensure json tags are respected (casing)
		cleanData, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal clean config: %w", err)
		}

		res, err := handler.Create(cleanData)
		if err != nil {
			return err
		}
		fmt.Printf("Azure monitoring config created: %s\n", res.ObjectID)
	} else {
		// Re-marshal to ensure json tags are respected (casing)
		cleanData, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal clean config: %w", err)
		}
		// Update
		res, err := handler.Update(objectID, cleanData)
		if err != nil {
			return err
		}
		fmt.Printf("Azure monitoring config updated: %s\n", res.ObjectID)
	}
	return nil
}

// capitalize capitalizes the first letter of a string
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// printFederatedInstructions prints configuration instructions for Federated Identity Credential
func printFederatedInstructions(baseURL, objectID string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		// Should not happen if client is initialized correctly, but fail gracefully
		fmt.Printf("Warning: Could not parse base URL for instructions: %v\n", err)
		return
	}
	host := u.Host

	// Determine issuer based on environment heuristic
	// Default to SaaS production
	issuer := "https://token.dynatrace.com"
	if strings.Contains(host, "dev.apps.dynatracelabs.com") || strings.Contains(host, "dev.dynatracelabs.com") {
		issuer = "https://dev.token.dynatracelabs.com"
	}

	fmt.Println("\nFurther configuration required in Azure Portal (Federated Credentials):")
	fmt.Printf("  Issuer:    %s\n", issuer)
	fmt.Printf("  Subject:   dt:connection-id/%s\n", objectID)
	fmt.Printf("  Audiences: %s/svc-id/com.dynatrace.da\n", host)
}

// printFederatedErrorSnippet prints az cli snippet for AADSTS70025 error
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

	fmt.Println("\nTo fix the Federated Identity error, run the following command:")
	// Use format validated by user: "{'key': 'value'}"
	fmt.Printf("az ad app federated-credential create --id %q --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", clientID, issuer, objectID, host)
	fmt.Println()
}

