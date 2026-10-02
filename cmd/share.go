package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// The two ways --environment opens a document to everyone in the environment.
// The Document API keeps them apart, so each mode does (and undoes) only its
// own half.
const (
	// environmentLink is an environment share: a #share=<id> link that anyone
	// in the environment can claim, at read or read-write access.
	environmentLink = "link"
	// environmentPublic is isPrivate=false: everyone in the environment can
	// find and read the document, with no share object involved.
	environmentPublic = "public"
)

// environmentMode reads --environment: "" when it was not given, otherwise
// "link" or "public". The flag deliberately has no default value: with one,
// pflag would read `--environment link` as a bare --environment followed by a
// second positional argument "link".
func environmentMode(cmd *cobra.Command) (string, error) {
	mode, _ := cmd.Flags().GetString("environment")
	if !cmd.Flags().Changed("environment") {
		return "", nil
	}
	if mode != environmentLink && mode != environmentPublic {
		return "", fmt.Errorf("invalid --environment %q, must be 'link' or 'public'", mode)
	}
	return mode, nil
}

// environmentLinkResult is what `share --environment link` reports.
type environmentLinkResult struct {
	DocumentID string `json:"documentId" table:"DOCUMENT_ID"`
	ShareID    string `json:"shareId" table:"SHARE_ID"`
	Access     string `json:"access" table:"ACCESS"`
	Created    bool   `json:"created" table:"CREATED"`
	URL        string `json:"url" table:"URL"`
}

// environmentPublicResult is what `share --environment public` reports.
type environmentPublicResult struct {
	DocumentID string `json:"documentId" table:"DOCUMENT_ID"`
	Public     bool   `json:"public" table:"PUBLIC"`
	Changed    bool   `json:"changed" table:"CHANGED"`
}

// shareCmd represents the share command
var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share documents with users, groups, or the environment",
	Long:  `Share documents (dashboards, notebooks, launchpads, ...) with specific users or groups, or with everyone in the environment.`,
}

