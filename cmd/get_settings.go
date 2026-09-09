package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
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
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

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
	Use:     "settings [object-id]",
	Aliases: []string{"setting"},
	Short:   "Get settings objects",
	Long: `Get settings objects for a schema, or a specific object by objectId.

Examples:
  # List settings objects for a schema
  dtctl get settings --schema builtin:openpipeline.logs.pipelines

  # List settings with a specific scope
  dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment

  # Get a specific settings object by objectId
  dtctl get settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Output as JSON
  dtctl get settings --schema builtin:openpipeline.logs.pipelines -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get specific object if ID provided
		if len(args) > 0 {
			obj, err := handler.Get(args[0])
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
	Use:   "settings <object-id>",
	Short: "Delete a settings object",
	Long: `Delete a settings object by objectId.

Examples:
  # Delete by objectId
  dtctl delete settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Delete without confirmation
  dtctl delete settings <object-id> -y
`,
	Aliases: []string{"setting"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		objectID := args[0]

		_, c, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get current settings object for confirmation
		obj, err := handler.Get(objectID)
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

		if err := handler.Delete(objectID); err != nil {
			return err
		}

		output.PrintSuccess("Settings object %q deleted", objectID)
		return nil
	},
}

func init() {
	// Settings flags
	getSettingsCmd.Flags().String("schema", "", "Schema ID (required when listing settings objects)")
	getSettingsCmd.Flags().String("scope", "", "Scope to filter settings (e.g., 'environment')")

	// Delete settings flags
	deleteSettingsCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}
