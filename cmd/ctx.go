package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// ctxCmd is a top-level shortcut for context management.
// It provides quick access to the most common context operations
// without the "config" prefix.
var ctxCmd = &cobra.Command{
	Use:   "ctx [context-name]",
	Short: "Manage contexts (shortcut for config context commands)",
	Long: `Quick context management without the "config" prefix.

When called without arguments, lists all contexts.
When called with a context name, switches to that context.

Examples:
  # List all contexts
  dtctl ctx

  # Switch to a context
  dtctl ctx production

  # Show current context
  dtctl ctx current

  # Describe a context
  dtctl ctx describe production

  # Create or update a context (also switches to it)
  dtctl ctx set staging --environment https://staging.example.com

  # Delete a context
  dtctl ctx delete old-env
`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var names []string
		for _, nc := range cfg.Contexts {
			names = append(names, nc.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// No args: list contexts (same as config get-contexts)
			return listContexts()
		}

		// One arg: switch to that context (same as config use-context)
		return useContext(args[0])
	},
}

// ctxTokenCmd prints the resolved token for the current (or named) context.
// It prints the raw credential only — not a full Authorization header value
// (no "Bearer "/"Api-Token " scheme prefix), so callers must add the scheme themselves.
// OAuth tokens are auto-refreshed if expired, so the printed value matches what
// dtctl itself sends.
var ctxTokenCmd = &cobra.Command{
	Use:   "token [context-name]",
	Short: "Print the resolved token for a context",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		name := cfg.CurrentContext
		if len(args) == 1 {
			name = args[0]
		}
		nc, err := cfg.GetContext(name)
		if err != nil {
			return err
		}
		tok, err := client.GetTokenForContext(cfg, nc.Context.Environment, nc.Context.TokenRef)
		if err != nil {
			return err
		}
		fmt.Println(tok)
		return nil
	},
}

// ctxCurrentCmd shows the current context name
var ctxCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Display the current context name",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		fmt.Println(cfg.CurrentContext)
		return nil
	},
}

