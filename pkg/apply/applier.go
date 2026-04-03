package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/hook"
	"github.com/dynatrace-oss/dtctl/pkg/resources/anomalydetector"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
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
	preApplyHook  string // hook command (empty = no hook)
	sourceFile    string // original filename for hook context
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

// WithPreApplyHook sets the pre-apply hook command.
// The command is run via sh -c with the resource type and source file as
// positional parameters ($1 and $2), and the processed JSON on stdin.
func (a *Applier) WithPreApplyHook(command string) *Applier {
	a.preApplyHook = command
	return a
}

// WithSourceFile sets the original filename (passed to hook as context).
// This is the file path from "dtctl apply -f <file>" — informational only.
func (a *Applier) WithSourceFile(filename string) *Applier {
	a.sourceFile = filename
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
	NoHooks      bool // skip pre-apply hooks
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
	ResourceSegment               ResourceType = "segment"
	ResourceAnomalyDetector       ResourceType = "anomaly_detector"
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

	// Run pre-apply hook (if configured and not skipped)
	if !opts.NoHooks && a.preApplyHook != "" {
		result, err := hook.RunPreApply(
			context.Background(),
			a.preApplyHook,
			string(resourceType),
			a.sourceFile,
			jsonData,
		)
		if err != nil {
			return nil, err
		}
		if result.ExitCode != 0 {
			return nil, &HookRejectedError{
				Command:  a.preApplyHook,
				ExitCode: result.ExitCode,
				Stderr:   result.Stderr,
			}
		}
	}

	if opts.DryRun {
		result, err := a.dryRun(resourceType, jsonData)
		if err != nil {
			return nil, err
		}
		return []ApplyResult{result}, nil
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
	case ResourceSegment:
		result, err = a.applySegment(jsonData)
	case ResourceAnomalyDetector:
		result, err = a.applyAnomalyDetector(jsonData)
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
	// Anomaly Detector detection (raw Settings format)
	if schema, ok := raw["schemaId"].(string); ok && schema == anomalydetector.SchemaID {
		return ResourceAnomalyDetector, nil
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
		if schemaIDValue == anomalydetector.SchemaID {
			return ResourceAnomalyDetector, nil
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

	// Anomaly Detector detection (flattened format): "analyzer" with "name" subfield + "eventTemplate"
	if analyzerRaw, hasAnalyzer := raw["analyzer"]; hasAnalyzer {
		if _, hasEventTemplate := raw["eventTemplate"]; hasEventTemplate {
			if analyzerMap, ok := analyzerRaw.(map[string]interface{}); ok {
				if _, hasName := analyzerMap["name"]; hasName {
					return ResourceAnomalyDetector, nil
				}
			}
		}
	}

	// Filter segments: "includes" + "isPublic" is a positive, segment-specific marker.
	// We also check for "name" since it's required, and exclude known overlapping resources.
	if _, hasIncludes := raw["includes"]; hasIncludes {
		if _, hasIsPublic := raw["isPublic"]; hasIsPublic {
			return ResourceSegment, nil
		}
		// Fallback: "includes" + "name" without workflow/bucket/SLO markers
		if _, hasName := raw["name"]; hasName {
			_, hasTasks := raw["tasks"]
			_, hasBucketName := raw["bucketName"]
			_, hasCriteria := raw["criteria"]
			if !hasTasks && !hasBucketName && !hasCriteria {
				return ResourceSegment, nil
			}
		}
	}

	return ResourceUnknown, fmt.Errorf("could not detect resource type from file content")
}

// dryRun validates what would be applied without actually applying.
// Returns a DryRunResult with structured information about the planned operation.
func (a *Applier) dryRun(resourceType ResourceType, data []byte) (ApplyResult, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// For documents, check if it would be create or update
	if resourceType == ResourceDashboard || resourceType == ResourceNotebook {
		return a.dryRunDocument(resourceType, doc)
	}

	// Extension monitoring configs have specific fields
	if resourceType == ResourceExtensionConfig {
		return a.dryRunExtensionConfig(doc)
	}

	// For other resources, return basic info
	id, _ := doc["id"].(string)
	name, _ := doc["name"].(string)
	if name == "" {
		name, _ = doc["title"].(string)
	}

	action := ActionCreated // assume create unless we can prove otherwise
	if id != "" {
		action = ActionUpdated // has ID, likely an update (best guess without API call)
	}

	return &DryRunResult{
		ApplyResultBase: ApplyResultBase{
			Action:       action,
			ResourceType: string(resourceType),
			ID:           id,
			Name:         name,
		},
	}, nil
}

// capitalize capitalizes the first letter of a string
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