// shareDocumentCmd shares a document with users/groups or the environment
var shareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> --user <user-id> | --group <group-id> | --environment link|public",
	Short: "Share a document with users, groups, or the environment",
	Long: `Share a document of any type with specific users or groups, or with
everyone in the environment.

--environment opens the document to everyone in the environment, in one of
two ways. They are independent of each other and of --user/--group shares,
and --environment cannot be combined with --user or --group:

  link    Create an environment share and print its link
          (https://<environment>/ui/document/v0/#share=<id>). Anyone in the
          environment who opens the link gets --access to the document. The
          document's visibility is not changed. Re-running it reuses the
          existing share; an explicit --access at another level replaces it,
          which gives it a new link and breaks the old one.
  public  Mark the document public (isPrivate=false): everyone in the
          environment can find and read it. No share is created, and --access
          does not apply.

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

  # Create a link anyone in the environment can open to read a launchpad
  # (or any other document)
  dtctl share document my-launchpad-id --environment link

  # Create a link that grants read-write access
  dtctl share document my-launchpad-id --environment link --access read-write

  # Make a document readable by everyone in the environment
  dtctl share document my-launchpad-id --environment public
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		access, _ := cmd.Flags().GetString("access")
		noNotify, _ := cmd.Flags().GetBool("no-notify")
		environment, err := environmentMode(cmd)
		if err != nil {
			return err
		}

		if environment != "" {
			if len(users) > 0 || len(groups) > 0 {
				return fmt.Errorf("--environment cannot be combined with --user or --group")
			}
			if noNotify {
				return fmt.Errorf("--no-notify applies to --user/--group shares only, not to --environment")
			}
			if environment == environmentPublic && cmd.Flags().Changed("access") {
				return fmt.Errorf("--access does not apply to --environment public, which grants read access; " +
					"use --environment link --access read-write for a link with write access")
			}
		} else if len(users) == 0 && len(groups) == 0 {
			return fmt.Errorf("at least one --user, --group, or --environment link|public is required")
		}

		// Validate access level
		if access != "read" && access != "read-write" {
			return fmt.Errorf("invalid access level %q, must be 'read' or 'read-write'", access)
		}

		if environment != "" && dryRun {
			report := newDryRunReport(cmd).
				Detail("document", "%s", documentID).
				Detail("environment", "%s", environment)
			if environment == environmentLink {
				report.
					Linef("Dry run: would create an environment share link for document %q (%s access)", documentID, access).
					Detail("access", "%s", access)
			} else {
				report.Linef("Dry run: would make document %q public to everyone in the environment", documentID)
			}
			return report.Print()
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

		switch environment {
		case environmentLink:
			return shareEnvironmentLink(cmd, handler, c.BaseURL(), documentID, access)
		case environmentPublic:
			return shareEnvironmentPublic(cmd, handler, documentID)
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

// shareEnvironmentLink creates (or reuses) the document's environment share
// and prints its link: on stdout as a plain line, so it can be captured, or as
// the result in agent mode.
func shareEnvironmentLink(cmd *cobra.Command, handler *document.Handler, baseURL, documentID, access string) error {
	// Without an explicit --access, an existing share at another level is
	// kept: replacing it would break every link already handed out.
	res, err := handler.EnsureEnvironmentLink(documentID, access, cmd.Flags().Changed("access"))
	if err != nil {
		return fmt.Errorf("failed to create an environment share for document %q: %w", documentID, err)
	}

	result := environmentLinkResult{
		DocumentID: documentID,
		ShareID:    res.Share.ID,
		Access:     res.Share.Level(),
		Created:    res.Created,
		URL:        document.ShareURL(baseURL, res.Share.ID),
	}
	var warnings []string
	for _, old := range res.Replaced {
		warnings = append(warnings, fmt.Sprintf("replaced the %s environment share %s: links to it no longer work",
			old.Level(), old.ID))
	}
	if !res.Created && result.Access != access {
		warnings = append(warnings, fmt.Sprintf("kept the existing %s environment share; pass --access %s to replace it "+
			"(this breaks links to the existing share)", result.Access, access))
	}

	printer := NewPrinter()
	if ap := enrichAgent(printer, "share", cmd.Name()); ap != nil {
		ap.SetWarnings(warnings)
		return printer.Print(result)
	}
	for _, w := range warnings {
		output.PrintWarning("%s", w)
	}
	if res.Created {
		output.PrintSuccess("Created %s environment share %s for document %q", result.Access, result.ShareID, documentID)
	} else {
		output.PrintSuccess("Document %q already has a %s environment share (%s)", documentID, result.Access, result.ShareID)
	}
	fmt.Println(result.URL)
	return nil
}

// shareEnvironmentPublic marks the document public (isPrivate=false).
func shareEnvironmentPublic(cmd *cobra.Command, handler *document.Handler, documentID string) error {
	changed, err := handler.SetPrivate(documentID, false)
	if err != nil {
		return fmt.Errorf("failed to make document %q public: %w", documentID, err)
	}

	printer := NewPrinter()
	if enrichAgent(printer, "share", cmd.Name()) != nil {
		return printer.Print(environmentPublicResult{DocumentID: documentID, Public: true, Changed: changed})
	}
	if changed {
		output.PrintSuccess("Document %q is now public: everyone in the environment can find and read it", documentID)
	} else {
		fmt.Printf("Document %q is already public\n", documentID)
	}
	return nil
}

// environmentShareDeleteScopes are what `unshare --environment link` needs on
// top of the document scopes. OAuth sessions from before dtctl requested
// :delete at login do not carry it.
var environmentShareDeleteScopes = []string{"document:environment-shares:read", "document:environment-shares:delete"}

// unshareEnvironmentLinkError turns a 403 from listing or deleting environment
// shares into one that names the scopes and how to get them.
func unshareEnvironmentLinkError(err error, cfg *config.Config, documentID string) error {
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		return fmt.Errorf("failed to delete the environment share of document %q: %w", documentID, err)
	}
	return &diagnostic.Error{
		Operation:  fmt.Sprintf("delete the environment share of document %q", documentID),
		StatusCode: apiErr.StatusCode,
		Message:    "access denied: " + err.Error(),
		Err:        err,
		Suggestions: append(loginScopeAdvice(cfg, environmentShareDeleteScopes),
			"Platform token: create it with "+strings.Join(environmentShareDeleteScopes, ", "),
			"Check the scope gate: re-run the same command with --check-scopes"),
	}
}

// unshareCmd represents the unshare command
var unshareCmd = &cobra.Command{
	Use:   "unshare",
	Short: "Remove sharing from documents",
	Long:  `Remove sharing from documents (dashboards, notebooks, launchpads, ...).`,
}

// unshareDocumentCmd removes sharing from a document
var unshareDocumentCmd = &cobra.Command{
	Use:   "document <document-id> [--user <user-id>] [--group <group-id>] [--all] [--environment link|public]",
	Short: "Remove sharing from a document",
	Long: `Remove sharing from a document. Can remove specific users/groups, all
user/group shares, or what --environment link|public on share created.

--all removes user and group shares only. --environment undoes one of the two
ways a document is opened to the environment, and can be combined with the
other flags:

  link    Delete the document's environment share(s), so their links stop
          working. With --access, only shares at exactly that level are
          deleted. The document's visibility is not changed.
  public  Mark the document private again (isPrivate=true). Environment
          shares are not deleted.

