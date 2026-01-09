package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	cfgFile      string
	contextName  string
	outputFormat string
	verbosity    int
	dryRun       bool
	namespace    string
	plainMode    bool
	chunkSize    int64
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:           "dtctl",
	Short:         "Dynatrace platform CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `dtctl is a kubectl-inspired CLI tool for managing Dynatrace platform resources.

It provides a consistent interface for interacting with workflows, documents,
SLOs, queries, and other Dynatrace platform capabilities.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Setup enhanced error handling after all subcommands are registered
	setupErrorHandlers(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		errStr := err.Error()

		// Enhance unknown command errors with suggestions
		if strings.Contains(errStr, "unknown command") {
			err = enhanceCommandError(rootCmd, err)
		}

		// Enhance unknown flag errors with suggestions
		if strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand flag") {
			err = enhanceFlagError(rootCmd, err)
		}

		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// collectFlags gathers all flag names from a command and its parents
func collectFlags(cmd *cobra.Command) []string {
	var flags []string
	seen := make(map[string]bool)

	addFlags := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			if !seen[f.Name] {
				flags = append(flags, f.Name)
				seen[f.Name] = true
			}
		})
	}

	// Collect from current command and all parents
	for c := cmd; c != nil; c = c.Parent() {
		addFlags(c.Flags())
		addFlags(c.PersistentFlags())
	}

	return flags
}

// collectSubcommands gathers all subcommand names and aliases
func collectSubcommands(cmd *cobra.Command) []string {
	var commands []string
	for _, sub := range cmd.Commands() {
		commands = append(commands, sub.Name())
		commands = append(commands, sub.Aliases...)
	}
	return commands
}

// enhanceFlagError adds suggestions to flag errors
func enhanceFlagError(cmd *cobra.Command, err error) error {
	errStr := err.Error()

	// Handle unknown flag errors
	if strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand flag") {
		flags := collectFlags(cmd)
		return suggest.ParseFlagError(errStr, flags)
	}

	return err
}

// enhanceCommandError adds suggestions to unknown command errors
func enhanceCommandError(cmd *cobra.Command, err error) error {
	errStr := err.Error()

	// Handle unknown command errors
	if strings.Contains(errStr, "unknown command") {
		commands := collectSubcommands(cmd)
		return suggest.ParseCommandError(errStr, commands)
	}

	return err
}

// setupErrorHandlers configures enhanced error handling for a command and its children
func setupErrorHandlers(cmd *cobra.Command) {
	// Set flag error function for this command
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return enhanceFlagError(c, err)
	})

	// Recursively setup for all subcommands
	for _, sub := range cmd.Commands() {
		setupErrorHandlers(sub)
	}
}

// requireSubcommand returns an error with suggestions when a subcommand is required but not provided or invalid
func requireSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Build a helpful message showing available resources
		var resources []string
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				name := sub.Name()
				if len(sub.Aliases) > 0 {
					name += " (" + sub.Aliases[0] + ")"
				}
				resources = append(resources, name)
			}
		}
		return fmt.Errorf("requires a resource type\n\nAvailable resources:\n  %s\n\nUsage:\n  %s <resource> [id] [flags]",
			strings.Join(resources, "\n  "), cmd.CommandPath())
	}

	// Check if the first arg looks like an unknown subcommand
	subcommands := collectSubcommands(cmd)
	suggestion := suggest.FindClosest(args[0], subcommands)

	if suggestion != nil {
		return fmt.Errorf("unknown resource type %q, did you mean %q?", args[0], suggestion.Value)
	}

	return fmt.Errorf("unknown resource type %q\nRun '%s --help' for available resources", args[0], cmd.CommandPath())
}

// GetPlainMode returns the current plain mode setting
func GetPlainMode() bool {
	return plainMode
}

// GetChunkSize returns the current chunk size setting for pagination
func GetChunkSize() int64 {
	return chunkSize
}

// NewPrinter creates a new printer respecting plain mode setting
func NewPrinter() output.Printer {
	return output.NewPrinterWithOptions(outputFormat, os.Stdout, plainMode)
}

// NewClientFromConfig creates a new client from config with verbose mode configured
func NewClientFromConfig(cfg *config.Config) (*client.Client, error) {
	c, err := client.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	c.SetVerbosity(verbosity)
	return c, nil
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $XDG_CONFIG_HOME/dtctl/config)")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "use a specific context")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: json|yaml|csv|table|wide")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbose output (-v for details, -vv for full debug including auth headers)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "namespace for scoping")
	rootCmd.PersistentFlags().BoolVar(&plainMode, "plain", false, "plain output for machine processing (no colors, no interactive prompts)")
	rootCmd.PersistentFlags().Int64Var(&chunkSize, "chunk-size", 500, "Return large lists in chunks rather than all at once. Pass 0 to disable.")

	// Bind flags to viper
	viper.BindPFlag("context", rootCmd.PersistentFlags().Lookup("context"))
	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Use XDG-compliant config directory
		configDir := config.ConfigDir()
		viper.AddConfigPath(configDir)

		// Also check legacy path for backwards compatibility
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(filepath.Join(home, ".dtctl"))
		}

		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("DTCTL")

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		if verbosity > 0 {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
