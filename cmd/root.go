package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/dynatrace-oss/dtctl/pkg/aidetect"
	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/metrics"
	"github.com/dynatrace-oss/dtctl/pkg/inspect"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
	"github.com/dynatrace-oss/dtctl/pkg/tracing"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
	sdkauth "github.com/dynatrace-oss/dtctl/sdk/auth"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

var (
	cfgFile      string
	contextName  string
	outputFormat string
	jqFilter     string
	verbosity    int
	debugMode    bool // --debug flag (alias for -vv)
	dryRun       bool
	plainMode    bool
	chunkSize    int64
	agentMode    bool // --agent/-A flag: wrap output in machine-readable envelope
	noAgent      bool // --no-agent flag: opt out of auto-detected agent mode

	// tracingRootCtx holds the context carrying the root OTel span for this
	// invocation. Set by execute() and read by NewClientFromConfig to inject
	// W3C trace context headers on outgoing Dynatrace API requests.
	//
	// This is a package-level variable (rather than a function parameter) because
	// NewClientFromConfig is referenced as a function value in breakpoint_helpers.go
	// and changing its signature would cascade across 100+ call sites. The global is
	// acceptable here because dtctl is a single-invocation CLI: execute() sets it
	// once before any client is created, and the process exits shortly after.
	tracingRootCtx context.Context
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:           "dtctl",
	Short:         "Dynatrace platform CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return validateGlobalFlags()
	},
	Long: `dtctl is a kubectl-inspired CLI tool for managing Dynatrace platform resources.

It provides a consistent interface for interacting with workflows, documents,
SLOs, queries, and other Dynatrace platform capabilities.`,
}

// validateGlobalFlags enforces cross-command constraints for root persistent flags.
func validateGlobalFlags() error {
	if jqFilter == "" {
		return nil
	}

	outputFormat = output.NormalizeJQOutputFormat(outputFormat)
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	os.Exit(execute())
}

