package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/commands"
	"github.com/dynatrace-oss/dtctl/pkg/output"
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
type silentExitError struct{ code int }

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
func scopePreflight(c *cobra.Command, _ []string) (skip bool, err error) {
	// Only do work when explicitly requested or when auto-preflighting in agent mode.
	if !checkScopes && !agentMode {
		return false, nil
	}

	verb, resource := verbResource(c)

	if checkScopes {
		required, hasReq := requiredScopesFor(verb, resource)
		result := computeScopeVerdict(verb, resource, required, hasReq)
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
			return true, &silentExitError{code: client.ExitPermissionError}
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
	required, hasReq := requiredScopesFor(verb, resource)
	if !hasReq {
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

// requiredScopesFor looks up a command's required scopes from the catalog (the
// single source of truth). Returns (scopes, true) when a scope requirement is
// known, or (nil, false) for local/no-scope commands.
func requiredScopesFor(verb, resource string) ([]string, bool) {
	listing := commands.Build(rootCmd)
	v, ok := listing.Verbs[verb]
	if !ok {
		return nil, false
	}
	if resource != "" {
		if s, ok := v.RequiredScopesByResource[resource]; ok && len(s) > 0 {
			return s, true
		}
	}
	if len(v.RequiredScopes) > 0 { // DQL verbs (query/verify/wait)
		return v.RequiredScopes, true
	}
	return nil, false
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
func computeScopeVerdict(verb, resource string, required []string, hasReq bool) ScopeCheckResult {
	res := ScopeCheckResult{
		Verb:           verb,
		Resource:       resource,
		RequiredScopes: required,
	}
	if !hasReq {
		// No platform scopes required (local command); nothing to check.
		res.Status = scopeStatusOK
		res.RequiredScopes = []string{}
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
