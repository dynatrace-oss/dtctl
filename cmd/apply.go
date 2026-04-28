package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
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

Idempotent local workflow (--write-id):
  Use --write-id on first apply to stamp the generated ID back into the source file.
  Every subsequent apply will then update in place automatically — no manual ID
  tracking required.

    dtctl apply -f dashboard.yaml --write-id   # creates, stamps id into file
    dtctl apply -f dashboard.yaml              # updates the same dashboard

  If you forgot --write-id on the first run, use --id to recover without creating
  a duplicate:

    dtctl apply -f dashboard.yaml --write-id --id <id-from-first-run>

Template-driven deployments (--id):
  Keep a reusable template file (no ID) and supply the target ID externally.
  Ideal for CI pipelines or deploying the same template to a known resource.

    dtctl apply -f template.yaml --id $DASHBOARD_ID

Template variables can be used with the --set flag for reusable configurations,
making it easy to deploy the same resource across multiple environments.

Supported resource types:
  - Workflows (automation)
  - Dashboards
  - Notebooks
  - SLOs
  - Grail buckets
  - Settings objects
  - Extension monitoring configurations

Array input (bulk apply):
  Files containing an array of resources (e.g., from 'dtctl get settings --schema ...
  -o yaml') are applied element-by-element. Partial failures do not abort the batch;
  successful results are printed and a summary error reports any failures.

Examples:
  # Create a new dashboard and stamp the ID back into the file
  dtctl apply -f dashboard.yaml --write-id

  # Update existing dashboard (file exported with 'get' command includes ID)
  dtctl get dashboard my-dash -o yaml > dashboard.yaml
  # Edit dashboard.yaml...
  dtctl apply -f dashboard.yaml  # Updates the existing dashboard

  # Forgot --write-id on first run? Recover without creating a duplicate:
  dtctl apply -f dashboard.yaml --write-id --id <id-from-first-run>

  # CI/scripting: apply template to a known target resource
  dtctl apply -f template.yaml --id $DASHBOARD_ID

  # Update a settings object
  dtctl get settings <objectId> -o yaml > setting.yaml
  # Edit setting.yaml (modify the 'value' field)...
  dtctl apply -f setting.yaml  # Updates the existing setting

  # Bulk update settings (round-trip from get --schema)
  dtctl get settings --schema builtin:rum.web.enablement -o yaml > rum-settings.yaml
  # Edit rum-settings.yaml (modify values for specific applications)...
  dtctl apply -f rum-settings.yaml  # Updates all settings in the file

  # Apply with template variables
  dtctl apply -f dashboard.yaml --set environment=prod --set owner=team-a

  # Preview changes before applying
  dtctl apply -f notebook.yaml --dry-run

  # See what changed when updating
  dtctl apply -f dashboard.yaml --show-diff

  # Apply and get JSON output (for scripting/CI)
  dtctl apply -f dashboard.yaml -o json

  # Apply and get YAML output
  dtctl apply -f workflow.yaml -o yaml

  # Apply with wide table (includes URL for dashboards/notebooks)
  dtctl apply -f notebook.yaml -o wide

Note: The 'create' command always creates new resources. Use 'apply' to keep
resources in sync with their file definitions.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		setFlags, _ := cmd.Flags().GetStringArray("set")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		showDiff, _ := cmd.Flags().GetBool("show-diff")
		noHooks, _ := cmd.Flags().GetBool("no-hooks")
		overrideID, _ := cmd.Flags().GetString("id")
		writeID, _ := cmd.Flags().GetBool("write-id")
		shareEnvironment, _ := cmd.Flags().GetString("share-environment")

		if err := validateShareEnvironmentValue(shareEnvironment); err != nil {
			return err
		}

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

		// Create applier with safety checker (safety checks happen inside applier
		// with proper ownership determination for updates)
		applier := apply.NewApplier(c)
		if !dryRun {
			checker, err := NewSafetyChecker(cfg)
			if err != nil {
				return err
			}
			applier = applier.WithSafetyChecker(checker)
		}

		// Configure pre-apply and post-apply hooks
		if !noHooks {
			if hookCmd := cfg.GetPreApplyHook(); hookCmd != "" {
				applier = applier.WithPreApplyHook(hookCmd).WithSourceFile(file)
			}
			if hookCmd := cfg.GetPostApplyHook(); hookCmd != "" {
				applier = applier.WithPostApplyHook(hookCmd).WithSourceFile(file)
			}
			// Hook output (stdout and stderr) always goes to stderr so that
			// stdout carries only the structured result — JSON, YAML, or table
			// — regardless of output mode, and regardless of hook success or failure.
			applier = applier.WithHookOutputs(os.Stderr, os.Stderr)
		}

		// Apply the resource
		opts := apply.ApplyOptions{
			TemplateVars: templateVars,
			DryRun:       dryRun,
			ShowDiff:     showDiff,
			OverrideID:   overrideID,
			WriteID:      writeID,
		}

		results, applyErr := applier.Apply(fileData, opts)

		// For ListApplyError (partial batch failure), we still want to print
		// the successful results before returning the error.
		if applyErr != nil && len(results) == 0 {
			return applyErr
		}

		// Run environment sharing before printing so per-document "Shared X" stderr
		// lines appear adjacent to the apply output. Errors are collected rather than
		// returned immediately so the user always sees the apply results.
		var shareErr error
		if shareEnvironment != "" && !dryRun {
			shareErr = ensureEnvironmentShareForResults(c, results, shareEnvironment)
		}

		// Print structured output using the global -o flag.
		// The concrete type (DashboardApplyResult, WorkflowApplyResult, DryRunResult, etc.)
		// determines which columns/fields appear in the output.
		printer := NewPrinter()

		// Enrich agent output with apply-specific context
		resourceType := ""
		if base := extractApplyBase(results[0]); base != nil {
			resourceType = base.ResourceType
		}
		if ap := enrichAgent(printer, "apply", resourceType); ap != nil {
			ap.SetTotal(len(results))
			suggestions := buildApplySuggestions(results)
			ap.SetSuggestions(suggestions)
			// Forward any warnings from apply results
			var warnings []string
			for _, r := range results {
				if base := extractApplyBase(r); base != nil && len(base.Warnings) > 0 {
					warnings = append(warnings, base.Warnings...)
				}
			}
			if len(warnings) > 0 {
				ap.SetWarnings(warnings)
			}
		}

		if len(results) == 1 {
			if err := printer.Print(results[0]); err != nil {
				return err
			}
		} else {
			// Multiple results (e.g., connection list apply) — use list output
			items := make([]interface{}, len(results))
			for i, r := range results {
				items[i] = r
			}
			if err := printer.PrintList(items); err != nil {
				return err
			}
		}
		if shareErr != nil {
			return fmt.Errorf("apply succeeded but environment share failed: %w", shareErr)
		}
		return applyErr
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringP("file", "f", "", "file containing resource definition (required)")
	applyCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	applyCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	applyCmd.Flags().Bool("show-diff", false, "show diff of changes when updating existing resources")
	applyCmd.Flags().Bool("no-hooks", false, "skip pre-apply and post-apply hooks")
	applyCmd.Flags().String("id", "", "override or inject resource ID (use with --write-id to stamp ID into file)")
	applyCmd.Flags().Bool("write-id", false, "write the created resource ID back into the source file for idempotent future applies")
	applyCmd.Flags().String("share-environment", "", "share the applied notebook/dashboard with everyone in the environment (values: 'read' or 'read-write'; bare --share-environment defaults to 'read')")
	applyCmd.Flags().Lookup("share-environment").NoOptDefVal = "read"

	_ = applyCmd.MarkFlagRequired("file")
}