// execute runs the CLI and returns an exit code. Separating it from Execute
// ensures that deferred functions (e.g. tracing shutdown/flush) run before
// os.Exit is called, which os.Exit would otherwise bypass.
func execute() int {
	// Setup enhanced error handling after all subcommands are registered
	setupErrorHandlers(rootCmd)

	// Wrap runnable commands with the token-scope preflight (--check-scopes and
	// agent-mode auto-preflight). Must run after all subcommands are registered.
	installScopePreflight(rootCmd)

	// --- Alias resolution (before Cobra parses args AND before tracing init) ---
	// Resolving aliases first ensures the span name reflects the real command,
	// not the pre-expansion alias. Load config quietly; if it fails, skip alias
	// resolution (the real command will produce the proper error later).
	spanArgs := os.Args[1:]
	if cfg, err := config.Load(); err == nil {
		// Security: warn when an auto-discovered local .dtctl.yaml carries
		// code-execution keys (aliases / apply hooks) that are ignored. This
		// makes adoption of an untrusted per-project config visible instead of
		// silent. See config.Load / markLocal.
		if cfg.IgnoredExecKeys() {
			fmt.Fprintf(os.Stderr,
				"warning: ignoring aliases and hooks from local config %q "+
					"(honored only from the global config, --config, or DTCTL_CONFIG)\n",
				cfg.LocalConfigPath())
		}

		// os.Args[0] is the binary name; work with os.Args[1:]
		expanded, isShell, err := resolveAlias(os.Args[1:], cfg)
		if err != nil {
			output.PrintHumanError("%s", err)
			return 1
		}

		if isShell {
			if err := execShellAlias(expanded[0]); err != nil {
				return 1
			}
			return 0
		}

		if expanded != nil {
			rootCmd.SetArgs(expanded)
			spanArgs = expanded
		}
	}
	// --- End alias resolution ---

	// --- Command profile filter ---
	// Resolve the active profile (DTCTL_PROFILE > context binding > full) and
	// mask out-of-profile commands before Cobra dispatches, so help, the
	// `commands` catalog, and completion all reflect the reduced surface. A
	// nil profile is the full tree (backward compatible). An unknown profile
	// name is a hard error rather than a silent surface expansion.
	prof, profErr := resolveActiveProfile(spanArgs)
	if profErr != nil {
		output.PrintHumanError("%s", profErr)
		return exitCodeForError(profErr)
	}
	applyProfile(rootCmd, prof)
	// --- End command profile filter ---

	// Initialise OpenTelemetry tracing. Done after alias resolution so that
	// the span name reflects the actual command (not a pre-alias invocation).
	// The root span covers the entire invocation; shutdown flushes buffered
	// spans before the process exits (critical for short-lived processes that
	// OneAgent cannot instrument).
	spanName := buildSpanName(spanArgs)
	safeArgs := extractSafeArgs(spanArgs)
	tracingCtx, shutdownTracing, tracingErr := tracing.Init(
		context.Background(), spanName, safeArgs, verbosity,
	)
	tracingRootCtx = tracingCtx
	rootSpan := trace.SpanFromContext(tracingCtx)
	commandStart := time.Now()
	collector := metrics.Default()
	var runErr error
	defer func() {
		collector.RecordCommand(spanName, time.Since(commandStart), runErr)
	}()
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(flushCtx)
	}()
	if tracingErr != nil {
		// Non-fatal: warn and continue. The CLI still works; spans may not export.
		fmt.Fprintf(os.Stderr, "dtctl: tracing: %v (check OTEL_EXPORTER_OTLP_ENDPOINT or unset it to disable export)\n", tracingErr)
	}

	if err := rootCmd.Execute(); err != nil {
		// silentExitError carries an exit code only (e.g. --check-scopes already
		// printed its verdict); set the status and return without re-printing.
		var silent *silentExitError
		if errors.As(err, &silent) {
			if silent.code == 0 {
				rootSpan.SetStatus(codes.Ok, "")
				runErr = nil
			} else {
				rootSpan.SetStatus(codes.Error, "insufficient scope")
				runErr = err
			}
			return silent.code
		}

		runErr = err
		errStr := err.Error()

		// Unknown top-level commands get one shot at plugin dispatch before
		// the suggestion enhancer: `dtctl foo` execs dtctl-foo from PATH if
		// present (kubectl semantics; built-ins always win because they never
		// reach this error path). See docs/dev/PLUGIN_CONVENTIONS.md.
		if strings.Contains(errStr, "unknown command") {
			if code, handled := tryPluginDispatch(spanArgs); handled {
				return code
			}
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

		// Record the error on the root span so it appears in traces.
		rootSpan.SetStatus(codes.Error, err.Error())
		rootSpan.RecordError(err)

		if agentMode || plainMode {
			detail := errorToDetail(err)
			detail.Suggestions = append(detail.Suggestions, allHints...)
			// Agent/plain mode: error envelopes go to stdout (not stderr) because
			// machine consumers read all structured output — success and failure — from
			// stdout. Relying on stderr for structured error data is unreliable in these
			// modes; consumers must parse stdout for the full response envelope.
			_ = output.PrintError(os.Stdout, detail)
			return exitCodeForError(err)
		}

		output.PrintHumanError("%s", err)
		if len(allHints) > 0 {
			fmt.Fprintln(os.Stderr)
			for _, hint := range allHints {
				output.PrintHint("%s", hint)
			}
		}
		return exitCodeForError(err)
	}
	rootSpan.SetStatus(codes.Ok, "")
	return 0
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

var (
	unknownFlagRe = regexp.MustCompile(`unknown (?:shorthand )?flag: ['-]*(\w+)['-]*`)
	unknownCmdRe  = regexp.MustCompile(`unknown command "(\w+)"`)
)

// enhanceFlagError adds suggestions to flag errors
func enhanceFlagError(cmd *cobra.Command, err error) error {
	errStr := err.Error()

	// Handle unknown flag errors
	if strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand flag") {
		if m := unknownFlagRe.FindStringSubmatch(errStr); len(m) == 2 {
			if fe := adviseFlag(cmd, m[1]); fe != nil {
				return fe
			}
		}
		flags := collectFlags(cmd)
		return suggest.ParseFlagError(errStr, flags)
	}

	return err
}

