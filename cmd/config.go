package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/spf13/cobra"
)

// loadConfigRaw loads configuration respecting the --config flag but WITHOUT applying
// runtime overrides like --context. This is used for configuration management commands.
func loadConfigRaw() (*config.Config, error) {
	if cfgFile != "" {
		return config.LoadFrom(cfgFile)
	}
	return config.Load()
}

// saveConfig saves configuration respecting the --config flag and local config presence
func saveConfig(cfg *config.Config) error {
	if cfgFile != "" {
		return cfg.SaveTo(cfgFile)
	}
	// If a local config exists, save to it
	if local := config.FindLocalConfig(); local != "" {
		return cfg.SaveTo(local)
	}
	// Fall back to default global location
	return cfg.Save()
}

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
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		printer := NewPrinter()
		return printer.Print(cfg)
	},
}

// ContextListItem is a flattened view of a context for table display
type ContextListItem struct {
	Current     string `table:"CURRENT"`
	Name        string `table:"NAME"`
	Environment string `table:"ENVIRONMENT"`
	SafetyLevel string `table:"SAFETY-LEVEL"`
	Description string `table:"DESCRIPTION,wide"`
}

// configGetContextsCmd lists all contexts
var configGetContextsCmd = &cobra.Command{
	Use:   "get-contexts",
	Short: "List all available contexts",
	Long: `List all available contexts with their safety levels.

Examples:
  # List contexts
  dtctl config get-contexts

  # List contexts with descriptions
  dtctl config get-contexts -o wide
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Create flattened list for display
		var items []ContextListItem
		for _, nc := range cfg.Contexts {
			current := ""
			if nc.Name == cfg.CurrentContext {
				current = "*"
			}
			safetyLevel := nc.Context.SafetyLevel.String()
			items = append(items, ContextListItem{
				Current:     current,
				Name:        nc.Name,
				Environment: nc.Context.Environment,
				SafetyLevel: safetyLevel,
				Description: nc.Context.Description,
			})
		}

		printer := NewPrinter()
		return printer.PrintList(items)
	},
}

// configCurrentContextCmd shows the current context
var configCurrentContextCmd = &cobra.Command{
	Use:   "current-context",
	Short: "Display the current context",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
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
		cfg, err := loadConfigRaw()
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

		if err := saveConfig(cfg); err != nil {
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
	Long: `Create or update a context with connection and safety settings.

Safety Levels (from safest to most permissive):
  readonly                  - No modifications allowed (production monitoring)
  readwrite-mine            - Create/update/delete own resources only
  readwrite-all             - Modify all resources, no bucket deletion (default)
  dangerously-unrestricted  - All operations including bucket deletion

Note: Safety levels are client-side checks to prevent accidental mistakes.
For actual security, configure your API token with appropriate scopes.

Examples:
  # Create a production read-only context
  dtctl config set-context prod-viewer \
    --environment https://prod.dynatrace.com \
    --token-ref prod-token \
    --safety-level readonly

  # Create a context for team collaboration
  dtctl config set-context staging \
    --environment https://staging.dynatrace.com \
    --token-ref staging-token \
    --safety-level readwrite-all \
    --description "Staging environment"
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		environment, _ := cmd.Flags().GetString("environment")
		tokenRef, _ := cmd.Flags().GetString("token-ref")
		safetyLevel, _ := cmd.Flags().GetString("safety-level")
		description, _ := cmd.Flags().GetString("description")

		cfg, err := loadConfigRaw()
		if err != nil {
			// Create new config if it doesn't exist
			cfg = config.NewConfig()
		}

		// Check if this is an update to an existing context
		isUpdate := false
		for _, nc := range cfg.Contexts {
			if nc.Name == contextName {
				isUpdate = true
				if environment == "" {
					environment = nc.Context.Environment
				}
				break
			}
		}

		// Require environment for new contexts
		if !isUpdate && environment == "" {
			return fmt.Errorf("--environment is required for new contexts")
		}

		// Validate safety level if provided
		if safetyLevel != "" {
			level := config.SafetyLevel(safetyLevel)
			if !level.IsValid() {
				return fmt.Errorf("invalid safety level %q. Valid values: readonly, readwrite-mine, readwrite-all, dangerously-unrestricted", safetyLevel)
			}
		}

		opts := &config.ContextOptions{
			SafetyLevel: config.SafetyLevel(safetyLevel),
			Description: description,
		}

		cfg.SetContextWithOptions(contextName, environment, tokenRef, opts)

		// Set as current context if it's the first one
		if len(cfg.Contexts) == 1 || cfg.CurrentContext == "" {
			cfg.CurrentContext = contextName
		}

		if err := saveConfig(cfg); err != nil {
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

		cfg, err := loadConfigRaw()
		if err != nil {
			cfg = config.NewConfig()
		}

		if err := cfg.SetToken(name, token); err != nil {
			return err
		}

		if err := saveConfig(cfg); err != nil {
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

// configSetCmd sets a configuration value
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value such as preferences.

Supported keys:
  - preferences.editor: Set the default editor for edit commands`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		cfg, err := loadConfigRaw()
		if err != nil {
			// Create new config if it doesn't exist
			cfg = config.NewConfig()
		}

		switch key {
		case "preferences.editor":
			cfg.Preferences.Editor = value
		default:
			return fmt.Errorf("unknown configuration key %q", key)
		}

		if err := saveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Configuration %q set to %q\n", key, value)
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

		cfg, err := loadConfigRaw()
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

		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config after migration: %w", err)
		}

		fmt.Printf("Successfully migrated %d token(s) to %s\n", migrated, config.KeyringBackend())
		return nil
	},
}

// configDescribeContextCmd shows detailed info about a context
var configDescribeContextCmd = &cobra.Command{
	Use:     "describe-context <context-name>",
	Aliases: []string{"desc-ctx"},
	Short:   "Show detailed information about a context",
	Long: `Show detailed information about a context including its safety level and settings.

Examples:
  # Describe the current context
  dtctl config describe-context $(dtctl config current-context)

  # Describe a specific context
  dtctl config describe-context production
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Find the context
		var found *config.NamedContext
		for i := range cfg.Contexts {
			if cfg.Contexts[i].Name == contextName {
				found = &cfg.Contexts[i]
				break
			}
		}

		if found == nil {
			return fmt.Errorf("context %q not found", contextName)
		}

		// Print context details
		isCurrent := found.Name == cfg.CurrentContext
		currentMark := ""
		if isCurrent {
			currentMark = " (current)"
		}

		fmt.Printf("Name:         %s%s\n", found.Name, currentMark)
		fmt.Printf("Environment:  %s\n", found.Context.Environment)
		fmt.Printf("Token-Ref:    %s\n", found.Context.TokenRef)
		fmt.Printf("Safety Level: %s\n", found.Context.GetEffectiveSafetyLevel())

		// Show safety level description
		switch found.Context.GetEffectiveSafetyLevel() {
		case config.SafetyLevelReadOnly:
			fmt.Printf("              (No modifications allowed)\n")
		case config.SafetyLevelReadWriteMine:
			fmt.Printf("              (Create/update/delete own resources)\n")
		case config.SafetyLevelReadWriteAll:
			fmt.Printf("              (Modify all resources, no bucket deletion)\n")
		case config.SafetyLevelDangerouslyUnrestricted:
			fmt.Printf("              (All operations including bucket deletion)\n")
		}

		if found.Context.Description != "" {
			fmt.Printf("Description:  %s\n", found.Context.Description)
		}

		return nil
	},
}

// configDeleteContextCmd deletes a context from the configuration
var configDeleteContextCmd = &cobra.Command{
	Use:     "delete-context <context-name>",
	Aliases: []string{"rm-ctx"},
	Short:   "Delete a context from the config",
	Long: `Delete a context from the configuration.

If the deleted context is the current context, the current-context will be cleared.
You will need to use 'dtctl config use-context' to set a new current context.

Note: This does not delete the associated credentials. Use 'dtctl config set-credentials'
to manage credentials separately.

Examples:
  # Delete a context
  dtctl config delete-context old-env

  # Delete the staging context
  dtctl config delete-context staging
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextName := args[0]

		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		// Check if context exists before deleting
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

		// Delete the context
		if err := cfg.DeleteContext(contextName); err != nil {
			return err
		}

		// Clear current context if we just deleted it
		if cfg.CurrentContext == contextName {
			cfg.CurrentContext = ""
			fmt.Printf("Warning: deleted the current context. Use 'dtctl config use-context' to set a new one.\n")
		}

		if err := saveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Context %q deleted\n", contextName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configGetContextsCmd)
	configCmd.AddCommand(configCurrentContextCmd)
	configCmd.AddCommand(configUseContextCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configSetContextCmd)
	configCmd.AddCommand(configSetCredentialsCmd)
	configCmd.AddCommand(configMigrateTokensCmd)
	configCmd.AddCommand(configDescribeContextCmd)
	configCmd.AddCommand(configDeleteContextCmd)

	// Flags for set-context
	configSetContextCmd.Flags().String("environment", "", "environment URL")
	configSetContextCmd.Flags().String("token-ref", "", "token reference name")
	configSetContextCmd.Flags().String("safety-level", "", "safety level (readonly, readwrite-mine, readwrite-all, dangerously-unrestricted)")
	configSetContextCmd.Flags().String("description", "", "human-readable description for this context")

	// Flags for set-credentials
	configSetCredentialsCmd.Flags().String("token", "", "API token")
}