// ctxDescribeCmd shows detailed context information
var ctxDescribeCmd = &cobra.Command{
	Use:   "describe <context-name>",
	Short: "Show detailed information about a context",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var names []string
		for _, nc := range cfg.Contexts {
			names = append(names, nc.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return describeContext(args[0])
	},
}

// ctxSetCmd creates or updates a context and activates it
var ctxSetCmd = &cobra.Command{
	Use:   "set <context-name>",
	Short: "Create or update a context and set it as current",
	Long: `Create or update a context with connection and safety settings.

The context is always set as the current context after being created or updated.
To switch to an existing context without changing its settings, use: dtctl ctx <name>

Safety Levels (from safest to most permissive):
  readonly                  - No modifications allowed
  readwrite-mine            - Create/update/delete own resources only
  readwrite-all             - Modify all resources, no bucket deletion (default)
  dangerously-unrestricted  - All operations including bucket deletion

Examples:
  dtctl ctx set prod --environment https://prod.example.com --safety-level readonly
  dtctl ctx set staging --environment https://staging.example.com --token-ref my-token
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, _ := cmd.Flags().GetString("environment")
		tokenRef, _ := cmd.Flags().GetString("token-ref")
		safetyLevel, _ := cmd.Flags().GetString("safety-level")
		description, _ := cmd.Flags().GetString("description")

		return setContext(args[0], environment, tokenRef, safetyLevel, description)
	},
}

// ctxDeleteCmd deletes a context
var ctxDeleteCmd = &cobra.Command{
	Use:     "delete <context-name>",
	Aliases: []string{"rm"},
	Short:   "Delete a context",
	Args:    cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var names []string
		for _, nc := range cfg.Contexts {
			names = append(names, nc.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteContext(args[0])
	},
}

// listContexts lists all available contexts (shared logic)
func listContexts() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	var items []ContextListItem
	for _, nc := range cfg.Contexts {
		current := ""
		if nc.Name == cfg.CurrentContext {
			current = "*"
		}
		items = append(items, ContextListItem{
			Current:     current,
			Name:        nc.Name,
			Environment: nc.Context.Environment,
			SafetyLevel: nc.Context.SafetyLevel.String(),
			Description: nc.Context.Description,
		})
	}

	printer := NewPrinter()
	return printer.PrintList(items)
}

// useContext switches to a named context (shared logic)
func useContext(name string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		return err
	}

	found := false
	for _, nc := range cfg.Contexts {
		if nc.Name == name {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context %q not found", name)
	}

	cfg.CurrentContext = name

	if err := saveConfig(cfg); err != nil {
		return err
	}

	output.PrintSuccess("Switched to context %q", name)
	return nil
}

// describeContext shows detailed info about a named context (shared logic)
func describeContext(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	var found *config.NamedContext
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == name {
			found = &cfg.Contexts[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("context %q not found", name)
	}

	isCurrent := found.Name == cfg.CurrentContext
	currentMark := ""
	if isCurrent {
		currentMark = " (current)"
	}

	const w = 14
	output.DescribeKV("Name:", w, "%s%s", found.Name, currentMark)
	output.DescribeKV("Environment:", w, "%s", found.Context.Environment)
	output.DescribeKV("Token-Ref:", w, "%s", found.Context.TokenRef)
	output.DescribeKV("Safety Level:", w, "%s", found.Context.GetEffectiveSafetyLevel())

	switch found.Context.GetEffectiveSafetyLevel() {
	case config.SafetyLevelReadOnly:
		fmt.Printf("%*s(No modifications allowed)\n", w, "")
	case config.SafetyLevelReadWriteMine:
		fmt.Printf("%*s(Create/update/delete own resources)\n", w, "")
	case config.SafetyLevelReadWriteAll:
		fmt.Printf("%*s(Modify all resources, no bucket deletion)\n", w, "")
	case config.SafetyLevelDangerouslyUnrestricted:
		fmt.Printf("%*s(All operations including bucket deletion)\n", w, "")
	}

	if found.Context.Description != "" {
		output.DescribeKV("Description:", w, "%s", found.Context.Description)
	}

	return nil
}

// setContext creates or updates a named context (shared logic)
func setContext(name, environment, tokenRef, safetyLevel, description string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		cfg = config.NewConfig()
	}

	// Check if this is an update
	isUpdate := false
	for _, nc := range cfg.Contexts {
		if nc.Name == name {
			isUpdate = true
			if environment == "" {
				environment = nc.Context.Environment
			}
			break
		}
	}

	if !isUpdate && environment == "" {
		return fmt.Errorf("--environment is required for new contexts")
	}

	// Warn about potentially wrong environment URLs
	if environment != "" {
		if problems := diagnostic.CheckEnvironmentURL(environment); len(problems) > 0 {
			for _, p := range problems {
				output.PrintWarning("%s", p.Message)
				if p.SuggestedURL != "" {
					output.PrintHint("Did you mean: %s", p.SuggestedURL)
				}
			}
			fmt.Fprintln(os.Stderr)
		}
	}

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

	cfg.SetContextWithOptions(name, environment, tokenRef, opts)

	// Always activate the context that was just created or updated.
	// "ctx set" is the canonical way to configure a context, so the natural
	// expectation is that the named context becomes current afterward.
	cfg.CurrentContext = name

	if err := saveConfig(cfg); err != nil {
		return err
	}

	if isUpdate {
		output.PrintSuccess("Context %q updated and set as current", name)
	} else {
		output.PrintSuccess("Context %q created and set as current", name)
	}
	return nil
}

// deleteContext deletes a named context (shared logic)
func deleteContext(name string) error {
	cfg, err := loadConfigRaw()
	if err != nil {
		return err
	}

	found := false
	for _, nc := range cfg.Contexts {
		if nc.Name == name {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context %q not found", name)
	}

	if err := cfg.DeleteContext(name); err != nil {
		return err
	}

	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
		output.PrintWarning("Deleted the current context. Use 'dtctl ctx <name>' to set a new one.")
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	output.PrintSuccess("Context %q deleted", name)
	return nil
}

func init() {
	rootCmd.AddCommand(ctxCmd)

	ctxCmd.AddCommand(ctxTokenCmd)
	ctxCmd.AddCommand(ctxCurrentCmd)
	ctxCmd.AddCommand(ctxDescribeCmd)
	ctxCmd.AddCommand(ctxSetCmd)
	ctxCmd.AddCommand(ctxDeleteCmd)

	// Flags for ctx set
	ctxSetCmd.Flags().String("environment", "", "environment URL")
	ctxSetCmd.Flags().String("token-ref", "", "token reference name")
	ctxSetCmd.Flags().String("safety-level", "", "safety level (readonly, readwrite-mine, readwrite-all, dangerously-unrestricted)")
	ctxSetCmd.Flags().String("description", "", "human-readable description for this context")
}
