package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/commands"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	resapi "github.com/dynatrace-oss/dtctl/pkg/resources/api"
)

// checkScopes is the --check-scopes persistent flag: resolve the command's
// required token scopes, compare against the scopes granted in the active token,
// print the verdict, and do NOT run the command.
var checkScopes bool

// scope-check verdict statuses.
const (
	scopeStatusOK           = "ok"
	scopeStatusInsufficient = "insufficient_scope"
	scopeStatusUnknown      = "unknown"
)

// ScopeError is returned by the agent-mode auto-preflight when a mutating
// command is missing required scopes. It is rendered as an insufficient_scope
// envelope (see errorToDetail) and exits with ExitPermissionError.
type ScopeError struct {
	Verb     string
	Resource string
	Required []string
	Granted  []string
	Missing  []string
}

func (e *ScopeError) Error() string {
	target := e.Verb
	if e.Resource != "" {
		target += " " + e.Resource
	}
	return fmt.Sprintf("missing %d required scope(s) for %q: %s",
		len(e.Missing), target, strings.Join(e.Missing, ", "))
}

// silentExitError carries a process exit code without producing any output.
// Used by the explicit --check-scopes path, which prints its own verdict and
// then needs to set a non-zero exit code without root.go printing a second error.
type silentExitError struct {
	code int
	// reason is recorded as the root span's status message for non-zero
	// codes; it is never printed to the user (the command already produced
	// its output — the code is part of its contract, not an error).
	reason string
}

func (e *silentExitError) Error() string { return "" }

// ScopeCheckResult is the verdict emitted by `--check-scopes`.
type ScopeCheckResult struct {
	Verb           string   `json:"verb" yaml:"verb"`
	Resource       string   `json:"resource,omitempty" yaml:"resource,omitempty"`
	Status         string   `json:"status" yaml:"status"` // ok | insufficient_scope | unknown
	RequiredScopes []string `json:"required_scopes" yaml:"required_scopes"`
	GrantedScopes  []string `json:"granted_scopes,omitempty" yaml:"granted_scopes,omitempty"`
	MissingScopes  []string `json:"missing_scopes,omitempty" yaml:"missing_scopes,omitempty"`
	Suggestions    []string `json:"suggestions,omitempty" yaml:"suggestions,omitempty"`
}

// installScopePreflight wraps the RunE of every runnable command so that a scope
// preflight runs before the command body. It mirrors setupErrorHandlers' tree
// walk. Commands without a RunE (pure parent verbs) are skipped.
func installScopePreflight(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		installScopePreflight(sub)
	}
	orig := cmd.RunE
	if orig == nil {
		return
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		skip, err := scopePreflight(c, args)
		if err != nil {
			return err
		}
		if skip {
			return nil
		}
		return orig(c, args)
	}
}

// scopePreflight implements the --check-scopes verdict and the agent-mode
// auto-preflight. It returns (skip=true) when the original command body should
// not run, and an error to abort with. The preflight is intentionally resilient:
// any failure to determine scopes degrades to "proceed" rather than blocking a
// command, so it can never turn a working command into a broken one.
func scopePreflight(c *cobra.Command, args []string) (skip bool, err error) {
	// Only do work when explicitly requested or when auto-preflighting in agent mode.
	if !checkScopes && !agentMode {
		return false, nil
	}

	verb, resource := verbResource(c)

	if checkScopes {
		required, req := requiredScopesFor(verb, resource)
		if req == scopeRequirementPerCall {
			// --check-scopes is explicit and terminal: the command body does not run
			// afterwards, so resolving the requirement from the environment's own
			// specification spends one round trip the caller asked for. The agent-mode
			// auto-preflight below deliberately does not do this — it runs ahead of
			// every command and must stay free.
			if scopes, ok := resolvePerCallScopes(c, args); ok {
				required, req = scopes, scopeRequirementKnown
			}
		}
		result := computeScopeVerdict(verb, resource, required, req)
		// In agent mode the verdict must use the same envelope contract as every
		// other command: an insufficient verdict reuses the ScopeError path (so it
		// renders as an `insufficient_scope` error envelope with exit 5, identical
		// to the auto-preflight), and ok/unknown verdicts are wrapped in an OK
		// envelope rather than printed as a bare object.
		if agentMode {
			if result.Status == scopeStatusInsufficient {
				return false, &ScopeError{
					Verb:     verb,
					Resource: resource,
					Required: result.RequiredScopes,
					Granted:  result.GrantedScopes,
					Missing:  result.MissingScopes,
				}
			}
			printScopeVerdictAgent(result)
			return true, nil
		}
		printScopeVerdict(result)
		if result.Status == scopeStatusInsufficient {
			return true, &silentExitError{code: client.ExitPermissionError, reason: "insufficient scope"}
		}
		return true, nil
	}

	// Agent-mode auto-preflight: only block mutating commands whose missing
	// scopes we can prove from an introspectable (OAuth) token. The mutating
	// check is first so non-mutating commands (the common path) never pay for
	// resolving required scopes.
	if _, mutating := commands.MutatingVerbs[verb]; !mutating {
		return false, nil
	}
	// Only a catalog-known requirement can prove a command will fail. A per-call
	// requirement would have to be resolved over the network on every single
	// invocation, which is not a price a preflight may charge — `exec api` reports
	// the platform's own 403, with the declared scope, when it happens.
	required, req := requiredScopesFor(verb, resource)
	if req != scopeRequirementKnown {
		return false, nil
	}
	granted, known := grantedScopesFunc()
	if !known {
		return false, nil
	}
	missing := subtractScopes(required, granted)
	if len(missing) == 0 {
		return false, nil
	}
	return false, &ScopeError{
		Verb:     verb,
		Resource: resource,
		Required: required,
		Granted:  granted,
		Missing:  missing,
	}
}

