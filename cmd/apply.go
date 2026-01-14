package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply -f <file>",
	Short: "Apply a configuration to create or update resources",
	Long: `Apply a configuration to create or update resources from YAML or JSON files.

The apply command reads a resource definition from a file and applies it to the
Dynatrace environment. Resources are updated if they already exist (based on ID).

How it works:
  - If the file contains an 'id' field and that resource exists: UPDATE
  - If the file contains an 'id' field but resource doesn't exist: CREATE with that ID
  - If the file has no 'id' field: CREATE with auto-generated ID

This is similar to 'kubectl apply' - use it to keep resources in sync with their
file definitions. For round-trip workflows, use 'dtctl get <resource> -o yaml' to
export, edit, and apply back.

Template variables can be used with the --set flag for reusable configurations,
making it easy to deploy the same resource across multiple environments.

Supported resource types:
  - Workflows (automation)
  - Dashboards
  - Notebooks
  - SLOs
  - Grail buckets
  - Settings objects

Examples:
  # Create a new dashboard (no ID in file)
  dtctl apply -f dashboard.yaml

  # Update existing dashboard (file exported with 'get' command includes ID)
  dtctl get dashboard my-dash -o yaml > dashboard.yaml
  # Edit dashboard.yaml...
  dtctl apply -f dashboard.yaml  # Updates the existing dashboard

  # Update a settings object
  dtctl get settings <objectId> -o yaml > setting.yaml
  # Edit setting.yaml (modify the 'value' field)...
  dtctl apply -f setting.yaml  # Updates the existing setting

  # Apply with template variables
  dtctl apply -f dashboard.yaml --set environment=prod --set owner=team-a

  # Preview changes before applying
  dtctl apply -f notebook.yaml --dry-run

  # See what changed when updating
  dtctl apply -f dashboard.yaml --show-diff

Note: To update a dashboard via command line, use the apply command with a file
that contains the dashboard ID. The 'create' command always creates new resources.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		setFlags, _ := cmd.Flags().GetStringArray("set")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		showDiff, _ := cmd.Flags().GetBool("show-diff")

		// Read the file
		fileData, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Parse template variables
		var templateVars map[string]interface{}
		if len(setFlags) > 0 {
			templateVars, err = template.ParseSetFlags(setFlags)
			if err != nil {
				return fmt.Errorf("invalid --set flag: %w", err)
			}
		}

		// Load configuration
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Create applier
		applier := apply.NewApplier(c)

		// Apply the resource
		opts := apply.ApplyOptions{
			TemplateVars: templateVars,
			DryRun:       dryRun,
			ShowDiff:     showDiff,
		}

		return applier.Apply(fileData, opts)
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringP("file", "f", "", "file containing resource definition (required)")
	applyCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	applyCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	applyCmd.Flags().Bool("show-diff", false, "show diff of changes when updating existing resources")

	_ = applyCmd.MarkFlagRequired("file")
}
