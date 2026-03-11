package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dynatrace-oss/dtctl/pkg/aidetect"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

var (
	cfgFile      string
	contextName  string
	outputFormat string
	verbosity    int
	debugMode    bool // --debug flag (alias for -vv)
	dryRun       bool
	plainMode    bool
	chunkSize    int64
	agentMode    bool // --agent/-A flag: wrap output in machine-readable envelope
	noAgent      bool // --no-agent flag: opt out of auto-detected agent mode
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

	// --- Alias resolution (before Cobra parses args) ---
	// Load config quietly; if it fails, skip alias resolution (the real
	// command will produce the proper error later).
	if cfg, err := config.Load(); err == nil {
		// os.Args[0] is the binary name; work with os.Args[1:]
		expanded, isShell, err := resolveAlias(os.Args[1:], cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}

		if isShell {
			if err := execShellAlias(expanded[0]); err != nil {
				os.Exit(1)
			}
			return
		}

		if expanded != nil {
			rootCmd.SetArgs(expanded)
		}
	}
	// --- End alias resolution ---

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

		if agentMode || plainMode {
			detail := errorToDetail(err)
			_ = output.PrintError(os.Stderr, detail)
			os.Exit(exitCodeForError(err))
		}

		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(exitCodeForError(err))
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
	cmd.SetFlagErrorFunc(enhanceFlagError)

	// Recursively setup for all subcommands
	for _, sub := range cmd.Commands() {
		setupErrorHandlers(sub)
	}
}

// errorToDetail converts any error into a structured ErrorDetail for agent/plain mode output.
// It uses errors.As to extract rich context from typed errors when available.
func errorToDetail(err error) *output.ErrorDetail {
	// diagnostic.Error — wraps API errors with operation context and suggestions
	var diagErr *diagnostic.Error
	if errors.As(err, &diagErr) {
		code := output.ClassifyHTTPError(diagErr.StatusCode)
		if diagErr.StatusCode == 0 {
			code = "error"
		}
		return &output.ErrorDetail{
			Code:        code,
			Message:     diagErr.Message,
			Operation:   diagErr.Operation,
			StatusCode:  diagErr.StatusCode,
			RequestID:   diagErr.RequestID,
			Suggestions: diagErr.Suggestions,
		}
	}

	// client.APIError — raw API error without diagnostic wrapping
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		msg := apiErr.Message
		if apiErr.Details != "" {
			msg += " - " + apiErr.Details
		}
		return &output.ErrorDetail{
			Code:       output.ClassifyHTTPError(apiErr.StatusCode),
			Message:    msg,
			StatusCode: apiErr.StatusCode,
		}
	}

	// safety.SafetyError — operation blocked by safety level
	var safetyErr *safety.SafetyError
	if errors.As(err, &safetyErr) {
		return &output.ErrorDetail{
			Code:        "safety_blocked",
			Message:     safetyErr.Reason,
			Suggestions: safetyErr.Suggestions,
		}
	}

	// suggest.CommandError — unknown command with "did you mean?" suggestions
	var cmdErr *suggest.CommandError
	if errors.As(err, &cmdErr) {
		detail := &output.ErrorDetail{
			Code:    "unknown_command",
			Message: cmdErr.Message,
		}
		if cmdErr.Suggestion != nil {
			detail.Suggestions = []string{
				fmt.Sprintf("did you mean %q?", cmdErr.Suggestion.Value),
			}
		}
		return detail
	}

	// suggest.FlagError — unknown flag with "did you mean?" suggestion
	var flagErr *suggest.FlagError
	if errors.As(err, &flagErr) {
		detail := &output.ErrorDetail{
			Code:    "unknown_command",
			Message: flagErr.Message,
		}
		if flagErr.Suggestion != nil {
			detail.Suggestions = []string{
				fmt.Sprintf("did you mean --%s?", flagErr.Suggestion.Value),
			}
		}
		return detail
	}

	// Fallback — generic error with no structured context
	return &output.ErrorDetail{
		Code:    classifyGenericError(err),
		Message: err.Error(),
	}
}

// classifyGenericError attempts to classify an error by inspecting its message
// when no typed error is available.
func classifyGenericError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no active context") || strings.Contains(msg, "no context"):
		return "context_error"
	case strings.Contains(msg, "config") || strings.Contains(msg, "configuration"):
		return "config_error"
	case strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "validation") || strings.Contains(msg, "invalid"):
		return "validation_error"
	default:
		return "error"
	}
}

