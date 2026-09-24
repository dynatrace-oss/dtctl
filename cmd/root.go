package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
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
	"github.com/dynatrace-oss/dtctl/pkg/inspect"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	resapi "github.com/dynatrace-oss/dtctl/pkg/resources/api"
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
	// invocation. Set by executeArgs() and read by NewClientFromConfig to inject
	// W3C trace context headers on outgoing Dynatrace API requests.
	//
	// This is a package-level variable (rather than a function parameter) because
	// NewClientFromConfig is referenced as a function value in breakpoint_helpers.go
	// and changing its signature would cascade across 100+ call sites. The global is
	// acceptable here because invocations are serialized (see Run): executeArgs()
	// sets it before any client is created, and no other invocation can run
	// concurrently in the same process.
	tracingRootCtx context.Context
)

// cmdContext returns cmd.Context() if set, or context.Background() as a
// fallback for test callers that invoke RunE without ExecuteContext.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

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

// Execute runs the CLI process: one invocation from os.Args, then exit.
// Embedders use Run instead, which returns the exit code without terminating
// the process. Routing through Run (rather than executeArgs directly) keeps a
// single execution path, and ensures deferred functions (e.g. tracing
// shutdown/flush) run before os.Exit, which os.Exit would otherwise bypass.
func Execute() {
	os.Exit(Run(os.Args[1:], RunOptions{}))
}

