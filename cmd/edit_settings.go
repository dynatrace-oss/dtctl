package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
)

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

Examples:
  # Edit a settings object in YAML (default)
  dtctl edit setting vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Edit a settings object in JSON
  dtctl edit setting vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ... --format=json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get the settings object as raw JSON
		data, err := handler.GetRaw(identifier)
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
		result, err := handler.Update(identifier, value)
		if err != nil {
			return err
		}

		output.PrintSuccess("Settings object %q updated", result.ObjectID)
		return nil
	},
}

func init() {
	editSettingCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")
}