// exitCodeForError returns the appropriate process exit code for an error.
// Uses typed exit codes from client.APIError and diagnostic.Error when available,
// falling back to ExitUsageError for command/flag errors and ExitError for everything else.
func exitCodeForError(err error) int {
	var diagErr *diagnostic.Error
	if errors.As(err, &diagErr) {
		return diagErr.ExitCode()
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ExitCode()
	}

	var cmdErr *suggest.CommandError
	if errors.As(err, &cmdErr) {
		return client.ExitUsageError
	}

	var flagErr *suggest.FlagError
	if errors.As(err, &flagErr) {
		return client.ExitUsageError
	}

	return client.ExitError
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

// NewSafetyChecker creates a new safety checker for the current context
func NewSafetyChecker(cfg *config.Config) (*safety.Checker, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}

	return safety.NewChecker(cfg.CurrentContext, ctx), nil
}

// NewPrinter creates a new printer respecting agent and plain mode settings
func NewPrinter() output.Printer {
	if agentMode {
		ctx := &output.ResponseContext{}
		return output.NewAgentPrinter(os.Stdout, ctx)
	}
	return output.NewPrinterWithOptions(outputFormat, os.Stdout, plainMode)
}

// enrichAgent configures agent-mode metadata on the printer if agent mode is active.
// It is a no-op when the printer is not an AgentPrinter. Returns the AgentPrinter
// for further customization (or nil if not in agent mode).
func enrichAgent(printer output.Printer, verb, resource string) *output.AgentPrinter {
	ap, ok := printer.(*output.AgentPrinter)
	if !ok {
		return nil
	}
	ap.Context().Verb = verb
	ap.SetResource(resource)
	return ap
}

// GetAgentMode returns the current agent mode setting
func GetAgentMode() bool {
	return agentMode
}

// LoadConfig loads the config and applies the --context flag override if provided
func LoadConfig() (*config.Config, error) {
	var cfg *config.Config
	var err error

	// Load from specified config file or default location
	if cfgFile != "" {
		cfg, err = config.LoadFrom(cfgFile)
	} else {
		cfg, err = config.Load()
	}

	if err != nil {
		return nil, err
	}

	// Override current context if --context flag is provided
	if contextName != "" {
		cfg.CurrentContext = contextName
	}

	return cfg, nil
}

// NewClientFromConfig creates a new client from config with verbose mode configured
func NewClientFromConfig(cfg *config.Config) (*client.Client, error) {
	c, err := client.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	// If --debug flag is set, force verbosity to 2 (full debug mode)
	if debugMode {
		c.SetVerbosity(2)
	} else {
		c.SetVerbosity(verbosity)
	}
	return c, nil
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (searches .dtctl.yaml upward, then $XDG_CONFIG_HOME/dtctl/config)")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "use a specific context")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: json|yaml|csv|table|wide")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbose output (-v for details, -vv for full debug including auth headers)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode (full HTTP request/response logging, equivalent to -vv)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	rootCmd.PersistentFlags().BoolVar(&plainMode, "plain", false, "plain output for machine processing (no colors, no interactive prompts)")
	rootCmd.PersistentFlags().BoolVarP(&agentMode, "agent", "A", false, "agent output mode: wrap output in a structured JSON envelope with metadata")
	rootCmd.PersistentFlags().BoolVar(&noAgent, "no-agent", false, "disable auto-detected agent mode")
	rootCmd.PersistentFlags().Int64Var(&chunkSize, "chunk-size", 500, "Return large lists in chunks rather than all at once. Pass 0 to disable.")

	// Bind flags to viper
	_ = viper.BindPFlag("context", rootCmd.PersistentFlags().Lookup("context"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	// Auto-detect AI agent environment and enable agent mode
	if !agentMode && !noAgent {
		if info := aidetect.Detect(); info.Detected {
			// Only auto-enable if user hasn't explicitly chosen a non-JSON output format
			outputFlag := rootCmd.PersistentFlags().Lookup("output")
			if outputFlag == nil || !outputFlag.Changed {
				agentMode = true
			}
		}
	}

	// Agent mode implies plain mode (no colors, no interactive prompts)
	if agentMode {
		plainMode = true
	}

	// Propagate plain mode to the output package so ColorEnabled() respects --plain
	if plainMode {
		output.SetPlainMode(true)
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Check for local config first (.dtctl.yaml in current or parent directories)
		localConfig := config.FindLocalConfig()
		if localConfig != "" {
			viper.SetConfigFile(localConfig)
		} else {
			// Fall back to XDG-compliant config directory
			configDir := config.ConfigDir()
			viper.AddConfigPath(configDir)

			viper.SetConfigType("yaml")
			viper.SetConfigName("config")
		}
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
