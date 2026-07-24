package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/platformtoken"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var accountTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage platform tokens",
	Long:  "Commands for creating, listing, and revoking Dynatrace platform tokens.",
	RunE:  requireSubcommand,
}

var accountTokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a platform token",
	Long: `Create a new Dynatrace platform token.

The --user-uuid flag is optional; if omitted, the current user's UUID is
resolved automatically from the account token's JWT subject claim.

Examples:
  # Create a token with 90-day expiry (default)
  dtctl account token create --name ci-pipeline --scope account-idm-read

  # Create a token with multiple scopes (repeat --scope for each)
  dtctl account token create --name ci-pipeline --scope account-idm-read --scope storage:buckets:read

  # Create a token expiring in 30 days
  dtctl account token create --name ci-pipeline --scope account-idm-read --expires 30d

  # Create with explicit expiration date (RFC3339)
  dtctl account token create --name ci-pipeline --scope account-idm-read --expires-at 2026-10-01T00:00:00Z

  # Create for a specific user
  dtctl account token create --name ci-pipeline --scope account-idm-read --user-uuid <uuid>

  # Dry run to preview
  dtctl account token create --name ci-pipeline --scope account-idm-read --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		scopes, _ := cmd.Flags().GetStringArray("scope")
		expires, _ := cmd.Flags().GetString("expires")
		expiresAt, _ := cmd.Flags().GetString("expires-at")
		userUUID, _ := cmd.Flags().GetString("user-uuid")
		resources, _ := cmd.Flags().GetStringArray("resource")
		tags, _ := cmd.Flags().GetStringArray("tag")

		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if len(scopes) == 0 {
			return fmt.Errorf("--scope is required")
		}
		if expiresAt != "" && cmd.Flags().Changed("expires") {
			return fmt.Errorf("--expires and --expires-at are mutually exclusive")
		}

		var expirationDate string
		if expiresAt != "" {
			t, err := time.Parse(time.RFC3339, expiresAt)
			if err != nil {
				return fmt.Errorf("invalid --expires-at value %q: must be RFC3339 (e.g. 2026-10-01T00:00:00Z): %w", expiresAt, err)
			}
			expirationDate = t.UTC().Format("2006-01-02T15:04:05.000Z")
		} else {
			t, err := parseExpiresDuration(expires)
			if err != nil {
				return err
			}
			expirationDate = t.UTC().Format("2006-01-02T15:04:05.000Z")
		}

		accClient, accountUUID, err := SetupAccountWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		if userUUID == "" {
			userUUID, err = resolveCurrentAccountUserUUID(accountUUID)
			if err != nil {
				return fmt.Errorf("could not auto-resolve user UUID from account token: %w\nHint: provide --user-uuid explicitly", err)
			}
		}

		if len(resources) == 0 {
			resources = []string{"urn:dtaccount:" + accountUUID}
		}

		req := platformtoken.PlatformTokenCreate{
			Name:           name,
			UserUUID:       userUUID,
			Scope:          scopes,
			Resource:       resources,
			Tags:           tags,
			ExpirationDate: expirationDate,
		}

		if dryRun {
			output.PrintInfo("Dry run: would create platform token")
			output.PrintInfo("Name:     %s", req.Name)
			output.PrintInfo("Scope:    %s", strings.Join(req.Scope, ", "))
			output.PrintInfo("UserUUID: %s", req.UserUUID)
			output.PrintInfo("Expires:  %s", req.ExpirationDate)
			return nil
		}

		handler := platformtoken.NewHandler(accClient, accountUUID)
		res, err := handler.Create(req)
		if err != nil {
			return err
		}

		output.PrintSuccess("Platform token %q created (expires: %s)", res.Name, res.ExpirationDate)
		output.PrintWarning("Token secret shown once — store it now:")
		fmt.Println(res.Token)
		return nil
	},
}

var accountTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List platform tokens",
	Long: `List all platform tokens for the account.

Examples:
  # List all tokens
  dtctl account token list

  # Output as JSON
  dtctl account token list -o json
`,
	Aliases: []string{"ls", "get"},
	RunE: func(cmd *cobra.Command, args []string) error {
		accClient, accountUUID, err := SetupAccount()
		if err != nil {
			return err
		}

		handler := platformtoken.NewHandler(accClient, accountUUID)
		tokens, err := handler.List()
		if err != nil {
			return err
		}

		return NewPrinter().PrintList(tokens)
	},
}

var accountTokenRevokeCmd = &cobra.Command{
	Use:     "revoke <tokenId>",
	Aliases: []string{"delete"},
	Short:   "Revoke a platform token",
	Long: `Revoke (delete) a platform token by its ID.

Examples:
  # Revoke a token
  dtctl account token revoke <tokenId>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tokenID := args[0]

		if dryRun {
			output.PrintInfo("Dry run: would revoke platform token %q", tokenID)
			return nil
		}

		accClient, accountUUID, err := SetupAccountWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := platformtoken.NewHandler(accClient, accountUUID)
		if err := handler.Revoke(tokenID); err != nil {
			return err
		}

		output.PrintSuccess("Platform token %q revoked", tokenID)
		return nil
	},
}

// parseExpiresDuration parses strings like "90d" or "720h" into a future time.
func parseExpiresDuration(s string) (time.Time, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: expected Nd (e.g. 90d)", s)
		}
		return time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return time.Now().UTC().Add(d), nil
}

func init() {
	accountCmd.AddCommand(accountTokenCmd)
	accountTokenCmd.AddCommand(accountTokenCreateCmd)
	accountTokenCmd.AddCommand(accountTokenListCmd)
	accountTokenCmd.AddCommand(accountTokenRevokeCmd)

	// Create flags
	accountTokenCreateCmd.Flags().String("name", "", "token name (required)")
	accountTokenCreateCmd.Flags().StringArray("scope", nil, "token scope; may be specified multiple times (required)")
	accountTokenCreateCmd.Flags().String("expires", "90d", "token lifetime (e.g. 30d, 720h)")
	accountTokenCreateCmd.Flags().String("expires-at", "", "exact expiration date in RFC3339 format (mutually exclusive with --expires)")
	accountTokenCreateCmd.Flags().String("user-uuid", "", "user UUID the token belongs to (default: current user)")
	accountTokenCreateCmd.Flags().StringArray("resource", nil, "environment URL(s) the token is scoped to (default: current environment)")
	accountTokenCreateCmd.Flags().StringArray("tag", nil, "token tag; may be specified multiple times")
}
