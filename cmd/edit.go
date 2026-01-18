package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/spf13/cobra"
)

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a resource",
	Long:  `Edit a resource using the default editor.`,
	RunE:  requireSubcommand,
}

// editWorkflowCmd edits a workflow
var editWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id-or-name>",
	Aliases: []string{"workflows", "wf"},
	Short:   "Edit a workflow",
	Long: `Edit a workflow by opening it in your default editor.

The workflow will be fetched, opened in your editor (defined by EDITOR env var,
defaults to vim), and updated when you save and close the editor.

By default, resources are edited in YAML format for better readability.
Use --format=json to edit in JSON format.

Examples:
  # Edit a workflow in YAML (default)
  dtctl edit workflow <workflow-id>
  dtctl edit workflow "My Workflow"

  # Edit a workflow in JSON
  dtctl edit workflow <workflow-id> --format=json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		workflowID, err := res.ResolveID(resolver.TypeWorkflow, identifier)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)

		// Get workflow to check ownership
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Determine ownership for safety check
		currentUserID, _ := c.CurrentUserID() // Ignore error - will be empty string
		ownership := safety.DetermineOwnership(wf.Owner, currentUserID)

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, ownership); err != nil {
			return err
		}

		// Get the workflow as raw JSON
		data, err := handler.GetRaw(workflowID)
		if err != nil {
			return err
		}

		// Get format preference
		editFormat, _ := cmd.Flags().GetString("format")
		var editData []byte
		var fileExt string

		if editFormat == "yaml" {
			// Convert JSON to YAML for editing
			editData, err = format.JSONToYAML(data)
			if err != nil {
				return fmt.Errorf("failed to convert to YAML: %w", err)
			}
			fileExt = "*.yaml"
		} else {
			// Pretty print JSON for editing
			editData, err = format.PrettyJSON(data)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fileExt = "*.json"
		}

		// Create a temp file with appropriate extension
		tmpfile, err := os.CreateTemp("", "dtctl-workflow-"+fileExt)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(editData); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		// Get the editor
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Preferences.Editor
		}
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		editorCmd := exec.Command(editor, tmpfile.Name())
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Read the edited file
		editedData, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return fmt.Errorf("failed to read edited file: %w", err)
		}

		// Convert edited data to JSON (auto-detect format)
		jsonData, err := format.ValidateAndConvert(editedData)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}

		// Check if anything changed
		var originalCompact, editedCompact bytes.Buffer
		if err := json.Compact(&originalCompact, data); err != nil {
			return fmt.Errorf("failed to compact original JSON: %w", err)
		}
		if err := json.Compact(&editedCompact, jsonData); err != nil {
			return fmt.Errorf("failed to compact edited JSON: %w", err)
		}

		if bytes.Equal(originalCompact.Bytes(), editedCompact.Bytes()) {
			fmt.Println("Edit cancelled, no changes made.")
			return nil
		}

		// Update the workflow
		result, err := handler.Update(workflowID, jsonData)
		if err != nil {
			return err
		}

		fmt.Printf("Workflow %q updated successfully\n", result.Title)
		return nil
	},
}

// editDashboardCmd edits a dashboard
var editDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id-or-name>",
	Aliases: []string{"dashboards", "dash", "db"},
	Short:   "Edit a dashboard",
	Long: `Edit a dashboard by opening it in your default editor.

The dashboard will be fetched, opened in your editor (defined by EDITOR env var,
defaults to vim), and updated when you save and close the editor.

By default, resources are edited in YAML format for better readability.
Use --format=json to edit in JSON format.

