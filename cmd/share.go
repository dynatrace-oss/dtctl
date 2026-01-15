package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// shareCmd represents the share command
var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share documents with users or groups",
	Long:  `Share documents (dashboards, notebooks) with specific users or groups.`,
}

// shareDocumentCmd shares a document with users/groups
var shareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> --user <user-id> | --group <group-id>",
	Short: "Share a document with users or groups",
	Long: `Share a document with specific users or groups.

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
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		access, _ := cmd.Flags().GetString("access")

		if len(users) == 0 && len(groups) == 0 {
			return fmt.Errorf("at least one --user or --group is required")
		}

		// Validate access level
		if access != "read" && access != "read-write" {
			return fmt.Errorf("invalid access level %q, must be 'read' or 'read-write'", access)
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
			fmt.Printf("Dry run: would share document %q with %d recipient(s) (%s access)\n",
				documentID, len(recipients), access)
			for _, r := range recipients {
				fmt.Printf("  - %s: %s\n", r.Type, r.ID)
			}
			return nil
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - sharing modifies document permissions
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}
		if checker.IsOverridden() {
			fmt.Fprintln(os.Stderr, "⚠️ ", checker.OverrideWarning(safety.OperationUpdate))
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Check if a share already exists for this document with the same access level
		shares, err := handler.ListDirectShares(documentID)
		if err != nil {
			return fmt.Errorf("failed to check existing shares: %w", err)
		}

		var existingShare *document.DirectShare
		for _, s := range shares.Shares {
			if s.Access == access {
				existingShare = &s
				break
			}
		}

		if existingShare != nil {
			// Add recipients to existing share
			err = handler.AddDirectShareRecipients(existingShare.ID, recipients)
			if err != nil {
				return fmt.Errorf("failed to add recipients to share: %w", err)
			}
			fmt.Printf("Added %d recipient(s) to existing %s share for document %q\n",
				len(recipients), access, documentID)
		} else {
			// Create new share
			share, err := handler.CreateDirectShare(document.CreateDirectShareRequest{
				DocumentID: documentID,
				Access:     access,
				Recipients: recipients,
			})
			if err != nil {
				return fmt.Errorf("failed to create share: %w", err)
			}
			fmt.Printf("Created %s share (%s) for document %q with %d recipient(s)\n",
				access, share.ID, documentID, len(recipients))
		}

		return nil
	},
}

// unshareCmd represents the unshare command
var unshareCmd = &cobra.Command{
	Use:   "unshare",
	Short: "Remove sharing from documents",
	Long:  `Remove sharing from documents (dashboards, notebooks).`,
}

// unshareDocumentCmd removes sharing from a document
var unshareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> [--user <user-id>] [--group <group-id>] [--all]",
	Short: "Remove sharing from a document",
	Long: `Remove sharing from a document. Can remove specific users/groups or all shares.

Examples:
  # Remove a specific user from all shares
  dtctl unshare document my-dashboard-id --user user-sso-id

  # Remove a group from all shares
  dtctl unshare document my-dashboard-id --group group-sso-id

  # Remove all shares (revoke all access)
  dtctl unshare document my-dashboard-id --all

  # Remove only read shares
  dtctl unshare document my-dashboard-id --all --access read
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		all, _ := cmd.Flags().GetBool("all")
		access, _ := cmd.Flags().GetString("access")

		if !all && len(users) == 0 && len(groups) == 0 {
			return fmt.Errorf("specify --user, --group, or --all")
		}

		if dryRun {
			if all {
				fmt.Printf("Dry run: would remove all shares from document %q\n", documentID)
			} else {
				fmt.Printf("Dry run: would remove %d user(s) and %d group(s) from document %q shares\n",
					len(users), len(groups), documentID)
			}
			return nil
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - unsharing modifies document permissions
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}
		if checker.IsOverridden() {
			fmt.Fprintln(os.Stderr, "⚠️ ", checker.OverrideWarning(safety.OperationUpdate))
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

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
				if access != "" && share.Access != access {
					continue
				}
				if err := handler.DeleteDirectShare(share.ID); err != nil {
					return fmt.Errorf("failed to delete share %s: %w", share.ID, err)
				}
				deleted++
			}
			fmt.Printf("Deleted %d share(s) from document %q\n", deleted, documentID)
		} else {
			// Remove specific recipients from all shares
			recipientIDs := append(users, groups...)
			removed := 0
			for _, share := range shares.Shares {
				if access != "" && share.Access != access {
					continue
				}
				if err := handler.RemoveDirectShareRecipients(share.ID, recipientIDs); err != nil {
					// Log but continue - recipient might not be in this share
					if verbosity > 0 {
						fmt.Printf("Note: could not remove from share %s: %v\n", share.ID, err)
					}
					continue
				}
				removed++
			}
			fmt.Printf("Removed %d recipient(s) from %d share(s) of document %q\n",
				len(recipientIDs), removed, documentID)
		}

		return nil
	},
}

// shareNotebookCmd is an alias for sharing notebooks
var shareNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id> --user <user-id> | --group <group-id>",
	Short:   "Share a notebook with users or groups",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// shareDashboardCmd is an alias for sharing dashboards
var shareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> --user <user-id> | --group <group-id>",
	Short:   "Share a dashboard with users or groups",
	Aliases: []string{"db"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// unshareNotebookCmd is an alias for unsharing notebooks
var unshareNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id> [--user <user-id>] [--group <group-id>] [--all]",
	Short:   "Remove sharing from a notebook",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    unshareDocumentCmd.RunE,
}

// unshareDashboardCmd is an alias for unsharing dashboards
var unshareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> [--user <user-id>] [--group <group-id>] [--all]",
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
	}

	// Unshare flags (apply to all unshare subcommands)
	for _, cmd := range []*cobra.Command{unshareDocumentCmd, unshareNotebookCmd, unshareDashboardCmd} {
		cmd.Flags().StringArray("user", []string{}, "SSO user ID to remove (can be specified multiple times)")
		cmd.Flags().StringArray("group", []string{}, "SSO group ID to remove (can be specified multiple times)")
		cmd.Flags().Bool("all", false, "remove all shares")
		cmd.Flags().String("access", "", "filter by access level: 'read' or 'read-write'")
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
