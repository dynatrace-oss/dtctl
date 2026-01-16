package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create resources from files",
	Long:  `Create resources from YAML or JSON files.`,
}

// createWorkflowCmd creates a workflow from a file
var createWorkflowCmd = &cobra.Command{
	Use:   "workflow -f <file>",
	Short: "Create a workflow from a file",
	Long: `Create a new workflow from a YAML or JSON file.

Examples:
  # Create a workflow from YAML
  dtctl create workflow -f workflow.yaml

  # Create with template variables
  dtctl create workflow -f workflow.yaml --set env=prod --set owner=team-a

  # Dry run to preview
  dtctl create workflow -f workflow.yaml --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

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

		// Handle dry-run
		if dryRun {
			fmt.Println("Dry run: would create workflow")
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
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

		handler := workflow.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create workflow: %w", err)
		}

		fmt.Println("Workflow created successfully")
		fmt.Printf("  ID:   %s\n", result.ID)
		fmt.Printf("  Name: %s\n", result.Title)
		fmt.Printf("  URL:  %s/ui/apps/dynatrace.automations/workflows/%s\n", c.BaseURL(), result.ID)
		return nil
	},
}

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
	return string(s[0]-32) + s[1:]
}

// createBucketCmd creates a Grail bucket
var createBucketCmd = &cobra.Command{
	Use:   "bucket --name <name> --table <table> --retention <days>",
	Short: "Create a Grail storage bucket",
	Long: `Create a new Grail storage bucket.

Examples:
  # Create a logs bucket with 35 days retention
  dtctl create bucket --name custom_logs --table logs --retention 35

  # Create with display name
  dtctl create bucket --name custom_logs --table logs --retention 35 --display-name "Custom Logs Bucket"

  # Create from a file
  dtctl create bucket -f bucket.yaml

  # Dry run to preview
  dtctl create bucket --name custom_logs --table logs --retention 35 --dry-run
`,
	Aliases: []string{"bkt"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		name, _ := cmd.Flags().GetString("name")
		table, _ := cmd.Flags().GetString("table")
		retention, _ := cmd.Flags().GetInt("retention")
		displayName, _ := cmd.Flags().GetString("display-name")

		var req bucket.BucketCreate

		if file != "" {
			// Read from file
			fileData, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			jsonData, err := format.ValidateAndConvert(fileData)
			if err != nil {
				return fmt.Errorf("invalid file format: %w", err)
			}

			if err := json.Unmarshal(jsonData, &req); err != nil {
				return fmt.Errorf("failed to parse bucket definition: %w", err)
			}
		} else {
			// Use flags
			if name == "" {
				return fmt.Errorf("--name is required (or use -f to specify a file)")
			}
			if table == "" {
				return fmt.Errorf("--table is required (logs, events, or bizevents)")
			}
			if retention == 0 {
				return fmt.Errorf("--retention is required (1-3657 days)")
			}

			req = bucket.BucketCreate{
				BucketName:    name,
				Table:         table,
				RetentionDays: retention,
				DisplayName:   displayName,
			}
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create bucket\n")
			fmt.Printf("Name: %s\n", req.BucketName)
			fmt.Printf("Table: %s\n", req.Table)
			fmt.Printf("Retention: %d days\n", req.RetentionDays)
			if req.DisplayName != "" {
				fmt.Printf("Display Name: %s\n", req.DisplayName)
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

		handler := bucket.NewHandler(c)

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}

		fmt.Printf("Bucket %q created (status: %s)\n", result.BucketName, result.Status)
		fmt.Println("Note: Bucket creation can take up to 1 minute to complete")
		return nil
	},
}

// createLookupCmd creates a lookup table
var createLookupCmd = &cobra.Command{
	Use:   "lookup -f <file> --path <path> --lookup-field <field>",
	Short: "Create a lookup table",
	Long: `Create a lookup table from a CSV file or manifest.

The lookup table is stored in Grail Resource Store and can be loaded in DQL queries
for data enrichment.

For CSV files, column headers are auto-detected and a DPL parse pattern is generated automatically.
For non-CSV formats, use --parse-pattern to specify a custom Dynatrace Pattern Language pattern.

Examples:
  # Create from CSV (auto-detect headers)
  dtctl create lookup -f error_codes.csv \\
    --path /lookups/grail/pm/error_codes \\
    --lookup-field code \\
    --display-name "Error Codes"

  # Create with description
  dtctl create lookup -f error_codes.csv \\
    --path /lookups/grail/pm/error_codes \\
    --lookup-field code \\
    --description "HTTP error code descriptions"

  # Create with custom parse pattern (pipe-delimited)
  dtctl create lookup -f data.txt \\
    --path /lookups/custom/data \\
    --lookup-field id \\
    --parse-pattern "LD:id '|' LD:name '|' LD:value"

  # Create from manifest
  dtctl create lookup -f lookup-manifest.yaml

  # Dry run to preview
  dtctl create lookup -f error_codes.csv --path /lookups/test --lookup-field id --dry-run
`,
	Aliases: []string{"lkup", "lu"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		path, _ := cmd.Flags().GetString("path")
		lookupField, _ := cmd.Flags().GetString("lookup-field")
		displayName, _ := cmd.Flags().GetString("display-name")
		description, _ := cmd.Flags().GetString("description")
		parsePattern, _ := cmd.Flags().GetString("parse-pattern")
		skipRecords, _ := cmd.Flags().GetInt("skip-records")
		timezone, _ := cmd.Flags().GetString("timezone")
		locale, _ := cmd.Flags().GetString("locale")

		if file == "" {
			return fmt.Errorf("--file is required")
		}

		// Read file
		fileData, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Check if it's a manifest (YAML/JSON with apiVersion/kind)
		var manifest map[string]interface{}
		if err := json.Unmarshal(fileData, &manifest); err == nil {
			if _, hasKind := manifest["kind"]; hasKind {
				// It's a manifest - handle via apply command
				return fmt.Errorf("manifest files should be used with 'dtctl apply -f %s'", file)
			}
		}

		// Validate required flags for data files
		if path == "" {
			return fmt.Errorf("--path is required (e.g., /lookups/grail/pm/error_codes)")
		}
		if lookupField == "" {
			return fmt.Errorf("--lookup-field is required (name of the key field)")
		}

		// Build create request
		req := lookup.CreateRequest{
			FilePath:       path,
			DisplayName:    displayName,
			Description:    description,
			LookupField:    lookupField,
			ParsePattern:   parsePattern,
			SkippedRecords: skipRecords,
			Timezone:       timezone,
			Locale:         locale,
			DataContent:    fileData,
		}

		// Set defaults
		if req.Timezone == "" {
			req.Timezone = "UTC"
		}
		if req.Locale == "" {
			req.Locale = "en_US"
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create lookup table\n")
			fmt.Printf("Path: %s\n", req.FilePath)
			fmt.Printf("Lookup Field: %s\n", req.LookupField)
			if req.DisplayName != "" {
				fmt.Printf("Display Name: %s\n", req.DisplayName)
			}
			if req.Description != "" {
				fmt.Printf("Description: %s\n", req.Description)
			}
			if req.ParsePattern != "" {
				fmt.Printf("Parse Pattern: %s\n", req.ParsePattern)
			} else {
				fmt.Printf("Parse Pattern: (auto-detect from CSV)\n")
			}
			fmt.Printf("File Size: %d bytes\n", len(fileData))
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

		handler := lookup.NewHandler(c)

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create lookup table: %w", err)
		}

		fmt.Printf("Lookup table %q created successfully\n", path)
		fmt.Printf("  Records: %d\n", result.Records)
		fmt.Printf("  File Size: %d bytes\n", result.FileSize)
		if result.DiscardedDuplicates > 0 {
			fmt.Printf("  Note: %d duplicate records were discarded\n", result.DiscardedDuplicates)
		}
		return nil
	},
}

// createEdgeConnectCmd creates an EdgeConnect
var createEdgeConnectCmd = &cobra.Command{
	Use:   "edgeconnect --name <name> [--host-patterns <patterns>]",
	Short: "Create an EdgeConnect configuration",
	Long: `Create a new EdgeConnect configuration.

Examples:
  # Create an EdgeConnect with host patterns
  dtctl create edgeconnect --name my-edgeconnect --host-patterns "*.internal.example.com,api.example.com"

  # Create from a file
  dtctl create edgeconnect -f edgeconnect.yaml

  # Dry run to preview
  dtctl create edgeconnect --name my-edgeconnect --host-patterns "*.example.com" --dry-run
`,
	Aliases: []string{"ec"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		name, _ := cmd.Flags().GetString("name")
		hostPatterns, _ := cmd.Flags().GetString("host-patterns")

		var req edgeconnect.EdgeConnectCreate

		if file != "" {
			// Read from file
			fileData, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			jsonData, err := format.ValidateAndConvert(fileData)
			if err != nil {
				return fmt.Errorf("invalid file format: %w", err)
			}

			if err := json.Unmarshal(jsonData, &req); err != nil {
				return fmt.Errorf("failed to parse EdgeConnect definition: %w", err)
			}
		} else {
			// Use flags
			if name == "" {
				return fmt.Errorf("--name is required (or use -f to specify a file)")
			}

			var patterns []string
			if hostPatterns != "" {
				patterns = strings.Split(hostPatterns, ",")
				for i := range patterns {
					patterns[i] = strings.TrimSpace(patterns[i])
				}
			}

			req = edgeconnect.EdgeConnectCreate{
				Name:         name,
				HostPatterns: patterns,
			}
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create EdgeConnect\n")
			fmt.Printf("Name: %s\n", req.Name)
			if len(req.HostPatterns) > 0 {
				fmt.Printf("Host Patterns: %s\n", strings.Join(req.HostPatterns, ", "))
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

		handler := edgeconnect.NewHandler(c)

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create EdgeConnect: %w", err)
		}

		fmt.Printf("EdgeConnect %q created (ID: %s)\n", result.Name, result.ID)
		if result.OAuthClientSecret != "" {
			fmt.Println("\nOAuth Client Credentials (save these, the secret won't be shown again):")
			fmt.Printf("  Client ID:     %s\n", result.OAuthClientID)
			fmt.Printf("  Client Secret: %s\n", result.OAuthClientSecret)
			if result.OAuthClientResource != "" {
				fmt.Printf("  Resource:      %s\n", result.OAuthClientResource)
			}
		}
		return nil
	},
}

// createSLOCmd creates an SLO from a file
var createSLOCmd = &cobra.Command{
	Use:   "slo -f <file>",
	Short: "Create a service-level objective from a file",
	Long: `Create a new SLO from a YAML or JSON file.

Examples:
  # Create an SLO from YAML
  dtctl create slo -f slo.yaml

  # Create with template variables
  dtctl create slo -f slo.yaml --set target=99.9

  # Dry run to preview
  dtctl create slo -f slo.yaml --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

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

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create SLO\n")
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
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

		handler := slo.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create SLO: %w", err)
		}

		fmt.Println("SLO created successfully")
		fmt.Printf("  ID:   %s\n", result.ID)
		fmt.Printf("  Name: %s\n", result.Name)
		fmt.Printf("  URL:  %s/ui/apps/dynatrace.site.reliability/slos/%s\n", c.BaseURL(), result.ID)
		return nil
	},
}