// adviseFlag handles flags agents carry over from other CLIs, where the
// closest-name suggestion misleads (evals saw --limit → "did you mean --live?").
func adviseFlag(cmd *cobra.Command, flag string) *suggest.FlagError {
	switch {
	case flag == "format":
		return &suggest.FlagError{Flag: flag,
			Message:    "unknown flag --format",
			Suggestion: &suggest.Suggestion{Value: "output"}}
	case cmd.Name() == "query" && flag == "limit":
		return &suggest.FlagError{Flag: flag,
			Message: "unknown flag --limit — DQL limits rows inside the query text: append `| limit N`"}
	case cmd.Name() == "query" && flag == "query":
		return &suggest.FlagError{Flag: flag,
			Message: "unknown flag --query — pass the DQL text as the positional argument: dtctl query 'fetch ...'"}
	}
	return nil
}

// verbSynonyms maps verbs agents guess from other CLIs to the dtctl verb that
// does the job; the edit-distance fallback suggests nonsense for these
// (evals saw `list` → "did you mean alias?").
var verbSynonyms = map[string]struct{ verb, hint string }{
	"list":   {"get", "resources are listed with `dtctl get <resource>`; data is queried with `dtctl query '<DQL>'` (catalog: dtctl commands)"},
	"ls":     {"get", "resources are listed with `dtctl get <resource>` (catalog: dtctl commands)"},
	"show":   {"describe", "use `dtctl get <resource>` for lists, `dtctl describe <resource> <name>` for one item's detail"},
	"search": {"query", "search data with DQL: dtctl query 'fetch logs | filter contains(content, \"...\")'"},
	"remove": {"delete", "use `dtctl delete <resource> <id>`"},
	"rm":     {"delete", "use `dtctl delete <resource> <id>`"},
	// DQL commands agents promote to dtctl commands (evals: `dtctl smartscapeNodes ...`).
	"smartscapeNodes": {"query", `smartscapeNodes is a DQL command — run it through query: dtctl query 'smartscapeNodes "HOST" | limit 10'`},
	"smartscapeEdges": {"query", `smartscapeEdges is a DQL command — run it through query: dtctl query 'smartscapeEdges "runs_on" | limit 10'`},
	"fetch":           {"query", "fetch is DQL — run it through query: dtctl query 'fetch logs | limit 10'"},
	"timeseries":      {"query", "timeseries is DQL — run it through query: dtctl query 'timeseries avg(dt.host.cpu.usage), from:now()-1h'"},
}