// validateShareEnvironmentValue rejects any --share-environment value outside
// the empty string, "read", or "read-write".
func validateShareEnvironmentValue(v string) error {
	switch v {
	case "", "read", "read-write":
		return nil
	default:
		return fmt.Errorf("invalid --share-environment value %q, must be 'read' or 'read-write'", v)
	}
}

// ensureEnvironmentShareForResults walks apply results and creates an environment share for every notebook/dashboard.
// Other resource types are silently skipped — environment shares only apply to documents.
//
// Per-document failures do not abort the walk: we attempt a share for every eligible
// result and return a combined error at the end so multi-document applies are partially
// successful when possible.
func ensureEnvironmentShareForResults(c *client.Client, results []apply.ApplyResult, access string) error {
	handler := document.NewHandler(c)
	var errs []error
	for _, r := range results {
		base := extractApplyBase(r)
		if base == nil {
			continue
		}
		if base.ResourceType != "notebook" && base.ResourceType != "dashboard" {
			continue
		}
		if base.ID == "" {
			continue
		}
		if _, err := handler.EnsureEnvironmentShare(base.ID, access); err != nil {
			output.PrintWarning("failed to share %s %q with environment: %v", base.ResourceType, base.ID, err)
			errs = append(errs, fmt.Errorf("document %q: %w", base.ID, err))
			continue
		}
		output.PrintInfo("Shared %s %q with environment (%s)", base.ResourceType, base.ID, access)
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	ids := make([]string, 0, len(errs))
	for _, e := range errs {
		ids = append(ids, e.Error())
	}
	return fmt.Errorf("%d documents failed to share: %s", len(errs), strings.Join(ids, "; "))
}