// verbResource derives the verb and resource for a resolved command. The verb is
// the ancestor whose parent is the root command; the resource is the leaf
// command name (empty when the leaf is the verb itself, e.g. `query`).
func verbResource(c *cobra.Command) (verb, resource string) {
	node := c
	for node.Parent() != nil && node.Parent() != rootCmd {
		node = node.Parent()
	}
	verb = node.Name()
	if c != node {
		resource = c.Name()
	}
	return verb, resource
}

// scopeRequirement is what the catalog can say about a command's scopes.
//
// The two-way answer this replaced could not tell "needs no scope" apart from
// "cannot say", so it reported both as ok — and an ok verdict for a command
// nothing was checked against is worse than no verdict at all.
type scopeRequirement int

const (
	// scopeRequirementNone: the command touches no platform API (ctx, commands).
	scopeRequirementNone scopeRequirement = iota
	// scopeRequirementKnown: the catalog names the scopes.
	scopeRequirementKnown
	// scopeRequirementPerCall: the scopes are a property of the invocation, not of
	// the command, so the catalog cannot hold them.
	scopeRequirementPerCall
)

// perCallScopeCommands are `<verb> <resource>` leaves whose required scopes
// depend on their arguments.
//
// `exec api` is the only one: it can reach any endpoint the environment
// publishes, and each declares its own scope. Listing a union of every scope in
// the catalog would be both wrong and useless, so the requirement is resolved per
// call — from the environment's specification — or honestly reported as unknown.
var perCallScopeCommands = map[string]bool{
	"exec api": true,
}

// requiredScopesFor looks up a command's required scopes from the catalog (the
// single source of truth), and reports what kind of answer that is.
func requiredScopesFor(verb, resource string) ([]string, scopeRequirement) {
	if perCallScopeCommands[verb+" "+resource] {
		return nil, scopeRequirementPerCall
	}
	listing := commands.Build(rootCmd)
	v, ok := listing.Verbs[verb]
	if !ok {
		return nil, scopeRequirementNone
	}
	if resource != "" {
		if s, ok := v.RequiredScopesByResource[resource]; ok && len(s) > 0 {
			return s, scopeRequirementKnown
		}
	}
	if len(v.RequiredScopes) > 0 { // DQL verbs (query/verify/wait)
		return v.RequiredScopes, scopeRequirementKnown
	}
	return nil, scopeRequirementNone
}

// resolvePerCallScopes resolves the scopes this specific invocation needs, for a
// command whose requirement is per-call.
//
// It returns false whenever the answer is not certain — an unreadable
// specification, a path that matches no operation, or an operation that declares
// no scope. The verdict then stays "unknown", because a preflight that guesses is
// the one failure mode worse than a preflight that abstains.
func resolvePerCallScopes(c *cobra.Command, args []string) ([]string, bool) {
	// `exec api` is the only per-call command; a second one would add a case here.
	if c.Name() != "api" || c.Parent() == nil || c.Parent().Name() != "exec" || len(args) == 0 {
		return nil, false
	}

	method, _ := c.Flags().GetString("method")
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}

	_, client, err := SetupClient()
	if err != nil {
		return nil, false
	}
	_, op := resapi.NewHandler(client).ResolveForRequest(method, args[0])
	if op == nil || len(op.Scopes) == 0 {
		return nil, false
	}
	return op.Scopes, true
}

// grantedScopesFunc reads the active token's granted scopes. Overridable in tests.
var grantedScopesFunc = grantedScopes