// enhanceCommandError adds suggestions to unknown command errors
func enhanceCommandError(cmd *cobra.Command, err error) error {
	errStr := err.Error()

	// Handle unknown command errors
	if strings.Contains(errStr, "unknown command") {
		if m := unknownCmdRe.FindStringSubmatch(errStr); len(m) == 2 {
			if syn, ok := verbSynonyms[m[1]]; ok {
				return &suggest.CommandError{
					Command:    m[1],
					Message:    fmt.Sprintf("unknown command %q", m[1]),
					Suggestion: &suggest.Suggestion{Value: syn.verb},
					UsageHint:  syn.hint,
				}
			}
		}
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

// coreStreams are Grail data objects agents most often need. Used to answer
// UNKNOWN_DATA_OBJECT guesses with real names instead of leaving the agent
// to enumerate variants (evals: usersession→user_sessions→rum_events→…, ten
// failed guesses, then "0 RUM events" reported as the answer).
var coreStreams = []string{
	"logs", "spans", "events", "bizevents",
	"user.events", "user.sessions",
	"security.events",
	"dt.davis.events", "dt.davis.problems",
	"metric.series",
	"dt.system.buckets", "dt.system.data_objects", "dt.system.events",
}

// streamKeywords routes an unknown data-object name to the streams agents
// were actually looking for, by topic. Checked before edit distance because
// the guesses are usually semantic (rum_events), not typos.
var streamKeywords = []struct {
	keys    []string
	streams []string
}{
	{[]string{"session", "rum", "useraction", "user_action", "user.action"}, []string{"user.sessions", "user.events"}},
	{[]string{"user"}, []string{"user.events", "user.sessions"}},
	{[]string{"metric"}, []string{"metric.series"}},
	{[]string{"vuln", "security", "compliance", "detection"}, []string{"security.events"}},
	{[]string{"problem"}, []string{"dt.davis.problems"}},
	{[]string{"davis"}, []string{"dt.davis.events", "dt.davis.problems"}},
	{[]string{"trace", "span"}, []string{"spans"}},
	{[]string{"log"}, []string{"logs"}},
	{[]string{"bizevent", "business"}, []string{"bizevents"}},
	{[]string{"bucket"}, []string{"dt.system.buckets"}},
}

// unknownObjectRe pulls the offending name out of the API's
// "<name> isn't a valid data object." detail.
var unknownObjectRe = regexp.MustCompile(`(\S+) isn't a valid data object`)

// nearestStreams suggests real stream names for an unknown data-object guess:
// keyword routing first, edit distance over coreStreams as fallback.
func nearestStreams(name string) []string {
	n := strings.ToLower(strings.Trim(name, `"'`))
	for _, kw := range streamKeywords {
		for _, k := range kw.keys {
			if strings.Contains(n, k) {
				return kw.streams
			}
		}
	}
	norm := func(s string) string {
		return strings.NewReplacer(".", "", "_", "", "-", "").Replace(strings.ToLower(s))
	}
	var best []string
	for _, s := range coreStreams {
		if d := levenshtein(norm(n), norm(s)); d <= 3 {
			best = append(best, s)
		}
	}
	return best
}

// levenshtein is a plain edit distance; inputs here are short stream names.
func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// dqlErrorAdvice maps recurring DQL mistake classes (observed in agent evals)
// to recovery suggestions carried in the error envelope.
func dqlErrorAdvice(e *sdkquery.QueryError) []string {
	text := e.Error()
	var s []string
	switch {
	case strings.Contains(text, "smartscapeNode") || strings.Contains(text, "smartscapeEdge") ||
		strings.Contains(text, "smartscape.nodes") || strings.Contains(text, "smartscape.edges"):
		s = append(s, `smartscape is queried via the COMMANDS smartscapeNodes/smartscapeEdges, not fetch — start the query with them: dtctl query 'smartscapeNodes "HOST" | limit 10'`)
	case e.ErrorType == "UNKNOWN_DATA_OBJECT" && strings.Contains(text, "dt.entity."):
		s = append(s, `for a current-state entity census use: dtctl query 'smartscapeNodes "<TYPE>" | summarize count()' — dt.entity.* tables are event-lookback views and exist only for some types`)
	case e.ErrorType == "UNKNOWN_DATA_OBJECT":
		if m := unknownObjectRe.FindStringSubmatch(text); len(m) == 2 {
			if near := nearestStreams(m[1]); len(near) > 0 {
				s = append(s, fmt.Sprintf("no data object named %q — closest real streams: %s. The full catalog: dtctl query 'fetch dt.system.data_objects | fields name'", m[1], strings.Join(near, ", ")))
			} else {
				s = append(s, fmt.Sprintf("no data object named %q — common streams: %s. The full catalog: dtctl query 'fetch dt.system.data_objects | fields name'", m[1], strings.Join(coreStreams, ", ")))
			}
		}
	}
	if e.ErrorType == "FIELD_DOES_NOT_EXIST" &&
		(strings.Contains(text, "toRelationships") || strings.Contains(text, "fromRelationships")) {
		s = append(s, `toRelationships/fromRelationships are classic Environment-API fields, not DQL — topology hops use smartscapeEdges: dtctl query 'smartscapeEdges "runs_on" | filter in(source_id, {toSmartscapeId("SERVICE-…")}) | fields target_id'`)
	}
	if e.ErrorType == "INVALID_TIMEFRAME" {
		s = append(s, "timeframe values accept ISO-8601 timestamps or now()-relative expressions — parentheses required: now()-6h, not now-6h — e.g. from:now()-6h, to:now() or --default-timeframe-start 'now()-6h'")
	}
	return s
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

	// ScopeError — agent-mode preflight blocked a command missing token scopes
	var scopeErr *ScopeError
	if errors.As(err, &scopeErr) {
		return &output.ErrorDetail{
			Code:           "insufficient_scope",
			Message:        scopeErr.Error(),
			RequiredScopes: scopeErr.Required,
			GrantedScopes:  scopeErr.Granted,
			MissingScopes:  scopeErr.Missing,
			Suggestions: []string{
				"re-create your token with: " + strings.Join(scopeErr.Missing, ", "),
				"see 'dtctl commands howto' for token scope guidance",
			},
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

	// ProfileError — command masked by the active command profile (surface axis,
	// distinct from safety_blocked which is the permission axis).
	var profileErr *ProfileError
	if errors.As(err, &profileErr) {
		return &output.ErrorDetail{
			Code:        "profile_blocked",
			Message:     profileErr.Headline(),
			Suggestions: profileErr.Suggestions(),
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

	// query.QueryError — a typed DQL API error. The envelope code becomes the
	// API's error type (e.g. unknown_data_object) and recurring mistake
	// classes get a targeted recovery suggestion.
	var queryErr *sdkquery.QueryError
	if errors.As(err, &queryErr) {
		code := strings.ToLower(queryErr.ErrorType)
		if code == "" {
			code = output.ClassifyHTTPError(queryErr.StatusCode)
		}
		return &output.ErrorDetail{
			Code:        code,
			Message:     queryErr.Error(),
			StatusCode:  queryErr.StatusCode,
			Suggestions: dqlErrorAdvice(queryErr),
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
		if cmdErr.UsageHint != "" {
			detail.Suggestions = append(detail.Suggestions, cmdErr.UsageHint)
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

	// inspect.Error — `dtctl inspect` carries a stable envelope code (spill_file_*,
	// inspect_bad_flags, inspect_unknown_field) plus actionable suggestions.
	var inspectErr *inspect.Error
	if errors.As(err, &inspectErr) {
		return &output.ErrorDetail{
			Code:        inspectErr.Code,
			Message:     inspectErr.Message,
			Suggestions: inspectErr.Suggestions,
		}
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
	var silent *silentExitError
	if errors.As(err, &silent) {
		return silent.code
	}

	var scopeErr *ScopeError
	if errors.As(err, &scopeErr) {
		return client.ExitPermissionError
	}

	var diagErr *diagnostic.Error
	if errors.As(err, &diagErr) {
		return diagErr.ExitCode()
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ExitCode()
	}

	var profileErr *ProfileError
	if errors.As(err, &profileErr) {
		return client.ExitUsageError
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

	// Schema introspection is DQL-side, not a resource — agents try
	// `describe field` / `describe dataobject` when hunting for a schema.
	switch args[0] {
	case "field", "fields", "dataobject", "data-object", "dataobjects", "schema":
		return fmt.Errorf("unknown resource type %q — the data schema is queried, not described: dtctl query 'fetch dt.system.data_objects | fields name' lists tables; a table's fields show up in its records", args[0])
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
		ap.SetJQFilter(jqFilter)
		// If the user explicitly requested an output format via -o,
		// use that format for the result field inside the agent envelope
		// (e.g. -o toon for token-efficient encoding).
		outputFlag := rootCmd.PersistentFlags().Lookup("output")
		if outputFlag != nil && outputFlag.Changed {
			ap.SetResultFormat(outputFormat)
		}
		return ap
	}

	return output.NewPrinterWithOpts(output.PrinterOptions{
		Format:    outputFormat,
		Writer:    os.Stdout,
		PlainMode: plainMode,
		JQFilter:  jqFilter,
		AgentMode: agentMode,
	})
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

// LoadConfig loads the config and applies the context override, if any.
// Precedence: --context flag > DTCTL_CONTEXT env var > current-context in the
// config file. Both overrides are session-local — the config file is never
// written, so a scripted `DTCTL_CONTEXT=x dtctl ...` cannot repoint other
// processes on the machine.
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

	override := contextName
	if override == "" {
		override = os.Getenv("DTCTL_CONTEXT")
	}
	if override != "" {
		cfg.CurrentContext = override
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
	// Propagate W3C trace context on every Dynatrace API request.
	if tracingRootCtx != nil {
		client.InjectTraceContext(c, tracingRootCtx)
	}
	return c, nil
}

// resolveAccountUUID resolves the account UUID using:
// 1. DTCTL_ACCOUNT_UUID env var
// 2. Context account-uuid config field
// 3. Auto-discovery via access-info
// Returns an error if none of the sources yields a UUID.
func resolveAccountUUID(cfg *config.Config, envToken string) (string, error) {
	if v := os.Getenv("DTCTL_ACCOUNT_UUID"); v != "" {
		return v, nil
	}
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return "", err
	}
	if ctx.AccountUUID != "" {
		return ctx.AccountUUID, nil
	}
	// Auto-discovery
	env := auth.DetectEnvironment(ctx.Environment)
	iamBase := client.IAMBaseURLForEnvironment(env)
	uuid, _, err := client.DiscoverAccountUUID(iamBase, envToken, extractEnvironmentID(ctx.Environment))
	if err != nil {
		return "", fmt.Errorf("could not resolve account UUID (set DTCTL_ACCOUNT_UUID or account-uuid in context): %w", err)
	}
	return uuid, nil
}

// extractEnvironmentID extracts the environment ID from a Dynatrace environment URL.
// e.g. "https://abc12345.apps.dynatrace.com" → "abc12345"
func extractEnvironmentID(envURL string) string {
	// strip scheme
	s := strings.TrimPrefix(envURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	// take first segment
	if i := strings.Index(s, "."); i >= 0 {
		return s[:i]
	}
	return s
}

// accountTokenKeyName returns the keyring token name for a given account UUID.
func accountTokenKeyName(uuid string) string {
	return "account-" + uuid
}

// resolveUUIDNoDiscovery returns the account UUID from flagValue > DTCTL_ACCOUNT_UUID > ctx.AccountUUID.
// It never calls the API, so it is safe to call before a token is available.
func resolveUUIDNoDiscovery(ctx *config.Context, flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("DTCTL_ACCOUNT_UUID"); v != "" {
		return v
	}
	return ctx.AccountUUID
}

// resolveLoginAccountUUID resolves the account UUID for `dtctl account login`
// in priority order: explicit flag > DTCTL_ACCOUNT_UUID > context account-uuid >
// auto-discovery via the IAM access-info endpoint (using the environment token).
//
// discoveredName is non-empty only when the UUID was obtained via auto-discovery,
// letting the caller surface an informational message. envToken and iamBaseURL
// are passed in (rather than resolved internally) to keep the function unit-testable.
func resolveLoginAccountUUID(ctx *config.Context, flagValue, envToken, iamBaseURL string) (uuid, discoveredName string, err error) {
	if flagValue != "" {
		return flagValue, "", nil
	}
	if v := os.Getenv("DTCTL_ACCOUNT_UUID"); v != "" {
		return v, "", nil
	}
	if ctx.AccountUUID != "" {
		return ctx.AccountUUID, "", nil
	}
	// Fall back to auto-discovery. This needs the environment token (openid scope);
	// the account-plane token does not exist yet and is rejected by access-info.
	if envToken == "" {
		return "", "", fmt.Errorf("no environment token available for auto-discovery")
	}
	u, name, derr := client.DiscoverAccountUUID(iamBaseURL, envToken, extractEnvironmentID(ctx.Environment))
	if derr != nil {
		return "", "", derr
	}
	return u, name, nil
}

// resolveAccountToken resolves the account token using:
// 1. DTCTL_ACCOUNT_TOKEN env var
// 2. Keyring (if accountUUID is known) — stored by `dtctl account login`
// 3. Error with hint to run `dtctl account login`
func resolveAccountToken(cfg *config.Config, accountUUID string) (string, error) {
	if v := os.Getenv("DTCTL_ACCOUNT_TOKEN"); v != "" {
		return v, nil
	}
	if accountUUID != "" {
		if ctx, err := cfg.CurrentContextObj(); err == nil {
			env := auth.DetectEnvironment(ctx.Environment)
			oauthCfg := auth.AccountOAuthConfig(env, ctx.SafetyLevel, accountUUID)
			if tm, err := auth.NewTokenManager(oauthCfg); err == nil {
				if token, err := tm.GetToken(accountTokenKeyName(accountUUID)); err == nil && token != "" {
					return token, nil
				}
			}
		}
	}
	return "", fmt.Errorf("account token required: set DTCTL_ACCOUNT_TOKEN or run 'dtctl account login'")
}

// resolveCurrentAccountUserUUID extracts the current user's UUID from the account
// token's JWT sub claim. Used to auto-populate --user-uuid on token creation.
func resolveCurrentAccountUserUUID(accountUUID string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	token, err := resolveAccountToken(cfg, accountUUID)
	if err != nil {
		return "", err
	}
	return sdkauth.ExtractJWTSubject(token)
}

// SetupAccount resolves account credentials and builds an account-plane httpclient.
// Use for read-only account commands (no safety check).
func SetupAccount() (*httpclient.Client, string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, "", err
	}
	return setupAccountClient(cfg)
}

// SetupAccountWithSafety resolves account credentials, runs a safety check,
// and builds an account-plane httpclient. Use for mutating account commands.
func SetupAccountWithSafety(op safety.Operation) (*httpclient.Client, string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, "", err
	}
	checker, err := NewSafetyChecker(cfg)
	if err != nil {
		return nil, "", err
	}
	if err := checker.CheckError(op, safety.OwnershipUnknown); err != nil {
		return nil, "", err
	}
	return setupAccountClient(cfg)
}

func setupAccountClient(cfg *config.Config) (*httpclient.Client, string, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, "", err
	}

	// Sniff UUID from env/config without a token — needed for keyring lookup.
	partialUUID := resolveUUIDNoDiscovery(ctx, "")

	accountToken, err := resolveAccountToken(cfg, partialUUID)
	if err != nil {
		return nil, "", err
	}

	env := auth.DetectEnvironment(ctx.Environment)

	// Use the environment token for discovery — the IAM access-info endpoint
	// rejects account-plane tokens.
	envToken, _ := client.GetTokenWithOAuthSupport(cfg, ctx.TokenRef)
	accountUUID, err := resolveAccountUUID(cfg, envToken)
	if err != nil {
		return nil, "", err
	}

	baseURL := client.AccountBaseURLForEnvironment(env)
	c, err := httpclient.New(baseURL, httpclient.WithToken(accountToken))
	if err != nil {
		return nil, "", err
	}
	level := verbosity
	if debugMode {
		level = 2
	}
	// Cap at 1 — level 2 dumps response bodies, which would expose the
	// one-time token secret returned by `account token create`.
	if level > 1 {
		level = 1
	}
	c.EnableVerboseLogging(level, os.Stderr)
	return c, accountUUID, nil
}

// flagsTakingValues is the set of persistent long flags that consume the next
// argument as their value when written without an inline '='.  Boolean and
// count flags are intentionally omitted so their neighbour is not skipped.
//
// NOTE: This must be kept in sync with the PersistentFlags definitions in
// init() at the bottom of this file.  TestFlagsTakingValues_SyncGuard verifies
// this automatically.
var flagsTakingValues = map[string]bool{
	"--config":     true,
	"--context":    true,
	"--output":     true,
	"--jq":         true,
	"--chunk-size": true,
}

// shortFlagsTakingValues maps short flag letters to true when they consume the
// next argument as their value.  Must be kept in sync with init().
// TestFlagsTakingValues_SyncGuard verifies this automatically.
var shortFlagsTakingValues = map[string]bool{
	"-o": true, // --output
}

// buildSpanName derives a safe OTel span name from the supplied command-line
// arguments (typically the alias-expanded args). Only the verb and resource
// (first two positional tokens) are included; further positional arguments
// (e.g. resource IDs or names) and all flag names/values are excluded to avoid
// leaking sensitive data into trace span names.
//
// Leading flags are skipped so that invocations like
//
//	dtctl --context prod get workflows
//
// correctly produce "dtctl get workflows" instead of just "dtctl".
// For long flags that accept a separate value token (see flagsTakingValues),
// and short flags that accept a value (see shortFlagsTakingValues), those
// value tokens are also skipped.
func buildSpanName(args []string) string {
	parts := extractSafeArgs(args)
	if len(parts) == 0 {
		return "dtctl"
	}
	return "dtctl " + strings.Join(parts, " ")
}

// extractSafeArgs returns the first two positional tokens (verb + resource)
// from the supplied command-line arguments, skipping all flags and their
// values. The result is safe for use in span names and resource attributes
// because it never contains flag values, resource IDs, or other potentially
// sensitive data.
func extractSafeArgs(args []string) []string {
	var parts []string
	i := 0
	for i < len(args) && len(parts) < 2 {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--"):
			// Long flag: skip it.
			i++
			// For value-taking flags without inline '=' (e.g. --context prod),
			// also skip the associated value token.
			flagName := arg
			if eqIdx := strings.Index(arg, "="); eqIdx >= 0 {
				flagName = arg[:eqIdx]
			}
			if flagsTakingValues[flagName] && !strings.Contains(arg, "=") &&
				i < len(args) && !strings.HasPrefix(args[i], "-") {
				i++ // skip the value token
			}
		case strings.HasPrefix(arg, "-"):
			// Short flag (e.g. -v, -o json, -Av).
			// For value-taking short flags, also skip the next token.
			i++
			if shortFlagsTakingValues[arg] &&
				i < len(args) && !strings.HasPrefix(args[i], "-") {
				i++ // skip the value token
			}
		default:
			parts = append(parts, arg)
			i++
		}
	}
	return parts
}

// NewDQLExecutorFromConfig creates a DQL executor from a config and client, with OAuth
// token refresh support. When the OAuth token expires during a long-running query poll
// (which can exceed the 5-minute token lifetime), the executor automatically fetches a
// fresh token and retries without aborting the query.
func NewDQLExecutorFromConfig(cfg *config.Config, c *client.Client) *exec.DQLExecutor {
	executor := exec.NewDQLExecutor(c)
	if config.IsOAuthStorageAvailable() {
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
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "use a specific context for this invocation (env: DTCTL_CONTEXT; never persisted)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: json|yaml|csv|toon|table|wide")
	rootCmd.PersistentFlags().StringVar(&jqFilter, "jq", "", "jq filter expression for structured output (json|yaml|toon); non-structured formats are auto-promoted to json")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbose output (-v for details, -vv for full debug including auth headers)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode (full HTTP request/response logging, equivalent to -vv)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	rootCmd.PersistentFlags().BoolVar(&plainMode, "plain", false, "plain output for machine processing (no colors, no interactive prompts)")
	rootCmd.PersistentFlags().BoolVarP(&agentMode, "agent", "A", false, "agent output mode: wrap output in a structured JSON envelope with metadata")
	rootCmd.PersistentFlags().BoolVar(&noAgent, "no-agent", false, "disable auto-detected agent mode")
	rootCmd.PersistentFlags().BoolVar(&checkScopes, "check-scopes", false, "check the active token has the scopes this command requires, then exit without running it")
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
			// Only auto-enable if user hasn't explicitly chosen a non-JSON
			// output format. An explicit `-o json` is compatible — the agent
			// envelope IS json. Agents append `-o json` to nearly every call
			// out of habit, and treating that as an opt-out silently disarmed
			// every envelope affordance (suggestions, warnings, advice) for
			// exactly the audience they were built for (matrix-11 forensics).
			outputFlag := rootCmd.PersistentFlags().Lookup("output")
			if outputFlag == nil || !outputFlag.Changed || outputFormat == "json" {
				agentMode = true
			}
		}
	}

	// Agent mode implies plain mode (no colors, no interactive prompts)
	if agentMode {
		plainMode = true
	}

	// DTCTL_OUTPUT provides a default output format when -o/--output is not
	// given explicitly. The flag always wins; agent-mode auto-detection above
	// also treats the env value as a default, not an explicit choice.
	if f := rootCmd.PersistentFlags().Lookup("output"); f != nil && !f.Changed {
		if env := os.Getenv("DTCTL_OUTPUT"); env != "" {
			outputFormat = env
		}
	}

	// Propagate plain mode to the output package so ColorEnabled() respects --plain
	if plainMode {
		output.SetPlainMode(true)
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else if envPath := os.Getenv(config.EnvConfig); envPath != "" {
		// DTCTL_CONFIG is an explicit, trusted config that bypasses discovery —
		// mirror config.Load's precedence so diagnostics name the right file.
		viper.SetConfigFile(envPath)
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
