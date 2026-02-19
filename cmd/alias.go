package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// isBuiltinCommand returns true if name matches any registered Cobra command.
func isBuiltinCommand(name string) bool {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage command aliases",
	Long: `Create, list, and delete shorthand names for dtctl commands.

Aliases expand before command parsing, so they work exactly like typing
the full command. Use positional parameters ($1, $2, ...) for reusable
templates, or prefix with ! for shell expansion.

Examples:
  # Simple alias
  dtctl alias set prod-wf "get workflows --context=production"
  dtctl prod-wf

  # Parameterized alias
  dtctl alias set wf 'get workflow $1 --context=production'
  dtctl wf my-workflow-id

  # Shell alias (pipes, jq, etc.)
  dtctl alias set wf-count '!dtctl get workflows -o json | jq length'
  dtctl wf-count`,
}

var aliasSetCmd = &cobra.Command{
	Use:   "set <name> <expansion>",
	Short: "Create or update an alias",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, expansion := args[0], args[1]

		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		if err := cfg.SetAlias(name, expansion, isBuiltinCommand); err != nil {
			return err
		}

		if err := saveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Alias %q set to %q\n", name, expansion)
		return nil
	},
}

var aliasListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all aliases",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		entries := cfg.ListAliases()
		if len(entries) == 0 {
			fmt.Println("No aliases configured.")
			fmt.Println("Use 'dtctl alias set <name> <command>' to create one.")
			return nil
		}

		printer := NewPrinter()
		return printer.PrintList(entries)
	},
}

var aliasDeleteCmd = &cobra.Command{
	Use:     "delete <name> [name...]",
	Short:   "Delete one or more aliases",
	Aliases: []string{"rm"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		for _, name := range args {
			if err := cfg.DeleteAlias(name); err != nil {
				return err
			}
			fmt.Printf("Alias %q deleted\n", name)
		}

		return saveConfig(cfg)
	},
}

var aliasExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export aliases to a YAML file",
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		if len(cfg.Aliases) == 0 {
			return fmt.Errorf("no aliases to export")
		}

		if err := cfg.ExportAliases(file); err != nil {
			return err
		}

		fmt.Printf("Exported %d alias(es) to %s\n", len(cfg.Aliases), file)
		return nil
	},
}

var aliasImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import aliases from a YAML file",
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		overwrite, _ := cmd.Flags().GetBool("overwrite")

		if file == "" {
			return fmt.Errorf("--file is required")
		}

		cfg, err := loadConfigRaw()
		if err != nil {
			return err
		}

		conflicts, err := cfg.ImportAliases(file, overwrite, isBuiltinCommand)
		if err != nil {
			return err
		}

		if len(conflicts) > 0 && !overwrite {
			fmt.Printf("Skipped %d existing alias(es): %s\n",
				len(conflicts), strings.Join(conflicts, ", "))
			fmt.Println("Use --overwrite to replace existing aliases.")
		}

		if err := saveConfig(cfg); err != nil {
			return err
		}

		fmt.Println("Aliases imported successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(aliasCmd)

	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)
	aliasCmd.AddCommand(aliasExportCmd)
	aliasCmd.AddCommand(aliasImportCmd)

	aliasExportCmd.Flags().StringP("file", "f", "", "output file path")
	aliasImportCmd.Flags().StringP("file", "f", "", "input file path")
	aliasImportCmd.Flags().Bool("overwrite", false, "overwrite existing aliases")
}