// AddCommand registers an additional top-level command on the dtctl root.
// It exists for commands that live outside this package because they import
// packages that themselves import cmd — registering from here would be an
// import cycle. `dtctl serve` (pkg/serve, wired in main) is the canonical
// case. Call before the first Execute/Run.
func AddCommand(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

// executeArgs runs one invocation of the given command line (program name
// excluded) and returns an exit code. Callers go through Run, which provides
// serialization and the pristine-tree guarantee.
func executeArgs(argv []string) int {
	// --- Stage 1: development-feature registration ---
	// Attach the development-tier commands this invocation opted into and
	// detach the rest, before anything else walks the tree. Registration
	// (rather than hiding) is what makes an un-opted-in development feature
	// unreachable by accident: `dtctl <it>` is an unknown command like any
	// other, and it is absent from help, completion and the catalog.
	devEnabled, devSignpost := resolveDevelopmentFeatures(argv)
	applyDevelopmentRegistration(devEnabled)
	// --- End development-feature registration ---

	// Setup enhanced error handling after all subcommands are registered
	setupErrorHandlers(rootCmd)

	// Wrap runnable commands with the token-scope preflight (--check-scopes and
	// agent-mode auto-preflight). Must run after all subcommands are registered.
	installScopePreflight(rootCmd)

	// Cobra falls back to os.Args when no args were set — always pin the
	// requested argv so embedded invocations never see the host's arguments.
	rootCmd.SetArgs(argv)

	// --- Alias resolution (before Cobra parses args AND before tracing init) ---
	// Resolving aliases first ensures the span name reflects the real command,
	// not the pre-expansion alias. Load config quietly; if it fails, skip alias
	// resolution (the real command will produce the proper error later).
	// Session-backed invocations skip aliases entirely: they are a host-config
	// convenience, and a tenant request must not expand through the host's
	// alias table.
	spanArgs := argv
	if cfg, err := config.Load(); err == nil && runSession == nil {
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
		if cfg.IgnoredEnvRefs() {
			fmt.Fprintf(os.Stderr,
				"warning: local config %q contains env-var references ($...) that were not expanded "+
					"(use --config or DTCTL_CONFIG to use a trusted config with env-var expansion)\n",
				cfg.LocalConfigPath())
		}

		expanded, isShell, err := resolveAlias(argv, cfg)
		if err != nil {
			output.PrintHumanError("%s", err)
			return 1
		}

		if isShell {
			if !caps.ShellAliases {
				output.PrintHumanError("%s", &CapabilityError{Feature: "shell aliases"})
				return 1
			}
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

	// --- Blocked-command filter (embedded callers) ---
	// Mask commands the embedding caller declared unsupported in its
	// environment (RunOptions.BlockedCommands) — e.g. the service engine
	// removes host-oriented commands like config/ctx/auth. Applied after the
	// profile filter so both masks compose; a nil set is the full surface.
	applyBlockedCommands(rootCmd, runBlocked)
	// --- End blocked-command filter ---

	// --- Stage 3: stability floor ---
	// Mask every command and flag whose contract is weaker than the floor this
	// context accepts, unless an exception names it. Runs after the profile and
	// blocked-command masks so all three compose and none can widen what an
	// earlier one narrowed. A misspelled exception is a hard error rather than
	// a silent skip: it would otherwise tighten the surface and produce a
	// confusing block much later.
	policy, stabErr := resolveStabilityPolicy(spanArgs, devEnabled)
	if stabErr != nil {
		output.PrintHumanError("%s", stabErr)
		return client.ExitUsageError
	}
	applyStabilityFloor(rootCmd, policy)
	// Badge what survived, so a caller reading help is told the guarantee
	// rather than left to infer it from the tier's name.
	applyStabilityBadges(rootCmd)
	// --- End stability floor ---

	// --- Stage 5: deprecated surface ---
	// Show the caller the world as it will be after the removal release, so a
	// pipeline can find out it still depends on something scheduled to go
	// while that is a build failure rather than an outage. Orthogonal to the
	// floor -- a deprecated command is stable in shape, merely dated -- but
	// implemented as another narrowing stage so it composes without any
	// precedence rule. Last, because it is the only stage the caller turns on
	// to *find* problems rather than to avoid them.
	// surfaceConfig, not LoadConfig: this is a surface decision like the four
	// stages above it, and must not depend on credentials resolving.
	if surfaceConfig(spanArgs).NoDeprecated() {
		applyNoDeprecated(rootCmd)
	}
	// --- End deprecated surface ---

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
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(flushCtx)
	}()
	if tracingErr != nil {
		// Non-fatal: warn and continue. The CLI still works; spans may not export.
		fmt.Fprintf(os.Stderr, "dtctl: tracing: %v (check OTEL_EXPORTER_OTLP_ENDPOINT or unset it to disable export)\n", tracingErr)
	}

	if err := rootCmd.ExecuteContext(runCtx); err != nil {
		// silentExitError carries an exit code only (e.g. --check-scopes printed
		// its verdict, diff found differences, wait timed out); set the status
		// and return without re-printing.
		var silent *silentExitError
		if errors.As(err, &silent) {
			if silent.code == 0 {
				rootSpan.SetStatus(codes.Ok, "")
			} else {
				reason := silent.reason
				if reason == "" {
					reason = "silent non-zero exit"
				}
				rootSpan.SetStatus(codes.Error, reason)
			}
			return silent.code
		}

		errStr := err.Error()

		// Unknown top-level commands get one shot at plugin dispatch before
		// the suggestion enhancer: `dtctl foo` execs dtctl-foo from PATH if
		// present (kubectl semantics; built-ins always win because they never
		// reach this error path). See docs/dev/PLUGIN_CONVENTIONS.md.
		if strings.Contains(errStr, "unknown command") {
			if code, handled := tryPluginDispatch(spanArgs); handled {
				return code
			}
			// A disabled development feature explains itself only to a caller
			// who has already demonstrated knowledge of the mechanism, and
			// never in agent mode. Everyone else gets the ordinary unknown
			// command answer — see developmentSignposting.
			if devErr := developmentHint(errStr, devSignpost); devErr != nil {
				err = devErr
			} else {
				err = enhanceCommandError(rootCmd, err)
			}
		}

		// Enhance unknown flag errors with suggestions. A flag error the
		// command's own FlagErrorFunc already typed is left alone: its message
		// no longer matches cobra's raw wording, so re-running the enhancer
		// would flatten *suggest.FlagError back to a plain error and cost the
		// invocation its usage exit code.
		var typedFlagErr *suggest.FlagError
		if !errors.As(err, &typedFlagErr) &&
			(strings.Contains(errStr, "unknown flag") || strings.Contains(errStr, "unknown shorthand flag")) {
			err = enhanceFlagError(rootCmd, err)
		}

		// Check for URL-related hints (e.g., wrong domain like live.dynatrace.com)
		urlHints := getURLHintsForError(err)

		// Check for auth-related hints (e.g., expired OAuth session)
		authHints := getAuthHintsForError(err)

		// Check for a missing API specification index (`get apis` / `describe api`)
		indexHints := getAPIIndexHintsForError(err)

		allHints := make([]string, 0, len(urlHints)+len(authHints)+len(indexHints))
		allHints = append(allHints, urlHints...)
		allHints = append(allHints, authHints...)
		allHints = append(allHints, indexHints...)

		// Record the error on the root span so it appears in traces.
		rootSpan.SetStatus(codes.Error, err.Error())
		rootSpan.RecordError(err)

		// Two failures leave --agent/--plain unread in the flag vars: masked
		// commands (profile mask, blocked-command filter) disable flag parsing
		// so the guard is the only observable outcome, and a flag error stops
		// pflag before it reaches a mode flag given later on the line. Honor
		// them from the raw argv so a machine caller still gets the structured
		// envelope, wherever it put --agent. An unknown command stops cobra
		// even earlier, and neither failure ever reaches initConfig, so the
		// environment's auto-detection is asked here as well.
		structuredError := agentMode || plainMode
		if !structuredError {
			var maskedProfile *ProfileError
			var maskedUnsupported *UnsupportedCommandError
			var maskedStability *StabilityError
			var unknownFlag *suggest.FlagError
			var unknownCmd *suggest.CommandError
			if errors.As(err, &maskedProfile) || errors.As(err, &maskedUnsupported) ||
				errors.As(err, &maskedStability) || errors.As(err, &unknownFlag) ||
				errors.As(err, &unknownCmd) || errors.Is(err, errEmptyFlagValue) {
				structuredError = hasRawFlag(spanArgs, "--agent") ||
					hasShortFlagLetter(spanArgs, 'A') ||
					hasRawFlag(spanArgs, "--plain") ||
					agentModeAutoDetectedFromArgs(spanArgs)
			}
		}

		if structuredError {
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

	// An explicitly empty value for a flag that requires one (see
	// rejectEmptyFlag). pflag names the offending flag in the error it wraps
	// around errEmptyFlagValue, so the flag comes from the error, not from the
	// rendered message — which escapes a tab or a non-breaking space beyond
	// recognition. The sentinel stays wrapped: it is what errorToDetail and
	// exitCodeForError classify on.
	var invalidValue *pflag.InvalidValueError
	if errors.As(err, &invalidValue) && errors.Is(err, errEmptyFlagValue) {
		return fmt.Errorf("--%s %w; pass a value or leave the flag out", invalidValue.GetFlag().Name, errEmptyFlagValue)
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

// queryStateErrorDetail maps a query state error to its envelope code.
func queryStateErrorDetail(err error, state string) *output.ErrorDetail {
	detail := &output.ErrorDetail{Message: err.Error()}
	switch state {
	case sdkquery.StateFailed:
		detail.Code = "query_failed"
		detail.Suggestions = []string{"the query itself failed: fix the query, re-running it unchanged will fail again"}
	case sdkquery.StateCancelled:
		detail.Code = "query_cancelled"
		detail.Suggestions = []string{"the query was cancelled on the server: run it again if the cancellation was not intended"}
	case sdkquery.StateResultGone:
		detail.Code = "result_expired"
		detail.Suggestions = []string{"the query is valid but its result expired: run the same query again"}
	default:
		detail.Code = "unknown_query_state"
		detail.Suggestions = []string{
			"upgrade dtctl: this Grail version reports a query state that this dtctl does not know",
			"if the query was simply slow, running it again may still succeed",
		}
	}
	return detail
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
		suggestions := scopeErr.Advice
		if len(suggestions) == 0 {
			suggestions = []string{
				"re-create your token with: " + strings.Join(scopeErr.Missing, ", "),
				"see 'dtctl commands howto' for token scope guidance",
			}
		}
		return &output.ErrorDetail{
			Code:           "insufficient_scope",
			Message:        scopeErr.Error(),
			RequiredScopes: scopeErr.Required,
			GrantedScopes:  scopeErr.Granted,
			MissingScopes:  scopeErr.Missing,
			Suggestions:    suggestions,
		}
	}

	// resapi.BlockedError — a generic `exec api` request refused by the gate. It
	// wraps the safety refusal, so this case must come first: the same
	// safety_blocked code, but the message says why the request was classified as
	// it was, which is what tells a caller what to do differently.
	var apiBlockedErr *resapi.BlockedError
	if errors.As(err, &apiBlockedErr) {
		return &output.ErrorDetail{
			Code:        "safety_blocked",
			Message:     apiBlockedErr.Headline(),
			Operation:   fmt.Sprintf("call %s %s", apiBlockedErr.Method, apiBlockedErr.RequestPath),
			Suggestions: apiBlockedErr.Suggestions(),
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

	// StabilityError — command or flag below the active stability floor. A third
	// code alongside profile_blocked and safety_blocked, because the three are
	// different axes and a caller resolves them differently: a topical surface
	// change, a permission grant, and an accepted contract risk.
	var stabilityErr *StabilityError
	if errors.As(err, &stabilityErr) {
		return &output.ErrorDetail{
			Code:        "stability_blocked",
			Message:     stabilityErr.Headline(),
			Suggestions: stabilityErr.Suggestions(),
		}
	}

	// DevelopmentError — an un-opted-in development feature was invoked by a
	// caller who had already shown they know the mechanism. Never produced in
	// agent mode (see developmentSignposting), so this case exists for the
	// --plain structured path only.
	// DeprecatedError — deprecated surface was used under DTCTL_NO_DEPRECATED.
	// Its own code rather than stability_blocked: the caller's contract has not
	// been weakened, it has a removal date, and the fix is a migration rather
	// than an exception or a lower floor.
	var deprecatedErr *DeprecatedError
	if errors.As(err, &deprecatedErr) {
		return &output.ErrorDetail{
			Code:        "deprecated_surface",
			Message:     deprecatedErr.Headline(),
			Suggestions: deprecatedErr.Suggestions(),
		}
	}

	var devErr *DevelopmentError
	if errors.As(err, &devErr) {
		return &output.ErrorDetail{
			Code:        "development_disabled",
			Message:     devErr.Headline(),
			Suggestions: devErr.Suggestions(),
		}
	}

	// UnsupportedCommandError — command removed from the surface by the
	// embedding caller (e.g. host-oriented commands inside the service engine).
	var unsupportedErr *UnsupportedCommandError
	if errors.As(err, &unsupportedErr) {
		return &output.ErrorDetail{
			Code:        "unsupported_in_service",
			Message:     unsupportedErr.Headline(),
			Suggestions: unsupportedErr.Suggestions(),
		}
	}

	// CapabilityError — a host-restricted ability (subprocess spawn: plugins,
	// aliases, hooks, editor, browser) was requested but not granted.
	var capErr *CapabilityError
	if errors.As(err, &capErr) {
		return &output.ErrorDetail{
			Code:    "capability_disabled",
			Message: capErr.Error(),
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
	// A table the token cannot read is a scope problem, not a query problem:
	// report it with the precheck's code so an agent handles both the same way.
	if q, ok := isNotAuthorizedForTable(err); ok {
		return notAuthorizedForTableDetail(q)
	}
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

	// sdkquery.StateError — the query ended in a state other than SUCCEEDED.
	// The code tells an agent whether a verbatim retry can help.
	var stateErr *sdkquery.StateError
	if errors.As(err, &stateErr) {
		return queryStateErrorDetail(err, stateErr.State)
	}

	// suggest.CommandError — unknown command with "did you mean?" suggestions
	var cmdErr *suggest.CommandError
	if errors.As(err, &cmdErr) {
		detail := &output.ErrorDetail{
			Code:    "unknown_command",
			Message: cmdErr.Message,
		}
		if len(cmdErr.Runnable) > 0 {
			detail.Suggestions = cmdErr.Runnable
			return detail
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

	// An empty value for a flag that needs one. The flag exists and is spelled
	// right, so unknown_command's advice ("follow the did-you-mean") would
	// mislead; the fix is in the caller's input.
	if errors.Is(err, errEmptyFlagValue) {
		return &output.ErrorDetail{
			Code:    "validation_error",
			Message: err.Error(),
		}
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

	// apispec.RegistryUnavailableError — the environment publishes no
	// machine-readable API index. A distinct code because it is an expected
	// property of an environment, not a failure of the command: a consumer should
	// stop probing for specifications rather than retry.
	var registryErr *resapi.RegistryUnavailableError
	if errors.As(err, &registryErr) {
		// A refused index is not a missing one, and a consumer must be able to tell
		// them apart without parsing the message: only the former is worth retrying
		// with a different credential.
		if registryErr.StatusCode == 401 || registryErr.StatusCode == 403 {
			return &output.ErrorDetail{
				Code:       output.ClassifyHTTPError(registryErr.StatusCode),
				Message:    registryErr.Error(),
				StatusCode: registryErr.StatusCode,
			}
		}
		return &output.ErrorDetail{
			Code:       "api_index_unavailable",
			Message:    registryErr.Error(),
			StatusCode: registryErr.StatusCode,
		}
	}

	// apispec.SpecUnavailableError — a listed specification could not be read.
	// Distinct from the generic HTTP classification so a consumer can tell "no
	// specification" apart from "the API call failed".
	var specErr *resapi.SpecUnavailableError
	if errors.As(err, &specErr) {
		return &output.ErrorDetail{
			Code:       "api_spec_unavailable",
			Message:    specErr.Error(),
			StatusCode: specErr.StatusCode,
		}
	}

	// output.JQError — a --jq filter that addressed the wrong object shape. The
	// stable code lets an agent tell "you asked wrongly" from "no data", which a
	// null result could not.
	var jqErr *output.JQError
	if errors.As(err, &jqErr) {
		return &output.ErrorDetail{
			Code:        jqErr.Code,
			Message:     jqErr.Message,
			Suggestions: jqErr.Suggestions,
		}
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

// getAPIIndexHintsForError returns recovery hints when an environment publishes
// no machine-readable API index.
//
// The index is an observed convention rather than a documented contract, so its
// absence is an expected outcome with two concrete fallbacks — not a failure to
// merely report. Hints flow to both audiences: they become envelope suggestions
// in agent mode and printed hints for a human.
func getAPIIndexHintsForError(err error) []string {
	var registryErr *resapi.RegistryUnavailableError
	if errors.As(err, &registryErr) {
		// A refused index is a fact about the credential, not about the environment,
		// and it must not be answered with advice about the environment: someone told
		// their index is unpublished will go looking at the wrong layer entirely.
		if registryErr.StatusCode == 401 || registryErr.StatusCode == 403 {
			return []string{
				"the index request was rejected, so this says nothing about whether the " +
					"environment publishes one — check the credential first",
				"run 'dtctl doctor' to verify the active context's token",
			}
		}
		return []string{
			"browse this environment's own API explorer at " + resapi.SwaggerUIPath,
			"address an API by base path, e.g. dtctl describe api /platform/document/v1",
		}
	}

	// A listed document that cannot be read. The status carries the whole story,
	// and the body does not: a 403 here is answered with a page about SSO, which
	// would otherwise be forwarded verbatim to a caller it cannot help.
	var specErr *resapi.SpecUnavailableError
	if errors.As(err, &specErr) {
		switch specErr.StatusCode {
		case 401, 403:
			return []string{
				"this environment refused the specification document even though your token is valid — " +
					"some environments serve specifications only to an interactive session",
				"open " + resapi.SwaggerUIPath + " in a browser against this environment instead",
				"the API itself is unaffected: 'dtctl exec api <path>' does not need the specification",
			}
		case 404:
			return []string{
				"the environment's API index lists this API but publishes no specification document for it",
			}
		}
	}
	return nil
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

// isPermissionDenied reports whether err is a typed 403. Callers that fall back
// from one lookup to another use it to stop: a denial is not an absence, and
// reporting "not found" for a 403 both misleads the user and throws away the
// permission diagnostics the handler attached.
func isPermissionDenied(err error) bool {
	var diagErr *diagnostic.Error
	if errors.As(err, &diagErr) {
		return diagErr.StatusCode == 403
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 403
	}

	return false
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
	if _, ok := isNotAuthorizedForTable(err); ok {
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

	var unsupportedErr *UnsupportedCommandError
	if errors.As(err, &unsupportedErr) {
		return client.ExitUsageError
	}

	var stabilityErr *StabilityError
	if errors.As(err, &stabilityErr) {
		return client.ExitUsageError
	}

	var developmentErr *DevelopmentError
	if errors.As(err, &developmentErr) {
		return client.ExitUsageError
	}

	var deprecatedUse *DeprecatedError
	if errors.As(err, &deprecatedUse) {
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

	if errors.Is(err, errEmptyFlagValue) {
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
	// The errors are typed so the agent envelope reports unknown_command (and
	// the usage exit code) like any other misspelled command, with the
	// correction as a command line that runs as-is.
	switch args[0] {
	case "field", "fields", "dataobject", "data-object", "dataobjects", "schema":
		return &suggest.CommandError{
			Command:  args[0],
			Message:  fmt.Sprintf("unknown resource type %q — the data schema is queried, not described: dtctl query 'fetch dt.system.data_objects | fields name' lists tables; a table's fields show up in its records", args[0]),
			Runnable: []string{"dtctl query 'fetch dt.system.data_objects | fields name'"},
		}
	}

	// Check if the first arg looks like an unknown subcommand
	subcommands := collectSubcommands(cmd)
	suggestion := suggest.FindClosest(args[0], subcommands)

	if suggestion != nil {
		corrected := append([]string{cmd.CommandPath(), suggestion.Value}, quoteCommandArgs(args[1:])...)
		return &suggest.CommandError{
			Command:    args[0],
			Message:    fmt.Sprintf("unknown resource type %q", args[0]),
			Suggestion: suggestion,
			Runnable:   []string{strings.Join(corrected, " ")},
		}
	}

	return &suggest.CommandError{
		Command:   args[0],
		Message:   fmt.Sprintf("unknown resource type %q", args[0]),
		UsageHint: fmt.Sprintf("Run '%s --help' for available resources", cmd.CommandPath()),
		Runnable:  []string{"dtctl commands"},
	}
}

func quoteCommandArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quoteCommandArg(a)
	}
	return quoted
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

// SetupWithSafetyAndPrinter is SetupWithSafety plus a Printer, for mutating
// commands that render their result through the normal output pipeline.
func SetupWithSafetyAndPrinter(op safety.Operation) (*config.Config, *client.Client, output.Printer, error) {
	cfg, c, err := SetupWithSafety(op)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, c, NewPrinter(), nil
}

// NewSafetyChecker creates a new safety checker for the current context.
// For auto-discovered local configs, the safety level is clamped to
// min(local, global) so a rogue .dtctl.yaml cannot escalate past the global
// guard (see Config.GetEffectiveSafetyLevel).
func NewSafetyChecker(cfg *config.Config) (*safety.Checker, error) {
	if _, err := cfg.CurrentContextObj(); err != nil {
		return nil, err
	}
	return safety.NewCheckerWithLevel(cfg.CurrentContext, cfg.GetEffectiveSafetyLevel()), nil
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
	// A session-backed invocation (embedded callers, see Session) is pinned to
	// its own environment + token: the config file and context overrides do
	// not apply.
	if runSession != nil {
		return runSession.syntheticConfig(), nil
	}

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

	accountUUID := resolveUUIDNoDiscovery(ctx, "")
	if accountUUID == "" {
		return nil, "", fmt.Errorf("account UUID required: set DTCTL_ACCOUNT_UUID, add account-uuid to the current context, or pass --account-uuid")
	}

	accountToken, err := resolveAccountToken(cfg, accountUUID)
	if err != nil {
		return nil, "", err
	}

	env := auth.DetectEnvironment(ctx.Environment)
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
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: json|yaml|csv|toon|table|wide|auto")
	rootCmd.PersistentFlags().StringVar(&jqFilter, "jq", "", "jq filter expression for structured output (json|yaml|toon); applied to the result payload, not the --agent envelope (on query: '.records', not '.result.records'); non-structured formats are auto-promoted to json")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "verbose output (-v for details, -vv for full debug including auth headers)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode (full HTTP request/response logging, equivalent to -vv)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	rootCmd.PersistentFlags().BoolVar(&plainMode, "plain", false, "plain output for machine processing (no colors, no interactive prompts)")
	rootCmd.PersistentFlags().BoolVarP(&agentMode, "agent", "A", false, "agent output mode: wrap output in a structured JSON envelope with metadata")
	rootCmd.PersistentFlags().BoolVar(&noAgent, "no-agent", false, "disable auto-detected agent mode")
	rootCmd.PersistentFlags().BoolVar(&checkScopes, "check-scopes", false, "check the active token has the scopes this command requires, then exit without running it")
	rootCmd.PersistentFlags().Int64Var(&chunkSize, "chunk-size", 500, "Paginate through all results in chunks of this size. 0 returns only the first page.")

	// Both flags read as "absent" when empty, so `--context "$CTX"` with an
	// unset CTX would silently run against the default context — the worst
	// form of the bug rejectEmptyFlag exists for, since the command still
	// succeeds, against the wrong tenant.
	rejectEmptyFlag(rootCmd, "context")
	rejectEmptyFlag(rootCmd, "config")

	// Bind flags to viper
	_ = viper.BindPFlag("context", rootCmd.PersistentFlags().Lookup("context"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// agentModeAutoDetectedFromArgs answers initConfig's auto-detection question
// from the raw command line, for failures cobra raises before initConfig runs
// (unknown command, unknown flag). It applies the same rules: an explicit
// --no-agent, a session-backed invocation, or an explicit non-JSON -o opts out.
func agentModeAutoDetectedFromArgs(args []string) bool {
	if noAgent || runSession != nil || rawNoAgent(args) || !aidetect.Detect().Detected {
		return false
	}
	format, given := rawOutputFormat(args)
	return !given || format == "json"
}

// rawNoAgent reports whether the unparsed args turn --no-agent on.
func rawNoAgent(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--no-agent" {
			return true
		}
		if v, ok := strings.CutPrefix(a, "--no-agent="); ok {
			if on, err := strconv.ParseBool(v); err == nil && on {
				return true
			}
		}
	}
	return false
}

// rawOutputFormat extracts -o/--output from unparsed args, in all the
// spellings pflag accepts: "--output x", "--output=x", "-o x", "-ox", "-o=x".
func rawOutputFormat(args []string) (string, bool) {
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "--output" || a == "-o" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
		if v, ok := strings.CutPrefix(a, "--output="); ok {
			return v, true
		}
		if v, ok := strings.CutPrefix(a, "-o"); ok && !strings.HasPrefix(a, "--") {
			return strings.TrimPrefix(v, "="), true
		}
	}
	return "", false
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	// Auto-detect AI agent environment and enable agent mode. Session-backed
	// invocations skip auto-detection entirely: whether the *host process*
	// runs under an AI agent says nothing about the request, and host env
	// must not shape a tenant's output. Service callers opt in per request,
	// explicitly, with --agent on the command line.
	if !agentMode && !noAgent && runSession == nil {
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
