package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

var (
	idOnly  bool
	refresh bool
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication and user identity",
	Long:  `View authentication information and test permissions.`,
}

// WhoamiResult contains the current user information for output
type WhoamiResult struct {
	UserID       string `json:"userId" yaml:"userId"`
	UserName     string `json:"userName,omitempty" yaml:"userName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty" yaml:"emailAddress,omitempty"`
	Context      string `json:"context" yaml:"context"`
	Environment  string `json:"environment" yaml:"environment"`
}

// authWhoamiCmd shows current user identity
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display the current user identity",
	Long: `Display information about the currently authenticated user.

This command shows the user ID, name, and email address associated with
the current authentication token. It also displays the active context
and environment.

The user information is retrieved from the Dynatrace metadata API.
If that fails (e.g., missing scope), it falls back to decoding the
JWT token's 'sub' claim.`,
	Example: `  # View current user info
  dtctl auth whoami

  # Get just the user ID (useful for scripting)
  dtctl auth whoami --id-only

  # Output as JSON
  dtctl auth whoami -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ctx, err := cfg.CurrentContextObj()
		if err != nil {
			return fmt.Errorf("failed to get current context: %w", err)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		// If --id-only, just get the user ID
		if idOnly {
			userID, err := c.CurrentUserID()
			if err != nil {
				return fmt.Errorf("failed to get user ID: %w", err)
			}
			fmt.Println(userID)
			return nil
		}

		// Try to get full user info from metadata API
		userInfo, err := c.CurrentUser()
		if err != nil {
			// Fallback to JWT decoding for user ID only
			userID, jwtErr := client.ExtractUserIDFromToken(cfg.MustGetToken(ctx.TokenRef))
			if jwtErr != nil {
				return fmt.Errorf("failed to get user info: %w (JWT fallback also failed: %v)", err, jwtErr)
			}
			userInfo = &client.UserInfo{
				UserID: userID,
			}
		}

		result := WhoamiResult{
			UserID:       userInfo.UserID,
			UserName:     userInfo.UserName,
			EmailAddress: userInfo.EmailAddress,
			Context:      cfg.CurrentContext,
			Environment:  ctx.Environment,
		}

		printer := NewPrinter()

		// For table output, use a custom format
		if outputFormat == "table" || outputFormat == "" {
			const w = 13
			output.DescribeKV("User ID:", w, "%s", result.UserID)
			if result.UserName != "" {
				output.DescribeKV("User Name:", w, "%s", result.UserName)
			}
			if result.EmailAddress != "" {
				output.DescribeKV("Email:", w, "%s", result.EmailAddress)
			}
			output.DescribeKV("Context:", w, "%s", result.Context)
			output.DescribeKV("Environment:", w, "%s", result.Environment)
			return nil
		}

		return printer.Print(result)
	},
}

// authLoginCmd initiates browser-based OAuth login
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate using browser-based OAuth login",
	Long: `Authenticate with Dynatrace using OAuth 2.0 browser-based login.

This command will:
1. Open your default browser to the Dynatrace login page
2. Wait for you to complete authentication
3. Store the OAuth tokens securely in your system keyring
4. Configure a context to use the authenticated session

After successful login, you can use dtctl commands without needing to manage API tokens manually.

If --context and --environment are omitted, the current context is used. This is useful
for re-authenticating when both the access token and refresh token have expired.

