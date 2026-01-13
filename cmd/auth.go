package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
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

func init() {
	rootCmd.AddCommand(authCmd)

	authCmd.AddCommand(authWhoamiCmd)

	// Flags for whoami
	authWhoamiCmd.Flags().BoolVar(&idOnly, "id-only", false, "output only the user ID")
	authWhoamiCmd.Flags().BoolVar(&refresh, "refresh", false, "force refresh of cached user info")
}