Examples:
  # Edit a dashboard in YAML (default)
  dtctl edit dashboard <dashboard-id>
  dtctl edit dashboard "Production Dashboard"

  # Edit a dashboard in JSON
  dtctl edit dashboard <dashboard-id> --format=json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		dashboardID, err := res.ResolveID(resolver.TypeDashboard, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get the dashboard with content
		doc, err := handler.Get(dashboardID)
		if err != nil {
			return err
		}

		// Get metadata separately for ownership check - the multipart response
		// from Get() may not include the owner field
		metadata, err := handler.GetMetadata(dashboardID)
		if err != nil {
			return err
		}

		// Determine ownership for safety check
		currentUserID, _ := c.CurrentUserID() // Ignore error - will be empty string
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, ownership); err != nil {
			return err
		}

		// Get format preference
		editFormat, _ := cmd.Flags().GetString("format")
		var editData []byte
		var fileExt string

		if editFormat == "yaml" {
			// Convert JSON to YAML for editing
			editData, err = format.JSONToYAML(doc.Content)
			if err != nil {
				return fmt.Errorf("failed to convert to YAML: %w", err)
			}
			fileExt = "*.yaml"
		} else {
			// Pretty print JSON for editing
			editData, err = format.PrettyJSON(doc.Content)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fileExt = "*.json"
		}

		// Create a temp file with appropriate extension
		tmpfile, err := os.CreateTemp("", "dtctl-dashboard-"+fileExt)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(editData); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		// Get the editor
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Preferences.Editor
		}
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		editorCmd := exec.Command(editor, tmpfile.Name())
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Read the edited file
		editedData, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return fmt.Errorf("failed to read edited file: %w", err)
		}

		// Convert edited data to JSON (auto-detect format)
		jsonData, err := format.ValidateAndConvert(editedData)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}

		// Check if anything changed
		var originalCompact, editedCompact bytes.Buffer
		if err := json.Compact(&originalCompact, doc.Content); err != nil {
			return fmt.Errorf("failed to compact original JSON: %w", err)
		}
		if err := json.Compact(&editedCompact, jsonData); err != nil {
			return fmt.Errorf("failed to compact edited JSON: %w", err)
		}

		if bytes.Equal(originalCompact.Bytes(), editedCompact.Bytes()) {
			fmt.Println("Edit cancelled, no changes made.")
			return nil
		}

		// Re-fetch metadata for version (optimistic locking) - the document may have been
		// modified since we fetched it for the ownership check
		metadata, err = handler.GetMetadata(dashboardID)
		if err != nil {
			return err
		}

		// Update the dashboard
		result, err := handler.Update(dashboardID, metadata.Version, jsonData, "application/json")
		if err != nil {
			return err
		}

		fmt.Printf("Dashboard %q updated successfully\n", result.Name)
		return nil
	},
}

// editNotebookCmd edits a notebook
var editNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id-or-name>",
	Aliases: []string{"notebooks", "nb"},
	Short:   "Edit a notebook",
	Long: `Edit a notebook by opening it in your default editor.

The notebook will be fetched, opened in your editor (defined by EDITOR env var,
defaults to vim), and updated when you save and close the editor.

By default, resources are edited in YAML format for better readability.
Use --format=json to edit in JSON format.

Examples:
  # Edit a notebook in YAML (default)
  dtctl edit notebook <notebook-id>
  dtctl edit notebook "Analysis Notebook"

  # Edit a notebook in JSON
  dtctl edit notebook <notebook-id> --format=json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		notebookID, err := res.ResolveID(resolver.TypeNotebook, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get the notebook with content
		doc, err := handler.Get(notebookID)
		if err != nil {
			return err
		}

		// Get metadata separately for ownership check - the multipart response
		// from Get() may not include the owner field
		metadata, err := handler.GetMetadata(notebookID)
		if err != nil {
			return err
		}

		// Determine ownership for safety check
		currentUserID, _ := c.CurrentUserID() // Ignore error - will be empty string
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, ownership); err != nil {
			return err
		}

		// Get format preference
		editFormat, _ := cmd.Flags().GetString("format")
		var editData []byte
		var fileExt string

		if editFormat == "yaml" {
			// Convert JSON to YAML for editing
			editData, err = format.JSONToYAML(doc.Content)
			if err != nil {
				return fmt.Errorf("failed to convert to YAML: %w", err)
			}
			fileExt = "*.yaml"
		} else {
			// Pretty print JSON for editing
			editData, err = format.PrettyJSON(doc.Content)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fileExt = "*.json"
		}

		// Create a temp file with appropriate extension
		tmpfile, err := os.CreateTemp("", "dtctl-notebook-"+fileExt)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(editData); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		// Get the editor
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Preferences.Editor
		}
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		editorCmd := exec.Command(editor, tmpfile.Name())
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Read the edited file
		editedData, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return fmt.Errorf("failed to read edited file: %w", err)
		}

		// Convert edited data to JSON (auto-detect format)
		jsonData, err := format.ValidateAndConvert(editedData)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}

		// Check if anything changed
		var originalCompact, editedCompact bytes.Buffer
		if err := json.Compact(&originalCompact, doc.Content); err != nil {
			return fmt.Errorf("failed to compact original JSON: %w", err)
		}
		if err := json.Compact(&editedCompact, jsonData); err != nil {
			return fmt.Errorf("failed to compact edited JSON: %w", err)
		}

		if bytes.Equal(originalCompact.Bytes(), editedCompact.Bytes()) {
			fmt.Println("Edit cancelled, no changes made.")
			return nil
		}

		// Re-fetch metadata for version (optimistic locking) - the document may have been
		// modified since we fetched it for the ownership check
		metadata, err = handler.GetMetadata(notebookID)
		if err != nil {
			return err
		}

		// Update the notebook
		result, err := handler.Update(notebookID, metadata.Version, jsonData, "application/json")
		if err != nil {
			return err
		}

		fmt.Printf("Notebook %q updated successfully\n", result.Name)
		return nil
	},
}

// editSettingCmd edits a settings object
var editSettingCmd = &cobra.Command{
	Use:     "setting <object-id>",
	Aliases: []string{"settings"},
	Short:   "Edit a settings object",
	Long: `Edit a settings object by opening it in your default editor.

