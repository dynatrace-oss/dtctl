package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// shareCmd represents the share command
var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share documents with users, groups, or the environment",
	Long:  `Share documents (dashboards, notebooks, launchpads, ...) with specific users or groups, or with everyone in the environment.`,
}

// shareDocumentCmd shares a document with users/groups or the environment
var shareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> --user <user-id> | --group <group-id> | --environment",
	Short: "Share a document with users, groups, or the environment",
	Long: `Share a document of any type with specific users or groups, or with
everyone in the environment.

--environment does what "Share with environment" does in the web UI: it
creates an environment share at the --access level and marks the document
public. Re-running it with a different --access replaces the environment
share. It cannot be combined with --user or --group.

Examples:
  # Share a document with a user (read access)
  dtctl share document my-dashboard-id --user user-sso-id

  # Share with read-write access
  dtctl share document my-dashboard-id --user user-sso-id --access read-write

  # Share with multiple users
  dtctl share document my-dashboard-id --user user1 --user user2

  # Share with a group
  dtctl share document my-dashboard-id --group group-sso-id

  # Share with both users and groups
  dtctl share document my-dashboard-id --user user1 --group group1

  # Share without notifying the recipients (sharing with a group notifies
  # every member of the group by default)
  dtctl share document my-dashboard-id --group group-sso-id --no-notify

  # Share a launchpad (or any other document) with everyone in the environment
  dtctl share document my-launchpad-id --environment

  # Give everyone in the environment read-write access
  dtctl share document my-launchpad-id --environment --access read-write
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		access, _ := cmd.Flags().GetString("access")
		noNotify, _ := cmd.Flags().GetBool("no-notify")
		environment, _ := cmd.Flags().GetBool("environment")

		if environment {
			if len(users) > 0 || len(groups) > 0 {
				return fmt.Errorf("--environment cannot be combined with --user or --group")
			}
			if noNotify {
				return fmt.Errorf("--no-notify applies to --user/--group shares only, not to --environment")
			}
		} else if len(users) == 0 && len(groups) == 0 {
			return fmt.Errorf("at least one --user, --group, or --environment is required")
		}

		// Validate access level
		if access != "read" && access != "read-write" {
			return fmt.Errorf("invalid access level %q, must be 'read' or 'read-write'", access)
		}

		if environment && dryRun {
			return newDryRunReport(cmd).
				Linef("Dry run: would share document %q with the environment (%s access)", documentID, access).
				Detail("document", "%s", documentID).
				Detail("access", "%s", access).
				Detail("environment", "%t", true).
				Print()
		}

		// Build recipients list
		var recipients []document.SsoEntity
		for _, u := range users {
			recipients = append(recipients, document.SsoEntity{ID: u, Type: "user"})
		}
		for _, g := range groups {
			recipients = append(recipients, document.SsoEntity{ID: g, Type: "group"})
		}

		if dryRun {
			report := newDryRunReport(cmd).
				Linef("Dry run: would share document %q with %d recipient(s) (%s access)",
					documentID, len(recipients), access).
				Detail("document", "%s", documentID).
				Detail("access", "%s", access).
				Detail("recipients", "%d", len(recipients)).
				Detail("notify", "%t", !noNotify)
			for _, r := range recipients {
				report.Linef("  - %s: %s", r.Type, r.ID)
			}
			return report.Print()
		}

		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get document metadata for ownership check
		metadata, err := handler.GetMetadata(documentID)
		if err != nil {
			return err
		}

		// Safety check with actual ownership - sharing modifies document permissions
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)
		if err := CheckSafety(cfg, safety.OperationUpdate, ownership); err != nil {
			return err
		}

		if environment {
			share, err := handler.EnsureEnvironmentShare(documentID, access)
			if err != nil {
				return fmt.Errorf("failed to share document %q with the environment: %w", documentID, err)
			}
			output.PrintSuccess("Shared document %q with the environment (%s access, share %s)",
				documentID, access, share.ID)
			return nil
		}

		// Check if a share already exists for this document with the same access level
		shares, err := handler.ListDirectShares(documentID)
		if err != nil {
			return fmt.Errorf("failed to check existing shares: %w", err)
		}

		var existingShare *document.DirectShare
		for _, s := range shares.Shares {
			if s.ExactAccess(access) {
				existingShare = &s
				break
			}
		}

		if existingShare != nil {
			// Add recipients to existing share
			err = handler.AddDirectShareRecipientsWithOptions(existingShare.ID, recipients,
				document.AddDirectShareRecipientsOptions{SuppressNotification: noNotify})
			if err != nil {
				return fmt.Errorf("failed to add recipients to share: %w", err)
			}
			output.PrintSuccess("Added %d recipient(s) to existing %s share for document %q",
				len(recipients), access, documentID)
		} else {
			// Create new share
			share, err := handler.CreateDirectShare(document.CreateDirectShareRequest{
				DocumentID:           documentID,
				Access:               access,
				Recipients:           recipients,
				SuppressNotification: noNotify,
			})
			if err != nil {
				return fmt.Errorf("failed to create share: %w", err)
			}
			output.PrintSuccess("Created %s share (%s) for document %q with %d recipient(s)",
				access, share.ID, documentID, len(recipients))
		}

		return nil
	},
}

// unshareCmd represents the unshare command
var unshareCmd = &cobra.Command{
	Use:   "unshare",
	Short: "Remove sharing from documents",
	Long:  `Remove sharing from documents (dashboards, notebooks, launchpads, ...).`,
}

// unshareDocumentCmd removes sharing from a document
var unshareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> [--user <user-id>] [--group <group-id>] [--all] [--environment]",
	Short: "Remove sharing from a document",
	Long: `Remove sharing from a document. Can remove specific users/groups, all
