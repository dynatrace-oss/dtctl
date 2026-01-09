package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dtctl configuration",
	Long:  `View and modify dtctl configuration including contexts and credentials.`,
}

// configViewCmd represents the config view command
var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		printer := NewPrinter()
		return printer.Print(cfg)
	},
}

// configGetContextsCmd lists all contexts
var configGetContextsCmd = &cobra.Command{
	Use:   "get-contexts",
	Short: "List all available contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		printer := NewPrinter()
		return printer.PrintList(cfg.Contexts)
	},
}

// configCurrentContextCmd shows the current context
var configCurrentContextCmd = &cobra.Command{
	Use:   "current-context",
	Short: "Display the current context",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		fmt.Println(cfg.CurrentContext)
		return nil
	},
}

// configUseContextCmd switches to a different context
var configUseContextCmd = &cobra.Command{
	Use:   "use-context <context-name>",
	Short: "Switch to a different context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		contextName := args[0]

		// Check if context exists
		found := false
		for _, nc := range cfg.Contexts {
			if nc.Name == contextName {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("context %q not found", contextName)
		}

		cfg.CurrentContext = contextName

		if err := cfg.Save(); err != nil {
			return err
		}

		fmt.Printf("Switched to context %q\n", contextName)
		return nil
	},
}

// configSetContextCmd creates or updates a context
var configSetContextCmd = &cobra.Command{
	Use:   "set-context <context-name>",
	Short: "Set a context entry in the config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		environment, _ := cmd.Flags().GetString("environment")
		tokenRef, _ := cmd.Flags().GetString("token-ref")
		ns, _ := cmd.Flags().GetString("namespace")

		if environment == "" {
			return fmt.Errorf("--environment is required")
		}

		cfg, err := config.Load()
		if err != nil {
			// Create new config if it doesn't exist
			cfg = config.NewConfig()
		}

		cfg.SetContext(contextName, environment, tokenRef, ns)

		// Set as current context if it's the first one
		if len(cfg.Contexts) == 1 || cfg.CurrentContext == "" {
			cfg.CurrentContext = contextName
		}

		if err := cfg.Save(); err != nil {
			return err
		}

		fmt.Printf("Context %q set\n", contextName)
		return nil
	},
}

// configSetCredentialsCmd sets credentials for a context
var configSetCredentialsCmd = &cobra.Command{
	Use:   "set-credentials <name>",
	Short: "Set credentials in the config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			return fmt.Errorf("--token is required")
		}

		cfg, err := config.Load()
		if err != nil {
			cfg = config.NewConfig()
		}

		if err := cfg.SetToken(name, token); err != nil {
			return err
		}

		if err := cfg.Save(); err != nil {
			return err
		}

		if config.IsKeyringAvailable() {
			fmt.Printf("Credentials %q stored securely in %s\n", name, config.KeyringBackend())
		} else {
			fmt.Printf("Credentials %q set (warning: stored in plaintext, keyring not available)\n", name)
		}
		return nil
	},
}

// configMigrateTokensCmd migrates tokens from config file to OS keyring
var configMigrateTokensCmd = &cobra.Command{
	Use:   "migrate-tokens",
	Short: "Migrate tokens from config file to OS keyring",
	Long: `Migrate plaintext tokens from the config file to the secure OS keyring.

This command moves tokens stored in ~/.config/dtctl/config to:
  - macOS: Keychain
  - Linux: Secret Service (GNOME Keyring, KWallet)
  - Windows: Credential Manager

After migration, tokens are removed from the config file and stored securely.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.IsKeyringAvailable() {
			return fmt.Errorf("keyring not available on this system. Tokens will remain in config file")
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		migrated, err := config.MigrateTokensToKeyring(cfg)
		if err != nil {
			return err
		}

		if migrated == 0 {
			fmt.Println("No tokens to migrate (already migrated or none configured)")
			return nil
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config after migration: %w", err)
		}

		fmt.Printf("Successfully migrated %d token(s) to %s\n", migrated, config.KeyringBackend())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configGetContextsCmd)
	configCmd.AddCommand(configCurrentContextCmd)
	configCmd.AddCommand(configUseContextCmd)
	configCmd.AddCommand(configSetContextCmd)
	configCmd.AddCommand(configSetCredentialsCmd)
	configCmd.AddCommand(configMigrateTokensCmd)

	// Flags for set-context
	configSetContextCmd.Flags().String("environment", "", "environment URL")
	configSetContextCmd.Flags().String("token-ref", "", "token reference name")
	configSetContextCmd.Flags().String("namespace", "", "namespace")

	// Flags for set-credentials
	configSetCredentialsCmd.Flags().String("token", "", "API token")
}
