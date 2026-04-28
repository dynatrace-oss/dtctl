package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// getSegmentsCmd retrieves Grail filter segments
var getSegmentsCmd = &cobra.Command{
	Use:     "segments [uid]",
	Aliases: []string{"segment", "seg", "filter-segments", "filter-segment"},
	Short:   "Get Grail filter segments",
	Long: `Get Grail filter segments.

Examples:
  # List all segments
  dtctl get segments

  # Get a specific segment by UID
  dtctl get segment <uid>

  # Output as JSON
  dtctl get segments -o json

  # Wide output with description and owner
  dtctl get segments -o wide
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := segment.NewHandler(c)

		// Get specific segment if UID provided
		if len(args) > 0 {
			seg, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(seg)
		}

		// List all segments
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list.FilterSegments)
	},
}

// deleteSegmentCmd deletes a filter segment
var deleteSegmentCmd = &cobra.Command{
	Use:     "segment <uid>",
	Aliases: []string{"segments", "seg", "filter-segment", "filter-segments"},
	Short:   "Delete a Grail filter segment",
	Long: `Delete a Grail filter segment by UID.

Examples:
  # Delete a segment (requires typing the UID to confirm)
  dtctl delete segment <uid>

  # Delete with confirmation flag (non-interactive)
  dtctl delete segment <uid> --confirm=<uid>

  # Delete without confirmation (use with caution)
  dtctl delete segment <uid> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		uid := args[0]

		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := segment.NewHandler(c)

		// Verify segment exists before prompting for confirmation
		seg, err := handler.Get(uid)
		if err != nil {
			return err
		}

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(seg.Owner, currentUserID)
		if err := checker.CheckError(safety.OperationDelete, ownership); err != nil {
			return err
		}

		// Handle confirmation
		displayName := seg.Name
		if displayName == "" {
			displayName = uid
		}

		confirmFlag, _ := cmd.Flags().GetString("confirm")
		if !forceDelete && !plainMode {
			if confirmFlag != "" {
				if !prompt.ValidateConfirmFlag(confirmFlag, uid) {
					return fmt.Errorf("confirmation value %q does not match segment UID %q", confirmFlag, uid)
				}
			} else {
				if !prompt.ConfirmDataDeletion("segment", displayName) {
					fmt.Println("Deletion cancelled")
					return nil
				}
			}
		}

		if err := handler.Delete(uid); err != nil {
			return err
		}

		output.PrintSuccess("Segment %q deleted", displayName)
		return nil
	},
}

func init() {
	// Delete confirmation flags
	deleteSegmentCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteSegmentCmd.Flags().String("confirm", "", "Confirm deletion by providing the segment UID (for non-interactive use)")
}