user/group shares, or the environment share.

--all removes user and group shares only. --environment removes the
environment share and marks the document private again; it can be combined
with the other flags.

Examples:
  # Remove a specific user from all shares
  dtctl unshare document my-dashboard-id --user user-sso-id

  # Remove a group from all shares
  dtctl unshare document my-dashboard-id --group group-sso-id

  # Remove all shares (revoke all access)
  dtctl unshare document my-dashboard-id --all

  # Remove only read shares
  dtctl unshare document my-dashboard-id --all --access read

  # Stop sharing a document with the environment (makes it private again)
  dtctl unshare document my-launchpad-id --environment

  # Remove every share: users, groups, and the environment
  dtctl unshare document my-launchpad-id --all --environment
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		all, _ := cmd.Flags().GetBool("all")
		access, _ := cmd.Flags().GetString("access")
		environment, _ := cmd.Flags().GetBool("environment")
		direct := all || len(users) > 0 || len(groups) > 0

		if !direct && !environment {
			return fmt.Errorf("specify --user, --group, --all, or --environment")
		}
		// An unrecognized level would otherwise be matched as 'read'.
		if access != "" && access != "read" && access != "read-write" {
			return fmt.Errorf("invalid access level %q, must be 'read' or 'read-write'", access)
		}

		if dryRun {
			report := newDryRunReport(cmd).Detail("document", "%s", documentID)
			if all {
				report.Linef("Dry run: would remove all shares from document %q", documentID)
			} else if direct {
				report.
					Linef("Dry run: would remove %d user(s) and %d group(s) from document %q shares",
						len(users), len(groups), documentID).
					Detail("users", "%d", len(users)).
					Detail("groups", "%d", len(groups))
			}
			if environment {
				report.
					Linef("Dry run: would stop sharing document %q with the environment", documentID).
					Detail("environment", "%t", true)
			}
			return report.Print()
		}

		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get document metadata for ownership check
		metadata, err := handler.GetMetadata(documentID)
		if err != nil {
			return err
		}

		// Safety check with actual ownership - unsharing modifies document permissions
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)
		if err := CheckSafety(cfg, safety.OperationUpdate, ownership); err != nil {
			return err
		}

		if environment {
			deleted, madePrivate, err := handler.RemoveEnvironmentShares(documentID, access)
			if err != nil {
				return fmt.Errorf("failed to stop sharing document %q with the environment: %w", documentID, err)
			}
			if deleted > 0 || madePrivate {
				output.PrintSuccess("Stopped sharing document %q with the environment (%d environment share(s) removed)",
					documentID, deleted)
			} else if access != "" {
				fmt.Printf("No %s environment share found for document %q\n", access, documentID)
			} else {
				fmt.Printf("Document %q is not shared with the environment\n", documentID)
			}
			if !direct {
				return nil
			}
		}

		// Get existing shares for this document
		shares, err := handler.ListDirectShares(documentID)
		if err != nil {
			return fmt.Errorf("failed to list shares: %w", err)
		}

		if len(shares.Shares) == 0 {
			fmt.Printf("No shares found for document %q\n", documentID)
			return nil
		}

		if all {
			// Delete all shares (optionally filtered by access level)
			deleted := 0
			for _, share := range shares.Shares {
				if access != "" && !share.ExactAccess(access) {
					continue
				}
				if err := handler.DeleteDirectShare(share.ID); err != nil {
					return fmt.Errorf("failed to delete share %s: %w", share.ID, err)
				}
				deleted++
			}
			output.PrintSuccess("Deleted %d share(s) from document %q", deleted, documentID)
		} else {
			// Remove specific recipients from all shares
			recipientIDs := append(users, groups...)
			removed := 0
			for _, share := range shares.Shares {
				if access != "" && !share.ExactAccess(access) {
					continue
				}
				if err := handler.RemoveDirectShareRecipients(share.ID, recipientIDs); err != nil {
					// Log but continue - recipient might not be in this share
					if verbosity > 0 {
						output.PrintInfo("Note: could not remove from share %s: %v", share.ID, err)
					}
					continue
				}
				removed++
			}
			output.PrintSuccess("Removed %d recipient(s) from %d share(s) of document %q",
				len(recipientIDs), removed, documentID)
		}

		return nil
	},
}

