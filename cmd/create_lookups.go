package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

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
  dtctl create lookup -f error_codes.csv \
    --path /lookups/grail/pm/error_codes \
    --lookup-field code \
    --display-name "Error Codes"

  # Create with description
  dtctl create lookup -f error_codes.csv \
    --path /lookups/grail/pm/error_codes \
    --lookup-field code \
    --description "HTTP error code descriptions"

  # Create with custom parse pattern (pipe-delimited)
  dtctl create lookup -f data.txt \
    --path /lookups/custom/data \
    --lookup-field id \
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

func init() {
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
}