Note: OAuth tokens require keyring support. If keyring is not available on your system,
you'll need to use API token authentication instead (dtctl config set-credentials).`,
	Example: `  # Re-authenticate the current context (e.g. after token expiry)
  dtctl auth login

  # Login and create a new context named "my-env"
  dtctl auth login --context my-env --environment https://abc12345.apps.dynatrace.com

  # Login with a specific token name
  dtctl auth login --context my-env --environment https://abc12345.apps.dynatrace.com --token-name my-oauth-token

  # Login with custom timeout
  dtctl auth login --context my-env --environment https://abc12345.apps.dynatrace.com --timeout 5m`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		contextName, _ := cmd.Flags().GetString("context")
		environment, _ := cmd.Flags().GetString("environment")
		tokenName, _ := cmd.Flags().GetString("token-name")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		safetyLevelStr, _ := cmd.Flags().GetString("safety-level")

		// If --context or --environment are not provided, fall back to the current context
		if contextName == "" || environment == "" {
			contextHint := "Use 'dtctl ctx' to list available context names, then pass --context <name> --environment <url>"
			cfg, err := LoadConfig()
			if err != nil {
				return &diagnostic.Error{
					Operation:   "auth login",
					Message:     "--context and --environment are required (no existing config found)",
					Suggestions: []string{contextHint},
					Err:         err,
				}
			}
			if cfg.CurrentContext == "" {
				return &diagnostic.Error{
					Operation:   "auth login",
					Message:     "--context and --environment are required when no current context is set",
					Suggestions: []string{contextHint, "Set a current context with 'dtctl ctx use-context <name>'"},
				}
			}
			ctx, err := cfg.CurrentContextObj()
			if err != nil {
				return &diagnostic.Error{
					Operation:   "auth login",
					Message:     "--context and --environment are required (failed to load current context)",
					Suggestions: []string{contextHint},
					Err:         err,
				}
			}
			if contextName == "" {
				contextName = cfg.CurrentContext
			}
			if environment == "" {
				environment = ctx.Environment
			}
			// Preserve the existing token name so the stored token is updated in-place
			if tokenName == "" && ctx.TokenRef != "" {
				tokenName = ctx.TokenRef
			}
		}

		// Default token name to context name if not provided
		if tokenName == "" {
			tokenName = contextName + "-oauth"
		}

		// Parse timeout
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}

		// Parse and validate safety level
		safetyLevel := config.SafetyLevel(safetyLevelStr)
		if safetyLevelStr == "" {
			safetyLevel = config.DefaultSafetyLevel
		} else if !safetyLevel.IsValid() {
			return fmt.Errorf("invalid safety level: %s (valid values: %v)", safetyLevelStr, config.ValidSafetyLevels())
		}

		// Load config
		cfg, err := LoadConfig()
		if err != nil {
			// If config doesn't exist, create a new one
			cfg = config.NewConfig()
		}

		// Ensure keyring is available before starting OAuth flow
		if keyringErr := config.CheckKeyring(); keyringErr != nil {
			recovered := false
			// On Linux/WSL the persistent keyring collection may not exist yet.
			// Attempt to create it — this may trigger an OS password prompt.
			if strings.Contains(keyringErr.Error(), "failed to unlock correct collection") {
				output.PrintInfo("No keyring collection found — creating one (you may be prompted for a password)...")
				if initErr := config.EnsureKeyringCollection(); initErr == nil {
					if config.CheckKeyring() == nil {
						output.PrintSuccess("Keyring collection created successfully")
						recovered = true
					}
				}
			}
			if !recovered {
				return &diagnostic.Error{
					Operation: "auth login",
					Message:   fmt.Sprintf("OAuth login requires a working system keyring: %v", keyringErr),
					Suggestions: []string{
						"Check that a D-Bus session bus is available (echo $DBUS_SESSION_BUS_ADDRESS)",
						"Ensure a Secret Service provider is running (e.g. gnome-keyring-daemon --start --components=secrets)",
						"Unset DTCTL_DISABLE_KEYRING if it was set unintentionally",
						"Use API token authentication instead: dtctl config set-credentials",
					},
				}
			}
		}

		// Warn about potentially wrong environment URLs
		if problems := diagnostic.CheckEnvironmentURL(environment); len(problems) > 0 {
			for _, p := range problems {
				output.PrintWarning("%s", p.Message)
				if p.SuggestedURL != "" {
					output.PrintHint("Did you mean: %s", p.SuggestedURL)
				}
			}
			fmt.Fprintln(os.Stderr)
		}

		// Detect environment and create appropriate OAuth config with safety level
		oauthConfig := auth.OAuthConfigFromEnvironmentURLWithSafety(environment, safetyLevel)

		// Log which environment we detected
		output.PrintInfo("Detected environment: %s", oauthConfig.Environment)
		output.PrintInfo("Safety level: %s", oauthConfig.SafetyLevel)
		output.PrintInfo("Requesting OAuth scopes for safety level %s...", oauthConfig.SafetyLevel)

		// Create OAuth flow
		flow, err := auth.NewOAuthFlow(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize OAuth: %w", err)
		}

		// Start OAuth flow with timeout
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		output.PrintInfo("Starting OAuth authentication flow...")
		tokens, err := flow.Start(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		output.PrintSuccess("Authentication successful!")

		// Get user info
		userInfo, err := flow.GetUserInfo(tokens.AccessToken)
		if err != nil {
			output.PrintWarning("Failed to retrieve user info: %v", err)
		} else {
			output.PrintInfo("Logged in as: %s (%s)", userInfo.Name, userInfo.Email)
		}

		// Store tokens
		tokenManager, err := auth.NewTokenManager(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to create token manager: %w", err)
		}

		if err := tokenManager.SaveToken(tokenName, tokens); err != nil {
			return fmt.Errorf("failed to store tokens: %w", err)
		}

		output.PrintSuccess("Tokens stored securely as '%s'", tokenName)

		// Create or update context with safety level
		cfg.SetContextWithOptions(contextName, environment, tokenName, &config.ContextOptions{
			SafetyLevel: safetyLevel,
		})
		cfg.CurrentContext = contextName

		// Save config (respects local .dtctl.yaml if present)
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		output.PrintSuccess("Context '%s' configured and activated", contextName)
		output.PrintInfo("\nYou can now use dtctl commands with this context.")

		return nil
	},
}