// shareNotebookCmd is an alias for sharing notebooks
var shareNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id> --user <user-id> | --group <group-id> | --environment",
	Short:   "Share a notebook with users, groups, or the environment",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// shareDashboardCmd is an alias for sharing dashboards
var shareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> --user <user-id> | --group <group-id> | --environment",
	Short:   "Share a dashboard with users, groups, or the environment",
	Aliases: []string{"db"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// unshareNotebookCmd is an alias for unsharing notebooks
var unshareNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id> [--user <user-id>] [--group <group-id>] [--all] [--environment]",
	Short:   "Remove sharing from a notebook",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    unshareDocumentCmd.RunE,
}

// unshareDashboardCmd is an alias for unsharing dashboards
var unshareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> [--user <user-id>] [--group <group-id>] [--all] [--environment]",
	Short:   "Remove sharing from a dashboard",
	Aliases: []string{"db"},
	Args:    cobra.ExactArgs(1),
	RunE:    unshareDocumentCmd.RunE,
}

func init() {
	rootCmd.AddCommand(shareCmd)
	rootCmd.AddCommand(unshareCmd)

	// Share subcommands
	shareCmd.AddCommand(shareDocumentCmd)
	shareCmd.AddCommand(shareNotebookCmd)
	shareCmd.AddCommand(shareDashboardCmd)

	// Unshare subcommands
	unshareCmd.AddCommand(unshareDocumentCmd)
	unshareCmd.AddCommand(unshareNotebookCmd)
	unshareCmd.AddCommand(unshareDashboardCmd)

	// Share flags (apply to all share subcommands)
	for _, cmd := range []*cobra.Command{shareDocumentCmd, shareNotebookCmd, shareDashboardCmd} {
		cmd.Flags().StringArray("user", []string{}, "SSO user ID to share with (can be specified multiple times)")
		cmd.Flags().StringArray("group", []string{}, "SSO group ID to share with (can be specified multiple times)")
		cmd.Flags().String("access", "read", "access level: 'read' or 'read-write'")
		cmd.Flags().Bool("no-notify", false, "do not notify recipients of the share (a group recipient notifies every member)")
		cmd.Flags().Bool("environment", false, "share with everyone in the environment and mark the document public (cannot be combined with --user/--group)")
		// A blank recipient would be sent to the API as an empty SSO ID.
		rejectEmptyFlag(cmd, "user")
		rejectEmptyFlag(cmd, "group")
	}

	// Unshare flags (apply to all unshare subcommands)
	for _, cmd := range []*cobra.Command{unshareDocumentCmd, unshareNotebookCmd, unshareDashboardCmd} {
		cmd.Flags().StringArray("user", []string{}, "SSO user ID to remove (can be specified multiple times)")
		cmd.Flags().StringArray("group", []string{}, "SSO group ID to remove (can be specified multiple times)")
		cmd.Flags().Bool("all", false, "remove all user and group shares (add --environment to remove the environment share too)")
		cmd.Flags().String("access", "", "filter by access level: 'read' or 'read-write'")
		cmd.Flags().Bool("environment", false, "stop sharing with the environment and mark the document private again")
		rejectEmptyFlag(cmd, "user")
		rejectEmptyFlag(cmd, "group")
	}
}

// formatRecipients formats recipients for display
//
//nolint:unused // Reserved for future share features
func formatRecipients(recipients []document.SsoEntity) string {
	var parts []string
	for _, r := range recipients {
		parts = append(parts, fmt.Sprintf("%s:%s", r.Type, r.ID))
	}
	return strings.Join(parts, ", ")
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(shareCmd)
	stability.MarkStable(shareDashboardCmd)
	stability.MarkStable(shareDocumentCmd)
	stability.MarkStable(shareNotebookCmd)
	stability.MarkStable(unshareCmd)
	stability.MarkStable(unshareDashboardCmd)
	stability.MarkStable(unshareDocumentCmd)
	stability.MarkStable(unshareNotebookCmd)

	for _, cmd := range []*cobra.Command{shareDocumentCmd, shareNotebookCmd, shareDashboardCmd} {
		stability.MarkFlag(cmd, "no-notify", stability.Experimental, "0.40.0")
		stability.MarkFlag(cmd, "environment", stability.Experimental, "0.42.0")
	}
	for _, cmd := range []*cobra.Command{unshareDocumentCmd, unshareNotebookCmd, unshareDashboardCmd} {
		stability.MarkFlag(cmd, "environment", stability.Experimental, "0.42.0")
	}

	// New: may still change shape (output fields, fallback order) -- see
	// AGENTS.md "Stability Tiers".
}
