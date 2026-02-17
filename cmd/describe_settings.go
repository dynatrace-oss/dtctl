package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/spf13/cobra"
)

// describeSettingsCmd shows detailed info about a settings object
var describeSettingsCmd = &cobra.Command{
	Use:     "settings <object-id-or-uid>",
	Aliases: []string{"setting", "set"},
	Short:   "Show details of a settings object",
	Long: `Show detailed information about a settings object including its value, scope, and metadata.

You can specify either:
  - ObjectID: The full base64-encoded object identifier
  - UID: A human-readable UUID (requires --schema-id and/or --scope for disambiguation)

Examples:
  # Describe a settings object by ObjectID
  dtctl describe settings vu9U3hXa3q0AAAABABlidWlsdGluOnJ1bS5mcm9...

  # Describe a settings object by UID
  dtctl describe settings b396f4-ec8f-3e02-bcef-0328b86a63cc --schema-id builtin:rum.frontend.name
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idOrUID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get optional context flags for UID resolution
		schemaID, _ := cmd.Flags().GetString("schema-id")
		scope, _ := cmd.Flags().GetString("scope")

		// Get settings object
		obj, err := handler.GetWithContext(idOrUID, schemaID, scope)
		if err != nil {
			return err
		}

		// Print settings object details
		fmt.Printf("Object ID:    %s\n", obj.ObjectID)
		if obj.UID != "" {
			fmt.Printf("UID:          %s\n", obj.UID)
		}
		fmt.Printf("Schema ID:    %s\n", obj.SchemaID)
		if obj.SchemaVersion != "" {
			fmt.Printf("Version:      %s\n", obj.SchemaVersion)
		}
		fmt.Printf("Scope:        %s\n", obj.Scope)
		if obj.ScopeType != "" {
			fmt.Printf("Scope Type:   %s\n", obj.ScopeType)
		}
		if obj.ScopeID != "" {
			fmt.Printf("Scope ID:     %s\n", obj.ScopeID)
		}
		if obj.ExternalID != "" {
			fmt.Printf("External ID:  %s\n", obj.ExternalID)
		}
		if obj.Summary != "" {
			fmt.Printf("Summary:      %s\n", obj.Summary)
		}

		// Print modification info
		if obj.ModificationInfo != nil {
			fmt.Println()
			if obj.ModificationInfo.CreatedTime != "" {
				fmt.Printf("Created:      %s", obj.ModificationInfo.CreatedTime)
				if obj.ModificationInfo.CreatedBy != "" {
					fmt.Printf(" (by %s)", obj.ModificationInfo.CreatedBy)
				}
				fmt.Println()
			}
			if obj.ModificationInfo.LastModifiedTime != "" {
				fmt.Printf("Modified:     %s", obj.ModificationInfo.LastModifiedTime)
				if obj.ModificationInfo.LastModifiedBy != "" {
					fmt.Printf(" (by %s)", obj.ModificationInfo.LastModifiedBy)
				}
				fmt.Println()
			}
		}

		// Print value as JSON
		if len(obj.Value) > 0 {
			fmt.Println()
			fmt.Println("Value:")
			valueJSON, err := json.MarshalIndent(obj.Value, "  ", "  ")
			if err == nil {
				fmt.Printf("  %s\n", string(valueJSON))
			}
		}

		return nil
	},
}

// describeSettingsSchemaCmd shows detailed info about a settings schema
var describeSettingsSchemaCmd = &cobra.Command{
	Use:     "settings-schema <schema-id>",
	Aliases: []string{"schema"},
	Short:   "Show details of a settings schema",
	Long: `Show detailed information about a settings schema including properties and validation rules.

Examples:
  # Describe a settings schema
  dtctl describe settings-schema builtin:openpipeline.logs.pipelines
  dtctl describe schema builtin:anomaly-detection.infrastructure
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		schema, err := handler.GetSchema(schemaID)
		if err != nil {
			return err
		}

		// Extract and print key schema information
		if schemaID, ok := schema["schemaId"].(string); ok {
			fmt.Printf("Schema ID:        %s\n", schemaID)
		}
		if displayName, ok := schema["displayName"].(string); ok {
			fmt.Printf("Display Name:     %s\n", displayName)
		}
		if description, ok := schema["description"].(string); ok && description != "" {
			fmt.Printf("Description:      %s\n", description)
		}
		if version, ok := schema["version"].(string); ok {
			fmt.Printf("Version:          %s\n", version)
		}
		if multiObj, ok := schema["multiObject"].(bool); ok {
			fmt.Printf("Multi-Object:     %v\n", multiObj)
		}
		if ordered, ok := schema["ordered"].(bool); ok {
			fmt.Printf("Ordered:          %v\n", ordered)
		}

		// Print properties if available
		if properties, ok := schema["properties"].(map[string]any); ok && len(properties) > 0 {
			fmt.Println()
			fmt.Printf("Properties:       %d defined\n", len(properties))
		}

		// Print scopes if available
		if scopesRaw, ok := schema["scopes"].([]any); ok && len(scopesRaw) > 0 {
			fmt.Println()
			fmt.Println("Scopes:")
			for _, s := range scopesRaw {
				if scope, ok := s.(string); ok {
					fmt.Printf("  - %s\n", scope)
				}
			}
		}

		return nil
	},
}

func init() {
	// Add flags for settings command
	describeSettingsCmd.Flags().String("schema-id", "", "Schema ID to use for UID resolution")
	describeSettingsCmd.Flags().String("scope", "", "Scope to use for UID resolution")
}
