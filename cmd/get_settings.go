package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// getSettingsSchemasCmd retrieves settings schemas
var getSettingsSchemasCmd = &cobra.Command{
	Use:     "settings-schemas [schema-id]",
	Aliases: []string{"settings-schema", "schemas", "schema"},
	Short:   "Get settings schemas",
	Long: `Get available settings schemas.

Examples:
  # List all settings schemas
  dtctl get settings-schemas

  # Get a specific schema definition
  dtctl get settings-schema builtin:openpipeline.logs.pipelines

  # Output as JSON
  dtctl get settings-schemas -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)
		printer := NewPrinter()

		// Get specific schema if ID provided
		if len(args) > 0 {
			schema, err := handler.GetSchema(args[0])
			if err != nil {
				return err
			}
			return printer.Print(schema)
		}

		// List all schemas
		list, err := handler.ListSchemas()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Items)
	},
}

// getSettingsCmd retrieves settings objects
var getSettingsCmd = &cobra.Command{
	Use:     "settings [object-id-or-uid]",
	Aliases: []string{"setting"},
	Short:   "Get settings objects",
	Long: `Get settings objects for a schema.

You can retrieve a specific settings object by providing either:
- The full objectId (base64-encoded composite key) - no flags needed
- The UID (UUID format) - REQUIRES --schema flag

When using a UID, you MUST specify --schema to narrow the search. This prevents
expensive operations that could search through thousands of objects and put load
on the Dynatrace backend.

Examples:
  # List settings objects for a schema
  dtctl get settings --schema builtin:openpipeline.logs.pipelines

  # List settings with a specific scope
  dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment

  # Get by objectId (direct API call, no flags needed)
  dtctl get settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Get by UID (requires --schema flag)
  dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines

  # Get by UID with custom scope
  dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines --scope environment

  # Output as JSON
  dtctl get settings --schema builtin:openpipeline.logs.pipelines -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)
		printer := NewPrinter()

		// Get specific object if ID provided
		if len(args) > 0 {
			obj, err := handler.GetWithContext(args[0], schemaID, scope)
			if err != nil {
				return err
			}
			return printer.Print(obj)
		}

		// List objects for schema
		if schemaID == "" {
			return fmt.Errorf("--schema is required when listing settings objects")
		}

		list, err := handler.ListObjects(schemaID, scope, GetChunkSize())
		if err != nil {
			return err
		}

		return printer.PrintList(list.Items)
	},
}

// deleteSettingsCmd deletes a settings object
var deleteSettingsCmd = &cobra.Command{
	Use:   "settings <object-id-or-uid>",
	Short: "Delete a settings object",
	Long: `Delete a settings object by objectId or UID.

You can specify either the full objectId or the UID (UUID format).
When using a UID, you MUST specify --schema.

Examples:
  # Delete by objectId
  dtctl delete settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Delete by UID (requires --schema)
  dtctl delete settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines

  # Delete without confirmation
  dtctl delete settings <object-id-or-uid> -y
`,
	Aliases: []string{"setting"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		objectID := args[0]
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get current settings object for confirmation
		obj, err := handler.GetWithContext(objectID, schemaID, scope)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			summary := obj.Summary
			if summary == "" {
				summary = obj.SchemaID
			}
			if !prompt.ConfirmDeletion("settings object", summary, objectID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.DeleteWithContext(objectID, schemaID, scope); err != nil {
			return err
		}

		fmt.Printf("Settings object %q deleted\n", objectID)
		return nil
	},
}

func init() {
	// Settings flags
	getSettingsCmd.Flags().String("schema", "", "Schema ID (required when listing or using UID)")
	getSettingsCmd.Flags().String("scope", "", "Scope to filter settings (e.g., 'environment')")

	// Delete settings flags
	deleteSettingsCmd.Flags().String("schema", "", "Schema ID (required when using UID)")
	deleteSettingsCmd.Flags().String("scope", "", "Scope for UID resolution (optional, defaults to 'environment')")
	deleteSettingsCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}
