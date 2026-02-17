package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// createNotebookCmd creates a notebook from a file
var createNotebookCmd = &cobra.Command{
	Use:   "notebook -f <file>",
	Short: "Create a notebook from a file",
	Long: `Create a new notebook from a YAML or JSON file.

Examples:
  # Create a notebook from YAML
  dtctl create notebook -f notebook.yaml

  # Create with a specific name
  dtctl create notebook -f notebook.yaml --name "My Notebook"

  # Create with template variables
  dtctl create notebook -f notebook.yaml --set env=prod

  # Dry run to preview
  dtctl create notebook -f notebook.yaml --dry-run
`,
	Aliases: []string{"nb"},
	RunE:    createDocumentRunE("notebook"),
}

// createDashboardCmd creates a dashboard from a file
var createDashboardCmd = &cobra.Command{
	Use:   "dashboard -f <file>",
	Short: "Create a dashboard from a file",
	Long: `Create a new dashboard from a YAML or JSON file.

IMPORTANT: This command always creates a NEW dashboard, even if your file contains
an 'id' field. To update an existing dashboard, use 'dtctl apply' instead.

Workflow:
  - Create: Use this command to create new dashboards
  - Update: Use 'dtctl apply -f dashboard.yaml' to update existing dashboards
  - Delete: Use 'dtctl delete dashboard <id> -y' to delete

The create command will:
  1. Extract dashboard name and description from the file
  2. Create a new dashboard with a unique ID
  3. Return the new dashboard ID and URL

Examples:
  # Create a dashboard from YAML
  dtctl create dashboard -f dashboard.yaml

  # Create with a specific name (overrides name in file)
  dtctl create dashboard -f dashboard.yaml --name "My Dashboard"

  # Create with template variables
  dtctl create dashboard -f dashboard.yaml --set env=prod

  # Dry run to preview without creating
  dtctl create dashboard -f dashboard.yaml --dry-run

  # Provide a custom ID (useful for predictable IDs)
  dtctl create dashboard -f dashboard.yaml --id my.custom.dashboard-id

See also:
  dtctl apply --help    # For updating existing dashboards
  dtctl get dashboard --help    # For exporting dashboards
`,
	Aliases: []string{"db"},
	RunE:    createDocumentRunE("dashboard"),
}

// createDocumentRunE returns a RunE function for creating documents of a specific type
func createDocumentRunE(docType string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		id, _ := cmd.Flags().GetString("id")
		setFlags, _ := cmd.Flags().GetStringArray("set")

		// Read the file
		fileData, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Convert to JSON if needed
		jsonData, err := format.ValidateAndConvert(fileData)
		if err != nil {
			return fmt.Errorf("invalid file format: %w", err)
		}

		// Apply template rendering if variables provided
		if len(setFlags) > 0 {
			templateVars, err := template.ParseSetFlags(setFlags)
			if err != nil {
				return fmt.Errorf("invalid --set flag: %w", err)
			}
			rendered, err := template.RenderTemplate(string(jsonData), templateVars)
			if err != nil {
				return fmt.Errorf("template rendering failed: %w", err)
			}
			jsonData = []byte(rendered)
		}

		// Parse the document to extract content properly
		var doc map[string]interface{}
		if err := json.Unmarshal(jsonData, &doc); err != nil {
			return fmt.Errorf("failed to parse %s JSON: %w", docType, err)
		}

		// Extract content, name, description using the same logic as apply
		contentData, extractedName, extractedDesc, warnings := extractDocumentContent(doc, docType)

		// Show validation warnings
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
		}

		// Use flag values if provided, otherwise use extracted values
		if name == "" {
			name = extractedName
		}
		if description == "" {
			description = extractedDesc
		}

		// Extract ID from document if not provided via flag
		if id == "" {
			if docID, ok := doc["id"].(string); ok && docID != "" {
				id = docID
			}
		}

		// Default name if still empty
		if name == "" {
			name = fmt.Sprintf("Untitled %s", docType)
		}

		// Count tiles/sections for feedback
		tileCount := countDocumentItems(contentData, docType)

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create %s\n", docType)
			fmt.Printf("  Name: %s\n", name)
			if id != "" {
				fmt.Printf("  ID: %s\n", id)
			}
			if description != "" {
				fmt.Printf("  Description: %s\n", description)
			}
			if tileCount > 0 {
				fmt.Printf("  %s: %d\n", capitalize(itemName(docType)), tileCount)
			}
			if len(warnings) == 0 {
				fmt.Println("\nDocument structure validated successfully")
			}
			return nil
		}

		// Load configuration
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		result, err := handler.Create(document.CreateRequest{
			ID:          id,
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
			if resultID == "" {
				resultID = "(ID not returned)"
			}
		}

		// Improved output formatting for better visibility
		fmt.Printf("%s created successfully\n", capitalize(docType))
		fmt.Printf("  Name: %s\n", resultName)
		fmt.Printf("  ID:   %s\n", resultID)
		if tileCount > 0 {
			fmt.Printf("  %s: %d\n", capitalize(itemName(docType)), tileCount)
		}
		if result.ID != "" {
			fmt.Printf("  URL:  %s/ui/document/v0/#/%ss/%s\n", c.BaseURL(), docType, result.ID)
		}
		return nil
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
				if inner, ok := innerContent.(map[string]interface{}); ok {
					warnings = append(warnings, "detected double-nested content (.content.content) - using inner content")
					contentMap = inner
				}
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
		// Tiles can be either an array or a map/object
		if tiles, ok := content["tiles"].([]interface{}); ok {
			return len(tiles)
		}
		if tiles, ok := content["tiles"].(map[string]interface{}); ok {
			return len(tiles)
		}
	} else if docType == "notebook" {
		// Sections can be either an array or a map/object
		if sections, ok := content["sections"].([]interface{}); ok {
			return len(sections)
		}
		if sections, ok := content["sections"].(map[string]interface{}); ok {
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

// capitalize capitalizes the first letter of a string
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(string(s[0])) + s[1:]
}

func init() {
	// Notebook flags
	createNotebookCmd.Flags().StringP("file", "f", "", "file containing notebook definition (required)")
	createNotebookCmd.Flags().String("name", "", "name for the notebook (extracted from content if not provided)")
	createNotebookCmd.Flags().String("description", "", "description for the notebook")
	createNotebookCmd.Flags().String("id", "", "custom ID for the notebook (auto-generated if not provided)")
	createNotebookCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createNotebookCmd.MarkFlagRequired("file")

	// Dashboard flags
	createDashboardCmd.Flags().StringP("file", "f", "", "file containing dashboard definition (required)")
	createDashboardCmd.Flags().String("name", "", "name for the dashboard (extracted from content if not provided)")
	createDashboardCmd.Flags().String("description", "", "description for the dashboard")
	createDashboardCmd.Flags().String("id", "", "custom ID for the dashboard (auto-generated if not provided)")
	createDashboardCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createDashboardCmd.MarkFlagRequired("file")
}