Examples:
  # Remove a specific user from all shares
  dtctl unshare document my-dashboard-id --user user-sso-id

  # Remove a group from all shares
  dtctl unshare document my-dashboard-id --group group-sso-id

  # Remove all shares (revoke all access)
  dtctl unshare document my-dashboard-id --all

  # Remove only read shares
  dtctl unshare document my-dashboard-id --all --access read

  # Delete the document's environment share link
  dtctl unshare document my-launchpad-id --environment link

  # Take back write access but keep a read-only link working
  dtctl unshare document my-launchpad-id --environment link --access read-write

  # Make a public document private again
  dtctl unshare document my-launchpad-id --environment public

  # Remove every user, group, and environment share
  dtctl unshare document my-launchpad-id --all --environment link
`,
	Aliases: []string{"doc"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]
		users, _ := cmd.Flags().GetStringArray("user")
		groups, _ := cmd.Flags().GetStringArray("group")
		all, _ := cmd.Flags().GetBool("all")
		access, _ := cmd.Flags().GetString("access")
		direct := all || len(users) > 0 || len(groups) > 0
		environment, err := environmentMode(cmd)
		if err != nil {
			return err
		}

		if !direct && environment == "" {
			return fmt.Errorf("specify --user, --group, --all, or --environment link|public")
		}
		// An unrecognized level would otherwise be matched as 'read'.
		if access != "" && access != "read" && access != "read-write" {
			return fmt.Errorf("invalid access level %q, must be 'read' or 'read-write'", access)
		}
		// --access filters shares, and --environment public removes none.
		if environment == environmentPublic && access != "" && !direct {
			return fmt.Errorf("--access does not apply to --environment public, which deletes no share")
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
			switch environment {
			case environmentLink:
				which := "all environment shares"
				if access != "" {
					which = access + " environment shares"
				}
				report.
					Linef("Dry run: would delete %s of document %q", which, documentID).
					Detail("environment", "%s", environment)
			case environmentPublic:
				report.
					Linef("Dry run: would make document %q private", documentID).
					Detail("environment", "%s", environment)
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

		switch environment {
		case environmentLink:
			deleted, err := handler.RemoveEnvironmentLinks(documentID, access)
			if err != nil {
				return unshareEnvironmentLinkError(err, cfg, documentID)
			}
			switch {
			case deleted > 0:
				output.PrintSuccess("Deleted %d environment share(s) from document %q; links to them no longer work",
					deleted, documentID)
			case access != "":
				fmt.Printf("No %s environment share found for document %q, nothing changed\n", access, documentID)
			default:
				fmt.Printf("Document %q has no environment share, nothing changed\n", documentID)
			}
		case environmentPublic:
			changed, err := handler.SetPrivate(documentID, true)
			if err != nil {
				return fmt.Errorf("failed to make document %q private: %w", documentID, err)
			}
			if changed {
				output.PrintSuccess("Document %q is private again", documentID)
			} else {
				fmt.Printf("Document %q is already private\n", documentID)
			}
		}
		if !direct {
			return nil
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
	Use:     "notebook <notebook-id> --user <user-id> | --group <group-id> | --environment link|public",
	Short:   "Share a notebook with users, groups, or the environment",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// shareDashboardCmd is an alias for sharing dashboards
var shareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> --user <user-id> | --group <group-id> | --environment link|public",
	Short:   "Share a dashboard with users, groups, or the environment",
	Aliases: []string{"db"},
	Args:    cobra.ExactArgs(1),
	RunE:    shareDocumentCmd.RunE,
}

// unshareNotebookCmd is an alias for unsharing notebooks
var unshareNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id> [--user <user-id>] [--group <group-id>] [--all] [--environment link|public]",
	Short:   "Remove sharing from a notebook",
	Aliases: []string{"nb"},
	Args:    cobra.ExactArgs(1),
	RunE:    unshareDocumentCmd.RunE,
}

// unshareDashboardCmd is an alias for unsharing dashboards
var unshareDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id> [--user <user-id>] [--group <group-id>] [--all] [--environment link|public]",
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
		cmd.Flags().String("environment", "", "open the document to everyone in the environment: 'link' (an environment share link, at --access) or 'public' (isPrivate=false, read access); cannot be combined with --user/--group")
		// A blank recipient would be sent to the API as an empty SSO ID.
		rejectEmptyFlag(cmd, "user")
		rejectEmptyFlag(cmd, "group")
	}

	// Unshare flags (apply to all unshare subcommands)
	for _, cmd := range []*cobra.Command{unshareDocumentCmd, unshareNotebookCmd, unshareDashboardCmd} {
		cmd.Flags().StringArray("user", []string{}, "SSO user ID to remove (can be specified multiple times)")
		cmd.Flags().StringArray("group", []string{}, "SSO group ID to remove (can be specified multiple times)")
		cmd.Flags().Bool("all", false, "remove all user and group shares (add --environment link to delete the environment share too)")
		cmd.Flags().String("access", "", "filter by access level: 'read' or 'read-write'")
		cmd.Flags().String("environment", "", "'link' deletes the environment share(s) (filtered by --access), 'public' makes the document private again")
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
