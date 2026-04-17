package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

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

		// File had no id field before this apply — stamp it back or hint.
		if resultID != "(ID not returned)" {
			applyWriteBack(a.sourceFile, resultID, docType, opts.WriteID, false, &resultWarnings)
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

		// For the UUID case the API generated a fresh id — stamp it if requested.
		// For the non-UUID case the file already carries the id field, so neither
		// the write-back nor the hint is needed (applyWriteBack treats it as a no-op).
		fileAlreadyHasID := !isUUID(id) // non-UUID id was in the file and is preserved
		applyWriteBack(a.sourceFile, resultID, docType, opts.WriteID, fileAlreadyHasID, &resultWarnings)

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

// dryRunDocument performs dry-run validation for dashboard/notebook documents
func (a *Applier) dryRunDocument(resourceType ResourceType, doc map[string]interface{}) (ApplyResult, error) {
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
	action := ActionCreated
	var existingName string
	if id != "" {
		handler := document.NewHandler(a.client)
		metadata, err := handler.GetMetadata(id)
		if err == nil {
			action = ActionUpdated
			existingName = metadata.Name
		}
	}

	var url string
	if id != "" {
		url = a.documentURL(docType, id)
	}

	return &DryRunResult{
		ApplyResultBase: ApplyResultBase{
			Action:       action,
			ResourceType: docType,
			ID:           id,
			Name:         name,
			Warnings:     warnings,
		},
		URL:             url,
		ItemCount:       tileCount,
		ItemType:        itemName(docType),
		ExistingName:    existingName,
		ValidationWarns: warnings,
	}, nil
}