The settings object will be fetched, opened in your editor (defined by EDITOR env var,
defaults to vim), and updated when you save and close the editor.

By default, settings are edited in YAML format for better readability.
Use --format=json to edit in JSON format.

The object ID can be provided as either:
- Full object ID (e.g., 14daea00-ea60-323d-9f79-9ea2d899bccd)
- UID (requires --schema and --scope flags)

Examples:
  # Edit a settings object in YAML (default)
  dtctl edit setting 14daea00-ea60-323d-9f79-9ea2d899bccd

  # Edit a settings object in JSON
  dtctl edit setting 14daea00-ea60-323d-9f79-9ea2d899bccd --format=json

  # Edit using UID (requires schema and scope)
  dtctl edit setting my-uid --schema builtin:rum.frontend.name --scope environment
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get schema and scope flags for UID resolution
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		// Get the settings object as raw JSON
		data, err := handler.GetRaw(identifier)
		if err != nil {
			// If GetRaw fails and we have schema/scope, try with context
			if schemaID != "" && scope != "" {
				obj, err := handler.GetWithContext(identifier, schemaID, scope)
				if err != nil {
					return err
				}
				data, err = json.MarshalIndent(obj.Value, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal settings value: %w", err)
				}
			} else {
				return err
			}
		}

		// Get format preference
		editFormat, _ := cmd.Flags().GetString("format")
		var editData []byte
		var fileExt string

		if editFormat == "yaml" {
			// Convert JSON to YAML for editing
			editData, err = format.JSONToYAML(data)
			if err != nil {
				return fmt.Errorf("failed to convert to YAML: %w", err)
			}
			fileExt = "*.yaml"
		} else {
			// Pretty print JSON for editing
			editData, err = format.PrettyJSON(data)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fileExt = "*.json"
		}

		// Create a temp file with appropriate extension
		tmpfile, err := os.CreateTemp("", "dtctl-setting-"+fileExt)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(editData); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		// Get the editor
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Preferences.Editor
		}
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		editorCmd := exec.Command(editor, tmpfile.Name())
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Read the edited file
		editedData, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return fmt.Errorf("failed to read edited file: %w", err)
		}

		// Convert edited data to JSON (auto-detect format)
		jsonData, err := format.ValidateAndConvert(editedData)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}

		// Check if anything changed
		var originalCompact, editedCompact bytes.Buffer
		if err := json.Compact(&originalCompact, data); err != nil {
			return fmt.Errorf("failed to compact original JSON: %w", err)
		}
		if err := json.Compact(&editedCompact, jsonData); err != nil {
			return fmt.Errorf("failed to compact edited JSON: %w", err)
		}

		if bytes.Equal(originalCompact.Bytes(), editedCompact.Bytes()) {
			fmt.Println("Edit cancelled, no changes made.")
			return nil
		}

		// Parse the edited JSON into a map for the Update call
		var value map[string]any
		if err := json.Unmarshal(jsonData, &value); err != nil {
			return fmt.Errorf("failed to parse edited JSON: %w", err)
		}

		// Update the settings object
		var result *settings.SettingsObject
		if schemaID != "" && scope != "" {
			result, err = handler.UpdateWithContext(identifier, value, schemaID, scope)
		} else {
			result, err = handler.Update(identifier, value)
		}
		if err != nil {
			return err
		}

		fmt.Printf("Settings object %q updated successfully\n", result.ObjectID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
	editCmd.AddCommand(editWorkflowCmd)
	editCmd.AddCommand(editDashboardCmd)
	editCmd.AddCommand(editNotebookCmd)
	editCmd.AddCommand(editSettingCmd)

	// Add format flag to all edit commands
	editWorkflowCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")
	editDashboardCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")
	editNotebookCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")
	editSettingCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")

	// Add schema and scope flags for settings (optional, for UID resolution)
	editSettingCmd.Flags().StringP("schema", "", "", "schema ID (required if using UID instead of object ID)")
	editSettingCmd.Flags().StringP("scope", "", "", "scope (required if using UID instead of object ID)")
}
