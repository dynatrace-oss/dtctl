package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/anomalydetector"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// getAnomalyDetectorsCmd retrieves anomaly detectors
var getAnomalyDetectorsCmd = &cobra.Command{
	Use:     "anomaly-detectors [id]",
	Aliases: []string{"anomaly-detector", "ad"},
	Short:   "Get custom anomaly detectors",
	Long: `Get custom anomaly detectors (builtin:davis.anomaly-detectors).

Examples:
  # List all anomaly detectors
  dtctl get anomaly-detectors

  # List only enabled detectors
  dtctl get anomaly-detectors --enabled

  # List only disabled detectors
  dtctl get anomaly-detectors --enabled=false

  # Get a specific detector by object ID
  dtctl get anomaly-detector <object-id>

  # Output as JSON
  dtctl get anomaly-detectors -o json

  # Wide output with object IDs
  dtctl get anomaly-detectors -o wide
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := anomalydetector.NewHandler(c)

		// Get specific detector if ID provided
		if len(args) > 0 {
			ad, err := resolveAnomalyDetector(handler, args[0])
			if err != nil {
				return err
			}

			ap := enrichAgent(printer, "get", "anomaly-detector")
			if ap != nil {
				ap.SetSuggestions([]string{
					fmt.Sprintf("dtctl describe anomaly-detector %s -- view full configuration and recent problems", ad.ObjectID),
					fmt.Sprintf("dtctl edit anomaly-detector %s -- modify detector configuration", ad.ObjectID),
					"dtctl get anomaly-detectors -- list all detectors",
				})
			}
			return printer.Print(ad)
		}

		// List all detectors
		opts := anomalydetector.ListOptions{}

		// Handle tri-state --enabled flag
		if cmd.Flags().Changed("enabled") {
			enabled, _ := cmd.Flags().GetBool("enabled")
			opts.Enabled = &enabled
		}

		detectors, err := handler.List(opts)
		if err != nil {
			return err
		}

		ap := enrichAgent(printer, "get", "anomaly-detector")
		if ap != nil {
			ap.SetTotal(len(detectors))
			ap.Context().Suggestions = []string{
				"dtctl describe anomaly-detector <title> -- view full configuration and recent problems",
				"dtctl get anomaly-detectors --enabled -- list only active detectors",
				"dtctl edit anomaly-detector <title> -- modify detector configuration",
			}
		}
		return printer.PrintList(detectors)
	},
}

// deleteAnomalyDetectorCmd deletes an anomaly detector
var deleteAnomalyDetectorCmd = &cobra.Command{
	Use:     "anomaly-detector <id-or-title>",
	Aliases: []string{"ad"},
	Short:   "Delete a custom anomaly detector",
	Long: `Delete a custom anomaly detector by object ID or title.

Examples:
  # Delete by object ID
  dtctl delete anomaly-detector <object-id>

  # Delete by title
  dtctl delete anomaly-detector "High CPU on production hosts"

  # Delete without confirmation
  dtctl delete anomaly-detector <object-id> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		_, c, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := anomalydetector.NewHandler(c)

		// Resolve identifier (could be objectID or title)
		ad, err := resolveAnomalyDetector(handler, identifier)
		if err != nil {
			return err
		}

		// Confirm deletion unless --yes or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("anomaly detector", ad.Title, ad.ObjectID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(ad.ObjectID); err != nil {
			return err
		}

		// In agent mode, output structured response
		if agentMode {
			printer := NewPrinter()
			ap := enrichAgent(printer, "delete", "anomaly-detector")
			if ap != nil {
				ap.SetSuggestions([]string{
					"Deleted. Verify with 'dtctl get anomaly-detectors'",
				})
			}
			return printer.Print(map[string]string{
				"objectId": ad.ObjectID,
				"title":    ad.Title,
				"status":   "deleted",
			})
		}

		output.PrintSuccess("Anomaly detector %q deleted", ad.Title)
		return nil
	},
}

func init() {
	// --enabled flag: tri-state (absent=all, --enabled=true, --enabled=false)
	getAnomalyDetectorsCmd.Flags().Bool("enabled", true, "Filter by enabled state (--enabled for enabled only, --enabled=false for disabled only)")

	// Delete confirmation flags
	deleteAnomalyDetectorCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")

	// Suppress the default value in help output for the tri-state flag
	_ = getAnomalyDetectorsCmd.Flags().SetAnnotation("enabled", "cobra_annotation_bash_completion_custom", []string{})
}

// resolveAnomalyDetector tries to find a detector by ID or title, used by describe/edit/delete commands.
func resolveAnomalyDetector(handler *anomalydetector.Handler, identifier string) (*anomalydetector.AnomalyDetector, error) {
	// Try by object ID first
	ad, err := handler.Get(identifier)
	if err == nil {
		return ad, nil
	}
	// A 403 means the detector may well exist — reporting it as missing sends
	// the user looking for the wrong problem and drops the permission
	// diagnostics the handler attached.
	if isPermissionDenied(err) {
		return nil, err
	}

	// Fall back to title match
	ad, findErr := handler.FindByName(identifier)
	if findErr == nil {
		return ad, nil
	}
	if isPermissionDenied(findErr) {
		return nil, findErr
	}

	return nil, fmt.Errorf("anomaly detector %q not found (run 'dtctl get anomaly-detectors' to list available detectors)", identifier)
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(deleteAnomalyDetectorCmd)
	stability.MarkStable(getAnomalyDetectorsCmd)
}
