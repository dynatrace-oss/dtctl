package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

var accountLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate for account-plane operations via browser-based OAuth",
	Long: `Authenticate with the Dynatrace Account Management API using browser-based OAuth.

Opens a browser window to complete login. On success, the account token is stored
in the system keyring (or DTCTL_TOKEN_STORAGE=file fallback) so subsequent
account commands work without DTCTL_ACCOUNT_TOKEN.

The account UUID is resolved from: --account-uuid flag > DTCTL_ACCOUNT_UUID >
context account-uuid. It must be provided by one of these; find it in the
Dynatrace Account Management console (myaccount.dynatrace.com).`,
	Example: `  # Login with explicit account UUID
  dtctl account login --account-uuid xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

  # Or provide it via the environment
  DTCTL_ACCOUNT_UUID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx dtctl account login`,
	RunE: func(cmd *cobra.Command, args []string) error {
		uuidFlag, _ := cmd.Flags().GetString("account-uuid")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		ctx, err := cfg.CurrentContextObj()
		if err != nil {
			return err
		}

		env := auth.DetectEnvironment(ctx.Environment)

		// Resolve UUID: flag > DTCTL_ACCOUNT_UUID > context config.
		accountUUID := resolveUUIDNoDiscovery(ctx, uuidFlag)
		if accountUUID == "" {
			return fmt.Errorf("account UUID required for environment %q: pass --account-uuid, set DTCTL_ACCOUNT_UUID, or add account-uuid to the current context", extractEnvironmentID(ctx.Environment))
		}

		oauthConfig := auth.AccountOAuthConfig(env, ctx.SafetyLevel, accountUUID)

		// Ensure token storage is available (mirrors auth login).
		if keyringErr := authCheckKeyringFunc(); keyringErr != nil {
			recovered := false
			if strings.Contains(keyringErr.Error(), config.ErrMsgCollectionUnlock) {
				output.PrintInfo("No keyring collection — creating one (you may be prompted for a password)...")
				if initErr := authEnsureKeyringFunc(cmd.Context()); initErr == nil {
					if authCheckKeyringFunc() == nil {
						output.PrintSuccess("Keyring collection created")
						recovered = true
					}
				}
			}
			if !recovered {
				if !config.IsFileTokenStorage() {
					return fmt.Errorf("token storage unavailable: %v\n\nSet DTCTL_TOKEN_STORAGE=file to use file-based fallback", keyringErr)
				}
				output.PrintWarning("Keyring unavailable; using file-based token storage (%s)", config.OAuthStorageBackend())
			}
		}

		output.PrintInfo("Detected environment: %s", env)
		output.PrintInfo("Account UUID: %s", accountUUID)
		output.PrintInfo("Starting account OAuth flow (browser will open)...")

		flow, err := auth.NewOAuthFlow(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize OAuth: %w", err)
		}

		flowCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		tokens, err := flow.Start(flowCtx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		output.PrintSuccess("Authentication successful!")

		tokenManager, err := auth.NewTokenManager(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to create token manager: %w", err)
		}

		if err := tokenManager.SaveToken(accountTokenKeyName(accountUUID), tokens); err != nil {
			return fmt.Errorf("failed to store tokens: %w", err)
		}

		// Persist the UUID so follow-up commands find the keyring entry without
		// requiring --account-uuid or DTCTL_ACCOUNT_UUID each time.
		if ctx.AccountUUID == "" {
			if nc, err := cfg.GetContext(cfg.CurrentContext); err == nil {
				nc.Context.AccountUUID = accountUUID
				if saveErr := cfg.Save(); saveErr != nil {
					output.PrintWarning("Could not persist account-uuid to context: %v", saveErr)
				}
			}
		}

		output.PrintSuccess("Account token stored. Run 'dtctl account list token' to verify access.")
		return nil
	},
}

func init() {
	accountCmd.AddCommand(accountLoginCmd)
	accountLoginCmd.Flags().String("account-uuid", "", "account UUID (overrides DTCTL_ACCOUNT_UUID and context account-uuid)")
	accountLoginCmd.Flags().String("timeout", "5m", "timeout for the OAuth browser flow")
}
