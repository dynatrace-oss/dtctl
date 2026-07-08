package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
)

var editAzureMonitoringName string

var editAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Edit Azure resources",
	RunE:  requireSubcommand,
}

var editAzureMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Edit an Azure monitoring configuration",
	Long: `Edit an Azure monitoring configuration by opening it in your default editor.

The configuration will be fetched, opened in your editor (defined by EDITOR env var,
defaults to vim), and updated when you save and close the editor.

By default, resources are edited in YAML format for better readability.
Use --format=json to edit in JSON format.

Examples:
  dtctl edit azure monitoring <id>
  dtctl edit azure monitoring --name "my-azure-monitoring"
  dtctl edit azure monitoring --name "my-azure-monitoring" --format=json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && editAzureMonitoringName == "" {
			return fmt.Errorf("provide monitoring config ID argument or --name")
		}

		cfg, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		handler := azuremonitoringconfig.NewHandler(c)

		var existing *azuremonitoringconfig.AzureMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = handler.FindByName(identifier)
			if err != nil {
				existing, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("azure monitoring config %q not found by name or ID", identifier)
				}
			}
		} else {
			existing, err = handler.FindByName(editAzureMonitoringName)
			if err != nil {
				return err
			}
		}

		data, err := handler.GetRaw(existing.ObjectID)
		if err != nil {
			return err
		}

		editFormat, _ := cmd.Flags().GetString("format")
		var editData []byte
		var fileExt string

		if editFormat == "yaml" {
			editData, err = format.JSONToYAML(data)
			if err != nil {
				return fmt.Errorf("failed to convert to YAML: %w", err)
			}
			fileExt = "*.yaml"
		} else {
			editData, err = format.PrettyJSON(data)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fileExt = "*.json"
		}

		tmpfile, err := os.CreateTemp("", "dtctl-azure-monitoring-"+fileExt)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer func() {
			_ = os.Remove(tmpfile.Name())
		}()

		if _, err := tmpfile.Write(editData); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Preferences.Editor
		}
		if editor == "" {
			editor = "vim"
		}

		parts := strings.Fields(editor)
		if len(parts) == 0 {
			return fmt.Errorf("no editor configured")
		}
		editorCmd := exec.Command(parts[0], append(parts[1:], tmpfile.Name())...)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		editedData, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return fmt.Errorf("failed to read edited file: %w", err)
		}

		jsonData, err := format.ValidateAndConvert(editedData)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}

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

		var editedValue azuremonitoringconfig.Value
		if err := json.Unmarshal(jsonData, &editedValue); err != nil {
			return fmt.Errorf("failed to parse edited config: %w", err)
		}
		payload := azuremonitoringconfig.AzureMonitoringConfig{Scope: existing.Scope, Value: editedValue}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		updated, err := handler.Update(existing.ObjectID, payloadBytes)
		if err != nil {
			return err
		}

		configName := updated.Value.Description
		if configName == "" {
			configName = updated.ObjectID
		}
		output.PrintSuccess("Azure monitoring config %q updated", configName)
		return nil
	},
}

func init() {
	editAzureMonitoringCmd.Flags().StringP("format", "", "yaml", "edit format (yaml|json)")
	editAzureMonitoringCmd.Flags().StringVar(&editAzureMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
}
