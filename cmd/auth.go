package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/spf13/cobra"
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
			fmt.Printf("User ID:     %s\n", result.UserID)
			if result.UserName != "" {
				fmt.Printf("User Name:   %s\n", result.UserName)
			}
			if result.EmailAddress != "" {
				fmt.Printf("Email:       %s\n", result.EmailAddress)
			}
			fmt.Printf("Context:     %s\n", result.Context)
			fmt.Printf("Environment: %s\n", result.Environment)
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

Note: OAuth tokens require keyring support. If keyring is not available on your system,
you'll need to use API token authentication instead (dtctl config set-credentials).`,
	Example: `  # Login and create a context named "my-env"
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
		
		// Validate required flags
		if contextName == "" {
			return fmt.Errorf("--context is required")
		}
		if environment == "" {
			return fmt.Errorf("--environment is required")
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
		if !config.IsKeyringAvailable() {
			return fmt.Errorf("OAuth login requires a working system keyring, but none is available. Please configure a keyring (or disable keyring usage if supported) and try again, or use an alternative authentication method.")
		}
		
		// Detect environment and create appropriate OAuth config with safety level
		oauthConfig := auth.OAuthConfigFromEnvironmentURLWithSafety(environment, safetyLevel)
		
		// Log which environment we detected
		fmt.Printf("Detected environment: %s\n", oauthConfig.Environment)
		fmt.Printf("Safety level: %s\n", oauthConfig.SafetyLevel)
		fmt.Printf("Requesting OAuth scopes for safety level %s...\n", oauthConfig.SafetyLevel)
		
		// Create OAuth flow
		flow, err := auth.NewOAuthFlow(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize OAuth: %w", err)
		}
		
		// Start OAuth flow with timeout
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		
		fmt.Println("Starting OAuth authentication flow...")
		tokens, err := flow.Start(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
		
		fmt.Println("✓ Authentication successful!")
		
		// Get user info
		userInfo, err := flow.GetUserInfo(tokens.AccessToken)
		if err != nil {
			fmt.Printf("Warning: Failed to retrieve user info: %v\n", err)
		} else {
			fmt.Printf("Logged in as: %s (%s)\n", userInfo.Name, userInfo.Email)
		}
		
		// Store tokens
		tokenManager, err := auth.NewTokenManager(oauthConfig)
		if err != nil {
			return fmt.Errorf("failed to create token manager: %w", err)
		}
		
		if err := tokenManager.SaveToken(tokenName, tokens); err != nil {
			return fmt.Errorf("failed to store tokens: %w", err)
		}
		
		fmt.Printf("✓ Tokens stored securely as '%s'\n", tokenName)
		
		// Create or update context with safety level
		cfg.SetContextWithOptions(contextName, environment, tokenName, &config.ContextOptions{
			SafetyLevel: safetyLevel,
		})
		cfg.CurrentContext = contextName
		
		// Save config
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		
		fmt.Printf("✓ Context '%s' configured and activated\n", contextName)
		fmt.Println("\nYou can now use dtctl commands with this context.")
		
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
			fmt.Printf("Warning: Failed to delete token from keyring: %v\n", err)
		} else {
			fmt.Printf("✓ Removed OAuth token '%s'\n", tokenName)
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
			
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			
			fmt.Printf("✓ Removed context '%s'\n", contextName)
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
		
		fmt.Println("Refreshing OAuth tokens...")
		tokens, err := tokenManager.RefreshToken(tokenName)
		if err != nil {
			return fmt.Errorf("failed to refresh tokens: %w", err)
		}
		
		fmt.Println("✓ Tokens refreshed successfully")
		fmt.Printf("New token expires at: %s\n", tokens.ExpiresAt.Format(time.RFC3339))
		
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
	authLoginCmd.Flags().String("context", "", "name for the context to create (required)")
	authLoginCmd.Flags().String("environment", "", "Dynatrace environment URL (required)")
	authLoginCmd.Flags().String("token-name", "", "name for storing the OAuth token (defaults to <context>-oauth)")
	authLoginCmd.Flags().String("timeout", "5m", "timeout for the authentication flow")
	authLoginCmd.Flags().String("safety-level", string(config.DefaultSafetyLevel), "safety level for the context (readonly, readwrite-mine, readwrite-all, dangerously-unrestricted)")
	authLoginCmd.MarkFlagRequired("context")
	authLoginCmd.MarkFlagRequired("environment")
	
	// Flags for logout
	authLogoutCmd.Flags().Bool("remove-context", false, "also remove the context configuration")
}
