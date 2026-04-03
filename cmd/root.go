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
	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
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
			output.PrintHumanError("%s", err)
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

		// Check for URL-related hints (e.g., wrong domain like live.dynatrace.com)
		urlHints := getURLHintsForError(err)

		// Check for auth-related hints (e.g., expired OAuth session)
		authHints := getAuthHintsForError(err)

		allHints := make([]string, 0, len(urlHints)+len(authHints))
		allHints = append(allHints, urlHints...)
		allHints = append(allHints, authHints...)

		if agentMode || plainMode {
			detail := errorToDetail(err)
			detail.Suggestions = append(detail.Suggestions, allHints...)
			_ = output.PrintError(os.Stderr, detail)
			os.Exit(exitCodeForError(err))
		}

		output.PrintHumanError("%s", err)
		if len(allHints) > 0 {
			fmt.Fprintln(os.Stderr)
			for _, hint := range allHints {
				output.PrintHint("%s", hint)
			}
		}
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

	// apply.HookRejectedError — pre-apply hook rejected the resource
	var hookErr *apply.HookRejectedError
	if errors.As(err, &hookErr) {
		return &output.ErrorDetail{
			Code:    "hook_rejected",
			Message: "pre-apply hook rejected the resource",
			Suggestions: []string{
				"check hook stderr output for details",
				"use --no-hooks to skip pre-apply hooks",
			},
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

// getURLHintsForError checks whether the current context's environment URL
// has known problems (e.g., live.dynatrace.com instead of apps.dynatrace.com)
// and returns actionable hints. Only returns hints for errors that could
// plausibly be caused by a wrong URL (403, 401, connectivity, auth failures).
func getURLHintsForError(err error) []string {
	// Only provide URL hints for errors that could be caused by wrong URL
	if !isURLRelatedError(err) {
		return nil
	}

	// Try to load config quietly — if we can't, there's nothing to check
	cfg, cfgErr := LoadConfig()
	if cfgErr != nil {
		return nil
	}
	ctx, ctxErr := cfg.CurrentContextObj()
	if ctxErr != nil {
		return nil
	}

	return diagnostic.URLSuggestions(ctx.Environment)
}

// getAuthHintsForError returns actionable hints when the error looks like an
// OAuth token refresh failure (e.g., expired session, revoked refresh token).
func getAuthHintsForError(err error) []string {
	if !isTokenRefreshError(err) {
		return nil
	}
	return []string{
		"Run 'dtctl auth login' to re-authenticate",
	}
}

// isTokenRefreshError returns true if the error looks like an OAuth token
// refresh failure (expired session, invalid grant, etc.).
func isTokenRefreshError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to refresh token") ||
		strings.Contains(msg, "token expired and refresh failed")
}

// isURLRelatedError returns true if the error could plausibly be caused by
// using the wrong environment URL (e.g., 403, 401, connectivity errors).
func isURLRelatedError(err error) bool {
	// Check typed errors for status codes
	var diagErr *diagnostic.Error
	if errors.As(err, &diagErr) {
		return diagErr.StatusCode == 401 || diagErr.StatusCode == 403
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}

	// Check untyped error messages (since resource handlers use fmt.Errorf)
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access denied") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "cannot reach") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host")
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

// Setup creates a Config, Client, and Printer for read-only commands.
// It consolidates the common LoadConfig → NewClientFromConfig → NewPrinter boilerplate.
func Setup() (*config.Config, *client.Client, output.Printer, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, c, NewPrinter(), nil
}

// SetupClient creates a Config and Client without a Printer.
// Use this for commands that need the client but handle output differently
// (e.g., exec commands, log streaming, or commands with conditional printers).
func SetupClient() (*config.Config, *client.Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, c, nil
}

// SetupWithSafety creates a Config + Client for mutating commands, performing a safety
// check before the client is created. Use this for commands where ownership is unknown
// (i.e., the resource doesn't need to be fetched first to determine the owner).
// A Printer is not included because many mutating commands don't use one.
func SetupWithSafety(op safety.Operation) (*config.Config, *client.Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	checker, err := NewSafetyChecker(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := checker.CheckError(op, safety.OwnershipUnknown); err != nil {
		return nil, nil, err
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, c, nil
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
		ap := output.NewAgentPrinter(os.Stdout, ctx)
		// If the user explicitly requested an output format via -o,
		// use that format for the result field inside the agent envelope
		// (e.g. -o toon for token-efficient encoding).
		outputFlag := rootCmd.PersistentFlags().Lookup("output")
		if outputFlag != nil && outputFlag.Changed {
			ap.SetResultFormat(outputFormat)
		}
		return ap
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

// NewDQLExecutorFromConfig creates a DQL executor from a config and client, with OAuth
// token refresh support. When the OAuth token expires during a long-running query poll
// (which can exceed the 5-minute token lifetime), the executor automatically fetches a
// fresh token and retries without aborting the query.
func NewDQLExecutorFromConfig(cfg *config.Config, c *client.Client) *exec.DQLExecutor {
	executor := exec.NewDQLExecutor(c)
	if config.IsKeyringAvailable() {
		ctx, err := cfg.CurrentContextObj()
		if err == nil && ctx.TokenRef != "" {
			tokenRef := ctx.TokenRef
			executor = executor.WithTokenRefresher(func() (string, error) {
				return client.GetTokenWithOAuthSupport(cfg, tokenRef)
			})
		}
	}
	return executor
}

func init() {
	cobra.OnInitialize(initConfig)

	// Register template functions for help/usage formatting
	cobra.AddTemplateFunc("bold", func(s string) string {
		return output.Colorize(output.Bold, s)
	})

	// Custom usage template with bold section headers.
	// NOTE: This is a copy of Cobra's default usage template with {{bold ...}} wrappers.
	// If upgrading Cobra, compare against the upstream default template for changes:
	//   https://github.com/spf13/cobra/blob/main/command.go (search "usageTemplate")
	rootCmd.SetUsageTemplate(`{{bold "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{bold "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{bold "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{bold "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{bold .Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{bold "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{bold "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{bold "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{bold "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (searches .dtctl.yaml upward, then $XDG_CONFIG_HOME/dtctl/config)")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "use a specific context")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: json|yaml|csv|toon|table|wide")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbose output (-v for details, -vv for full debug including auth headers)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode (full HTTP request/response logging, equivalent to -vv)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	rootCmd.PersistentFlags().BoolVar(&plainMode, "plain", false, "plain output for machine processing (no colors, no interactive prompts)")
	rootCmd.PersistentFlags().BoolVarP(&agentMode, "agent", "A", false, "agent output mode: wrap output in a structured JSON envelope with metadata")
	rootCmd.PersistentFlags().BoolVar(&noAgent, "no-agent", false, "disable auto-detected agent mode")
	rootCmd.PersistentFlags().Int64Var(&chunkSize, "chunk-size", 500, "Paginate through all results in chunks of this size. 0 returns only the first page.")

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