// createSettingsCmd creates a settings object from a file
var createSettingsCmd = &cobra.Command{
	Use:   "settings -f <file> --schema <schema-id> --scope <scope>",
	Short: "Create a settings object from a file",
	Long: `Create a new settings object from a YAML or JSON file.

Examples:
  # Create a settings object
  dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment

  # Create with template variables
  dtctl create settings -f settings.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --set name=prod

  # Dry run to preview
  dtctl create settings -f settings.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --dry-run
`,
	Aliases: []string{"setting"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")
		setFlags, _ := cmd.Flags().GetStringArray("set")

		if file == "" {
			return fmt.Errorf("--file is required")
		}
		if schemaID == "" {
			return fmt.Errorf("--schema is required")
		}
		if scope == "" {
			return fmt.Errorf("--scope is required")
		}

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

		// Parse the value
		var value map[string]any
		if err := json.Unmarshal(jsonData, &value); err != nil {
			return fmt.Errorf("failed to parse settings value: %w", err)
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create settings object\n")
			fmt.Printf("Schema: %s\n", schemaID)
			fmt.Printf("Scope: %s\n", scope)
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
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

		handler := settings.NewHandler(c)

		result, err := handler.Create(settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		})
		if err != nil {
			return fmt.Errorf("failed to create settings object: %w", err)
		}

		fmt.Printf("Settings object %q created successfully\n", result.ObjectID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createWorkflowCmd)
	createCmd.AddCommand(createNotebookCmd)
	createCmd.AddCommand(createDashboardCmd)
	createCmd.AddCommand(createSettingsCmd)
	createCmd.AddCommand(createSLOCmd)
	createCmd.AddCommand(createBucketCmd)
	createCmd.AddCommand(createLookupCmd)
	createCmd.AddCommand(createEdgeConnectCmd)

	// Workflow flags
	createWorkflowCmd.Flags().StringP("file", "f", "", "file containing workflow definition (required)")
	createWorkflowCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createWorkflowCmd.MarkFlagRequired("file")

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

	// Settings flags
	createSettingsCmd.Flags().StringP("file", "f", "", "file containing settings value (required)")
	createSettingsCmd.Flags().String("schema", "", "schema ID (required)")
	createSettingsCmd.Flags().String("scope", "", "scope for the settings object (required)")
	createSettingsCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createSettingsCmd.MarkFlagRequired("file")
	_ = createSettingsCmd.MarkFlagRequired("schema")
	_ = createSettingsCmd.MarkFlagRequired("scope")

	// SLO flags
	createSLOCmd.Flags().StringP("file", "f", "", "file containing SLO definition (required)")
	createSLOCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createSLOCmd.MarkFlagRequired("file")

	// Bucket flags
	createBucketCmd.Flags().StringP("file", "f", "", "file containing bucket definition")
	createBucketCmd.Flags().String("name", "", "bucket name (3-100 chars, lowercase alphanumeric, underscores, hyphens)")
	createBucketCmd.Flags().String("table", "", "table type (logs, events, or bizevents)")
	createBucketCmd.Flags().Int("retention", 0, "retention period in days (1-3657)")
	createBucketCmd.Flags().String("display-name", "", "display name for the bucket")

	// Lookup flags
	createLookupCmd.Flags().StringP("file", "f", "", "path to data file or manifest (required)")
	createLookupCmd.Flags().String("path", "", "lookup file path (e.g., /lookups/grail/pm/error_codes)")
	createLookupCmd.Flags().String("lookup-field", "", "name of the lookup key field")
	createLookupCmd.Flags().String("display-name", "", "display name for the lookup table")
	createLookupCmd.Flags().String("description", "", "description of the lookup table")
	createLookupCmd.Flags().String("parse-pattern", "", "custom DPL parse pattern (auto-detected for CSV)")
	createLookupCmd.Flags().Int("skip-records", 0, "number of records to skip (e.g., 1 for CSV headers)")
	createLookupCmd.Flags().String("timezone", "UTC", "timezone for parsing time/date fields")
	createLookupCmd.Flags().String("locale", "en_US", "locale for parsing locale-specific data")
	_ = createLookupCmd.MarkFlagRequired("file")

	// EdgeConnect flags
	createEdgeConnectCmd.Flags().StringP("file", "f", "", "file containing EdgeConnect definition")
	createEdgeConnectCmd.Flags().String("name", "", "EdgeConnect name (RFC 1123 compliant, max 50 chars)")
	createEdgeConnectCmd.Flags().String("host-patterns", "", "comma-separated list of host patterns")
}