// grantedScopes returns the scopes granted in the active context's token and
// whether they are introspectable. Opaque API/platform tokens are not
// introspectable, so (nil, false) is returned — the caller treats this as
// "unknown" rather than a false negative.
func grantedScopes() (scopes []string, known bool) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, false
	}
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, false
	}
	status, err := buildSessionStatusFunc(cfg.CurrentContext, ctx, ctx.TokenRef)
	if err != nil || status == nil || !status.IsOAuth || len(status.GrantedScopes) == 0 {
		return nil, false
	}
	return status.GrantedScopes, true
}

// computeScopeVerdict builds the verdict for the explicit --check-scopes path.
func computeScopeVerdict(verb, resource string, required []string, req scopeRequirement) ScopeCheckResult {
	res := ScopeCheckResult{
		Verb:           verb,
		Resource:       resource,
		RequiredScopes: required,
	}
	switch req {
	case scopeRequirementNone:
		// No platform scopes required (local command); nothing to check.
		res.Status = scopeStatusOK
		res.RequiredScopes = []string{}
		return res
	case scopeRequirementPerCall:
		// Resolution was not attempted or did not succeed. Reporting ok here would
		// assure the caller about a check that never happened — and this is exactly
		// the command where a false assurance is most expensive, since it can reach
		// any endpoint the environment publishes.
		res.Status = scopeStatusUnknown
		res.RequiredScopes = []string{}
		res.Suggestions = []string{
			"this command's required scopes depend on the endpoint it calls, so they are " +
				"not knowable in advance",
			"dtctl describe api <name> --operation '<METHOD> <path>'  -- the scope the " +
				"specification declares for that endpoint",
		}
		return res
	}

	granted, known := grantedScopesFunc()
	if !known {
		res.Status = scopeStatusUnknown
		res.Suggestions = []string{
			"token scopes are not introspectable (API/platform token); ensure it carries: " + strings.Join(required, ", "),
		}
		return res
	}

	res.GrantedScopes = granted
	missing := subtractScopes(required, granted)
	if len(missing) == 0 {
		res.Status = scopeStatusOK
		return res
	}
	res.Status = scopeStatusInsufficient
	res.MissingScopes = missing
	res.Suggestions = []string{
		"re-create your token with: " + strings.Join(missing, ", "),
		"see 'dtctl commands howto' for token scope guidance",
	}
	return res
}

// printScopeVerdictAgent writes an ok/unknown verdict wrapped in the agent
// envelope, so `--check-scopes` in agent mode emits the same {ok,result,context}
// shape as every other command. Insufficient verdicts go through the ScopeError
// error-envelope path instead (see scopePreflight).
func printScopeVerdictAgent(r ScopeCheckResult) {
	resp := output.Response{
		OK:      true,
		Result:  r,
		Context: &output.ResponseContext{Verb: r.Verb, Resource: r.Resource},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

// printScopeVerdict writes the verdict in the active output format (non-agent).
func printScopeVerdict(r ScopeCheckResult) {
	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
	case "yaml", "yml":
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		_ = enc.Encode(r)
		_ = enc.Close()
	default:
		printScopeVerdictHuman(r)
	}
}

func printScopeVerdictHuman(r ScopeCheckResult) {
	target := r.Verb
	if r.Resource != "" {
		target += " " + r.Resource
	}
	fmt.Printf("Scope check for %q:\n", target)
	if len(r.RequiredScopes) == 0 {
		// An empty requirement means two different things, and the status is what
		// separates them: nothing is needed, or nothing could be determined. Printing
		// "no platform scopes required" for the latter would turn an abstention into
		// a claim.
		if r.Status == scopeStatusUnknown {
			fmt.Println("  required: unknown")
			fmt.Println("  status:   unknown — dtctl could not determine what this call needs")
			for _, s := range r.Suggestions {
				fmt.Printf("    - %s\n", s)
			}
			return
		}
		fmt.Println("  no platform scopes required")
		return
	}
	fmt.Printf("  required: %s\n", strings.Join(r.RequiredScopes, ", "))
	switch r.Status {
	case scopeStatusUnknown:
		fmt.Println("  granted:  unknown (token scopes are not introspectable)")
		fmt.Println("  status:   unknown — cannot verify; ensure the token carries the required scopes")
	case scopeStatusInsufficient:
		fmt.Printf("  granted:  %s\n", strings.Join(r.GrantedScopes, ", "))
		fmt.Printf("  missing:  %s\n", strings.Join(r.MissingScopes, ", "))
		fmt.Printf("  status:   insufficient — missing %d scope(s)\n", len(r.MissingScopes))
	default:
		fmt.Println("  status:   ok — all required scopes granted")
	}
}

// subtractScopes returns the sorted elements of required not present in granted.
func subtractScopes(required, granted []string) []string {
	have := make(map[string]bool, len(granted))
	for _, g := range granted {
		have[g] = true
	}
	var missing []string
	for _, r := range required {
		if !have[r] {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	return missing
}