// authLogoutCmd logs out and removes OAuth tokens
var authLogoutCmd = &cobra.Command{
	Use:   "logout [context-name]",
	Short: "Logout and remove OAuth tokens",
	Long: `Remove stored OAuth tokens for a context.

This command will:
1. Remove OAuth tokens from the system keyring
2. Optionally remove the context configuration

If no context name is provided, the current context will be used.`,
	Example: `  # Logout from current context
  dtctl auth logout

  # Logout from specific context
  dtctl auth logout my-env

  # Logout and remove context
  dtctl auth logout my-env --remove-context`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		cfg, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Determine context name
		var contextName string
		if len(args) > 0 {
			contextName = args[0]
		} else {
			contextName = cfg.CurrentContext
		}

		if contextName == "" {
			return fmt.Errorf("no context specified and no current context set")
		}

		// Find context
		ctx, err := cfg.GetContext(contextName)
		if err != nil {
			return fmt.Errorf("context not found: %w", err)
		}

		// Get token name
		tokenName := ctx.Context.TokenRef
		if tokenName == "" {
			return fmt.Errorf("context has no token reference")
		}

		// Detect environment from context URL
		oauthConfig := auth.OAuthConfigFromEnvironmentURLWithSafety(ctx.Context.Environment, ctx.Context.SafetyLevel)

		// Delete OAuth token
		tokenManager, err := auth.NewTokenManager(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to create token manager: %w", err)
		}

		if err := tokenManager.DeleteToken(tokenName); err != nil {
			output.PrintWarning("Failed to delete token from keyring: %v", err)
		} else {
			output.PrintSuccess("Removed OAuth token '%s'", tokenName)
		}

		// Optionally remove context
		removeContext, _ := cmd.Flags().GetBool("remove-context")
		if removeContext {
			if err := cfg.DeleteContext(contextName); err != nil {
				return fmt.Errorf("failed to remove context: %w", err)
			}

			// If we deleted the current context, clear it
			if cfg.CurrentContext == contextName {
				cfg.CurrentContext = ""
			}

			if err := saveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			output.PrintSuccess("Removed context '%s'", contextName)
		}

		return nil
	},
}

// authRefreshCmd refreshes OAuth tokens
var authRefreshCmd = &cobra.Command{
	Use:   "refresh [context-name]",
	Short: "Refresh OAuth tokens",
	Long: `Refresh OAuth access tokens using the refresh token.

This command manually triggers a token refresh. Normally, dtctl will
automatically refresh tokens when needed, but this command can be used
to force a refresh.`,
	Example: `  # Refresh tokens for current context
  dtctl auth refresh

  # Refresh tokens for specific context
  dtctl auth refresh my-env`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		cfg, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Determine context name
		var contextName string
		if len(args) > 0 {
			contextName = args[0]
		} else {
			contextName = cfg.CurrentContext
		}

		if contextName == "" {
			return fmt.Errorf("no context specified and no current context set")
		}

		// Find context
		ctx, err := cfg.GetContext(contextName)
		if err != nil {
			return fmt.Errorf("context not found: %w", err)
		}

		// Get token name
		tokenName := ctx.Context.TokenRef
		if tokenName == "" {
			return fmt.Errorf("context has no token reference")
		}

		// Detect environment from context URL
		oauthConfig := auth.OAuthConfigFromEnvironmentURLWithSafety(ctx.Context.Environment, ctx.Context.SafetyLevel)

		// Refresh token
		tokenManager, err := auth.NewTokenManager(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to create token manager: %w", err)
		}

		output.PrintInfo("Refreshing OAuth tokens...")
		tokens, err := tokenManager.RefreshToken(tokenName)
		if err != nil {
			return fmt.Errorf("failed to refresh tokens: %w", err)
		}

		output.PrintSuccess("Tokens refreshed")
		output.PrintInfo("New token expires at: %s", tokens.ExpiresAt.Format(time.RFC3339))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)

	authCmd.AddCommand(authWhoamiCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authRefreshCmd)

	// Flags for whoami
	authWhoamiCmd.Flags().BoolVar(&idOnly, "id-only", false, "output only the user ID")
	authWhoamiCmd.Flags().BoolVar(&refresh, "refresh", false, "force refresh of cached user info")

	// Flags for login
	authLoginCmd.Flags().String("context", "", "name for the context to create or update (defaults to current context)")
	authLoginCmd.Flags().String("environment", "", "Dynatrace environment URL (defaults to current context's environment)")
	authLoginCmd.Flags().String("token-name", "", "name for storing the OAuth token (defaults to existing token name or <context>-oauth)")
	authLoginCmd.Flags().String("timeout", "5m", "timeout for the authentication flow")
	authLoginCmd.Flags().String("safety-level", string(config.DefaultSafetyLevel), "safety level for the context (readonly, readwrite-mine, readwrite-all, dangerously-unrestricted)")

	// Flags for logout
	authLogoutCmd.Flags().Bool("remove-context", false, "also remove the context configuration")
}
