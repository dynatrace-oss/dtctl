// Package commands builds a machine-readable catalog of dtctl's command tree.
// It walks the Cobra command hierarchy and produces a structured listing that
// AI agents and MCP servers can use for automated tool registration.
package commands

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	toon "github.com/toon-format/toon-go"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// SchemaVersion is incremented on breaking changes to the listing structure.
// v2 adds per-verb `access`, per-(verb,resource) `required_scopes_by_resource`,
// per-verb `required_scopes` (DQL verbs), and a top-level `resource_scopes`
// canonical table.
const SchemaVersion = 2

// Listing is the top-level output of `dtctl commands`.
type Listing struct {
	SchemaVersion int               `json:"schema_version" yaml:"schema_version"`
	Tool          string            `json:"tool" yaml:"tool"`
	Version       string            `json:"version" yaml:"version"`
	Description   string            `json:"description,omitempty" yaml:"description,omitempty"`
	CommandModel  string            `json:"command_model" yaml:"command_model"`
	GlobalFlags   map[string]*Flag  `json:"global_flags,omitempty" yaml:"global_flags,omitempty"`
	Verbs         map[string]*Verb  `json:"verbs" yaml:"verbs"`
	Aliases       map[string]string `json:"resource_aliases,omitempty" yaml:"resource_aliases,omitempty"`
	// ResourceScopes is the canonical (resource, access) → scopes table. Agents
	// can derive any command's required scopes from this table plus each verb's
	// access, which is what --brief relies on.
	ResourceScopes map[string]auth.AccessScopes `json:"resource_scopes,omitempty" yaml:"resource_scopes,omitempty"`
	TimeFormats    *TimeFormats                 `json:"time_formats,omitempty" yaml:"time_formats,omitempty"`
	Patterns       []string                     `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	Antipatterns   []string                     `json:"antipatterns,omitempty" yaml:"antipatterns,omitempty"`
}

// Verb represents a top-level verb (get, describe, apply, ...).
type Verb struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Mutating    bool     `json:"mutating" yaml:"mutating"`
	SafetyOp    string   `json:"safety_operation,omitempty" yaml:"safety_operation,omitempty"`
	Access      string   `json:"access,omitempty" yaml:"access,omitempty"`
	Resources   []string `json:"resources,omitempty" yaml:"resources,omitempty"`
	// RequiredScopes is the scope list for verbs whose scopes are not
	// per-resource (e.g. `query`/`verify`/`wait`, which read Grail via DQL).
	RequiredScopes []string `json:"required_scopes,omitempty" yaml:"required_scopes,omitempty"`
	// RequiredScopesByResource maps each resource the verb operates on to the
	// scopes required for this verb's access level. Materialized in full mode so
	// agents need zero client-side computation; omitted in --brief.
	RequiredScopesByResource map[string][]string `json:"required_scopes_by_resource,omitempty" yaml:"required_scopes_by_resource,omitempty"`
	Flags                    map[string]*Flag    `json:"flags,omitempty" yaml:"flags,omitempty"`
	RequiredArgs             []string            `json:"required_args,omitempty" yaml:"required_args,omitempty"`
	Subcommands              map[string]*Verb    `json:"subcommands,omitempty" yaml:"subcommands,omitempty"`
}

// Flag describes a CLI flag.
type Flag struct {
	Type        string `json:"type" yaml:"type"`
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// TimeFormats describes the time input formats dtctl accepts.
type TimeFormats struct {
	Relative []string `json:"relative" yaml:"relative"`
	Absolute string   `json:"absolute" yaml:"absolute"`
	Unix     string   `json:"unix" yaml:"unix"`
}

// MutatingVerbs maps verb names to their safety operation type.
// Verbs not listed here are read-only (mutating: false).
//
// This map must be kept in sync with actual safety-check call sites in cmd/
// (both NewSafetyChecker and SetupWithSafety helpers). The
// TestMutatingVerbsMatchSafetyCheckerUsage test in cmd/commands_test.go
// cross-references this map against the real command tree to detect drift.
var MutatingVerbs = map[string]string{
	"apply":   "OperationCreate",
	"create":  "OperationCreate",
	"edit":    "OperationUpdate",
	"delete":  "OperationDelete",
	"restore": "OperationUpdate",
	"share":   "OperationUpdate",
	"unshare": "OperationUpdate",
	"update":  "OperationUpdate",
	"exec":    "OperationCreate", // semantically mutating (runs workflows, functions)
	"enable":  "OperationUpdate", // PUTs updated monitoring/credential config to the tenant
}

// ResourceAliases are the standard resource aliases built into dtctl.
//
// This map must be kept in sync with Cobra command Aliases fields in cmd/.
// The TestResourceAliasesMatchCobraAliases test in cmd/commands_test.go
// cross-references this map against the real command tree to detect drift.
var ResourceAliases = map[string]string{
	"wf":   "workflows",
	"dash": "dashboards",
	"db":   "dashboards",
	"nb":   "notebooks",
	"bkt":  "buckets",
	"ec":   "edgeconnect",
	"fn":   "functions",
	"func": "functions",
	"ad":   "anomaly-detectors",
}

// hiddenCommands are commands excluded from the listing (internal, utility).
var hiddenCommands = map[string]bool{
	"help":       true,
	"completion": true,
	"commands":   true, // self-referential — not useful in the catalog
	"version":    true, // utility, not an operational command
}

// patterns are recommended usage patterns for AI agents.
var defaultPatterns = []string{
	"Use 'dtctl apply -f' for idempotent resource management",
	"Use 'dtctl diff' before 'dtctl apply' to preview changes",
	"Use 'dtctl query' for ad-hoc DQL queries, not resource-specific flags",
	"Use '--dry-run' to validate apply operations without executing",
	"Use '--agent' for JSON output with operational metadata",
	"Use 'dtctl wait' in CI/CD to poll for conditions",
	"Always specify '--context' in automation scripts",
}

// antipatterns are common mistakes agents should avoid.
var defaultAntipatterns = []string{
	"Don't use 'dtctl create' followed by 'dtctl edit' — use 'dtctl apply -f' instead",
	"Don't parse table output — use '-o json' or '--agent'",
	"Don't hardcode resource IDs — use 'dtctl get' to discover them",
	"Don't skip 'dtctl diff' before 'dtctl apply' in production contexts",
}

var defaultTimeFormats = &TimeFormats{
	Relative: []string{"1h", "30m", "7d", "5min"},
	Absolute: "RFC3339 (e.g., 2024-01-15T10:00:00Z)",
	Unix:     "Unix timestamp (e.g., 1705312800)",
}

// Build walks the Cobra command tree rooted at root and returns a Listing.
func Build(root *cobra.Command) *Listing {
	verbs := buildVerbs(root)
	listing := &Listing{
		SchemaVersion:  SchemaVersion,
		Tool:           "dtctl",
		Version:        version.Version,
		Description:    "kubectl-inspired CLI for the Dynatrace platform",
		CommandModel:   "verb-noun",
		GlobalFlags:    buildGlobalFlags(root),
		Verbs:          verbs,
		Aliases:        ResourceAliases,
		ResourceScopes: collectResourceScopes(verbs),
		TimeFormats:    defaultTimeFormats,
		Patterns:       defaultPatterns,
		Antipatterns:   defaultAntipatterns,
	}
	return listing
}

// buildGlobalFlags extracts the persistent flags from the root command.
func buildGlobalFlags(root *cobra.Command) map[string]*Flag {
	flags := make(map[string]*Flag)
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		// Skip internal/debug flags
		if f.Hidden {
			return
		}
		key := "--" + f.Name
		flags[key] = &Flag{
			Type:        flagTypeName(f),
			Default:     f.DefValue,
			Description: f.Usage,
		}
	})
	return flags
}

// buildVerbs iterates over the root's direct subcommands and maps them as verbs.
func buildVerbs(root *cobra.Command) map[string]*Verb {
	verbs := make(map[string]*Verb)
	for _, cmd := range root.Commands() {
		name := cmd.Name()
		if hiddenCommands[name] || cmd.Hidden {
			continue
		}

		verb := &Verb{
			Description: cmd.Short,
		}

		// Determine mutating status
		if safetyOp, ok := MutatingVerbs[name]; ok {
			verb.Mutating = true
			verb.SafetyOp = safetyOp
		}

		// Derive the access level (read/write/delete/run) from the verb.
		verb.Access = string(auth.AccessForVerb(name, verb.SafetyOp))

		// Extract resources (subcommands of the verb)
		subs := cmd.Commands()
		if len(subs) > 0 {
			var resources []string
			subcommands := make(map[string]*Verb)
			hasResources := false

			for _, sub := range subs {
				if sub.Hidden || sub.Name() == "help" {
					continue
				}

				subName := sub.Name()

				// Check if this subcommand itself has subcommands (nested, like exec copilot)
				nestedSubs := sub.Commands()
				hasNested := false
				for _, ns := range nestedSubs {
					if !ns.Hidden && ns.Name() != "help" {
						hasNested = true
						break
					}
				}

				if hasNested {
					// Nested subcommand (e.g., exec copilot -> nl2dql, dql2nl, ...)
					subVerb := &Verb{
						Description: sub.Short,
					}
					if safetyOp, ok := MutatingVerbs[name]; ok {
						subVerb.Mutating = true
						subVerb.SafetyOp = safetyOp
					}
					subVerb.Access = string(auth.AccessForVerb(name, subVerb.SafetyOp))
					if scopes := auth.ScopesForResource(subName, auth.Access(subVerb.Access)); len(scopes) > 0 {
						subVerb.RequiredScopes = append([]string(nil), scopes...)
					}
					nestedNames := make(map[string]*Verb)
					for _, ns := range nestedSubs {
						if ns.Hidden || ns.Name() == "help" {
							continue
						}
						nestedNames[ns.Name()] = &Verb{
							Description: ns.Short,
						}
					}
					if len(nestedNames) > 0 {
						subVerb.Subcommands = nestedNames
					}

					// Collect flags from the subcommand
					subFlags := collectLocalFlags(sub)
					if len(subFlags) > 0 {
						subVerb.Flags = subFlags
					}

					subcommands[subName] = subVerb
				} else {
					// Treat as a resource
					resources = append(resources, subName)
					hasResources = true
				}
			}

			if hasResources {
				verb.Resources = resources
			}
			if len(subcommands) > 0 {
				verb.Subcommands = subcommands
			}
		}

		// Extract verb-level flags (local flags, not from subcommands)
		verbFlags := collectLocalFlags(cmd)
		if len(verbFlags) > 0 {
			verb.Flags = verbFlags
		}

		// Extract required args from Use string (e.g., "apply -f <file>")
		if args := parseRequiredArgs(cmd.Use); len(args) > 0 {
			verb.RequiredArgs = args
		}

		populateVerbScopes(name, verb)

		verbs[name] = verb
	}
	return verbs
}

// dqlScopeVerbs read Grail data via DQL; their scopes attach to the verb rather
// than to a managed resource.
var dqlScopeVerbs = map[string]bool{"query": true, "verify": true, "wait": true}

// populateVerbScopes fills in a verb's required scopes from the canonical
// auth.ResourceScopes table: per-resource for resource-bearing verbs, and a flat
// DQL scope list for query-style verbs.
func populateVerbScopes(name string, verb *Verb) {
	if dqlScopeVerbs[name] {
		verb.RequiredScopes = append([]string(nil), auth.QueryScopes...)
	}

	if len(verb.Resources) == 0 {
		return
	}
	access := auth.Access(verb.Access)
	byResource := make(map[string][]string)
	for _, r := range verb.Resources {
		if scopes := auth.ScopesForResource(r, access); len(scopes) > 0 {
			byResource[r] = scopes
		}
	}
	if len(byResource) > 0 {
		verb.RequiredScopesByResource = byResource
	}
}

// collectResourceScopes returns the subset of the canonical ResourceScopes table
// covering the resources referenced by the given verbs (including subcommands).
// Returns nil when no referenced resource has a scope mapping.
func collectResourceScopes(verbs map[string]*Verb) map[string]auth.AccessScopes {
	table := make(map[string]auth.AccessScopes)
	add := func(resource string) {
		for _, key := range []string{resource, singularize(resource)} {
			if as, ok := auth.ResourceScopes[key]; ok {
				table[key] = as
				return
			}
		}
	}
	for _, verb := range verbs {
		for _, r := range verb.Resources {
			add(r)
		}
		for subName, sub := range verb.Subcommands {
			add(subName)
			for _, r := range sub.Resources {
				add(r)
			}
		}
	}
	if len(table) == 0 {
		return nil
	}
	return table
}

// collectLocalFlags extracts non-persistent, non-hidden flags from a command.
func collectLocalFlags(cmd *cobra.Command) map[string]*Flag {
	flags := make(map[string]*Flag)
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		// Skip flags that are inherited from parent (persistent flags)
		if cmd.InheritedFlags().Lookup(f.Name) != nil {
			return
		}

		key := "--" + f.Name
		if f.Shorthand != "" {
			key = "-" + f.Shorthand + "/" + key
		}

		fl := &Flag{
			Type:        flagTypeName(f),
			Default:     f.DefValue,
			Description: f.Usage,
		}

		// Mark as required if annotated
		if ann := f.Annotations; ann != nil {
			if _, ok := ann[cobra.BashCompOneRequiredFlag]; ok {
				fl.Required = true
			}
		}

		flags[key] = fl
	})
	return flags
}

// parseRequiredArgs extracts angle-bracket args from a Use string.
// e.g., "apply -f <file>" → ["file"]
func parseRequiredArgs(use string) []string {
	var args []string
	for _, part := range strings.Fields(use) {
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			args = append(args, strings.Trim(part, "<>"))
		}
	}
	return args
}

// flagTypeName returns a human-readable type name for a flag.
func flagTypeName(f *pflag.Flag) string {
	switch f.Value.Type() {
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "int", "int32", "int64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "stringSlice", "stringArray":
		return "string[]"
	case "duration":
		return "duration"
	case "count":
		return "integer"
	default:
		return f.Value.Type()
	}
}

// NewBrief returns a copy of the listing with verbose fields stripped for
// reduced token count. It preserves mutating status since agents always need it.
// The original listing is not modified.
func NewBrief(l *Listing) *Listing {
	brief := &Listing{
		SchemaVersion: l.SchemaVersion,
		Tool:          l.Tool,
		Version:       l.Version,
		CommandModel:  l.CommandModel,
		Verbs:         make(map[string]*Verb, len(l.Verbs)),
		Aliases:       l.Aliases,
		// Keep the compact canonical table; agents derive a command's scopes
		// from resource_scopes[resource][verb.access] without the materialized
		// per-command map.
		ResourceScopes: l.ResourceScopes,
	}

	for name, verb := range l.Verbs {
		bv := &Verb{
			Mutating:  verb.Mutating,
			Access:    verb.Access,
			Resources: verb.Resources,
			// DQL scopes are not derivable from resource_scopes, so retain them.
			RequiredScopes: verb.RequiredScopes,
		}

		// Simplify flags: just type, drop description/default
		if verb.Flags != nil {
			bv.Flags = make(map[string]*Flag, len(verb.Flags))
			for k, f := range verb.Flags {
				bf := &Flag{Type: f.Type}
				if f.Required {
					bf.Description = "(required)"
				}
				bv.Flags[k] = bf
			}
		}

		// Recurse into subcommands
		if verb.Subcommands != nil {
			bv.Subcommands = make(map[string]*Verb, len(verb.Subcommands))
			for subName, sub := range verb.Subcommands {
				bs := &Verb{
					Mutating:       sub.Mutating,
					Access:         sub.Access,
					Resources:      sub.Resources,
					RequiredScopes: sub.RequiredScopes,
				}
				if sub.Flags != nil {
					bs.Flags = make(map[string]*Flag, len(sub.Flags))
					for k, f := range sub.Flags {
						bs.Flags[k] = &Flag{Type: f.Type}
					}
				}
				if sub.Subcommands != nil {
					bs.Subcommands = make(map[string]*Verb, len(sub.Subcommands))
					for nestedName, nested := range sub.Subcommands {
						bs.Subcommands[nestedName] = &Verb{Mutating: nested.Mutating}
					}
				}
				bv.Subcommands[subName] = bs
			}
		}

		brief.Verbs[name] = bv
	}

	return brief
}

// FilterByResource returns a new listing containing only verbs that operate on
// the given resource name. The name is matched against resources, subcommands,
// and aliases. Returns the filtered listing and true if any verbs matched.
// The original listing is not modified.
func FilterByResource(l *Listing, name string) (*Listing, bool) {
	// Resolve alias
	resolved := name
	if target, ok := ResourceAliases[name]; ok {
		resolved = target
	}

	var filteredVerbs map[string]*Verb

	// Check if it's a verb name first
	if verb, ok := l.Verbs[name]; ok {
		filteredVerbs = map[string]*Verb{name: verb}
	} else {
		// Filter to verbs that contain the resource
		filteredVerbs = make(map[string]*Verb)
		for verbName, verb := range l.Verbs {
			if containsResource(verb, resolved) || containsResource(verb, name) {
				filteredVerbs[verbName] = verb
			}
		}
	}

	if len(filteredVerbs) == 0 {
		return nil, false
	}

	result := &Listing{
		SchemaVersion:  l.SchemaVersion,
		Tool:           l.Tool,
		Version:        l.Version,
		Description:    l.Description,
		CommandModel:   l.CommandModel,
		GlobalFlags:    l.GlobalFlags,
		Verbs:          filteredVerbs,
		Aliases:        l.Aliases,
		ResourceScopes: collectResourceScopes(filteredVerbs),
		TimeFormats:    l.TimeFormats,
		Patterns:       l.Patterns,
		Antipatterns:   l.Antipatterns,
	}
	return result, true
}

// containsResource checks if a verb or its subcommands reference a resource name.
// It handles singular/plural matching by normalizing both the query and each
// resource name to a common stem (stripping trailing "s"). This avoids fragile
// "append s" heuristics that break for irregular plurals.
func containsResource(verb *Verb, name string) bool {
	stem := singularize(name)
	// Check resources list
	for _, r := range verb.Resources {
		if r == name || singularize(r) == stem {
			return true
		}
	}
	// Check subcommands
	for subName := range verb.Subcommands {
		if subName == name || singularize(subName) == stem {
			return true
		}
	}
	return false
}

// singularize returns a basic singular form by stripping a trailing "s".
// This is intentionally simple — dtctl resource names follow the convention
// of using "s" for plurals (workflows/workflow, dashboards/dashboard).
func singularize(name string) string {
	if strings.HasSuffix(name, "s") {
		return strings.TrimSuffix(name, "s")
	}
	return name
}

// ResolveAlias resolves a resource alias to its canonical name.
// Returns the original name if no alias exists.
func ResolveAlias(name string) string {
	if target, ok := ResourceAliases[name]; ok {
		return target
	}
	return name
}

// RequiredScopesUnion returns the sorted, de-duplicated union of every scope
// required by the verbs in the listing (per-resource scopes, DQL scopes, and
// subcommand scopes). Combined with FilterByResource it yields the minimal
// token scope set for a filtered command set, e.g. the scopes a CI pipeline
// that only runs `get`/`apply` on workflows needs.
func RequiredScopesUnion(l *Listing) []string {
	seen := map[string]bool{}
	add := func(scopes []string) {
		for _, s := range scopes {
			seen[s] = true
		}
	}
	var walk func(v *Verb)
	walk = func(v *Verb) {
		add(v.RequiredScopes)
		for _, scopes := range v.RequiredScopesByResource {
			add(scopes)
		}
		for _, sub := range v.Subcommands {
			walk(sub)
		}
	}
	for _, v := range l.Verbs {
		walk(v)
	}

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// RequiredScopesForResource returns the sorted union of scopes needed to run
// every verb in the listing against a single resource — the minimal token scope
// set for a pipeline that only touches that resource. The resource name may be
// an alias or plural; it is resolved against the canonical table.
func RequiredScopesForResource(l *Listing, resource string) []string {
	resolved := ResolveAlias(resource)
	seen := map[string]bool{}
	for _, verb := range l.Verbs {
		if !containsResource(verb, resolved) && !containsResource(verb, resource) {
			continue
		}
		for _, s := range auth.ScopesForResource(resolved, auth.Access(verb.Access)) {
			seen[s] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// WriteTo writes the listing to w in the given format ("json", "yaml"/"yml", or "toon").
// Any other format value defaults to JSON.
func WriteTo(w io.Writer, l *Listing, format string) error {
	switch format {
	case "yaml", "yml":
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(l); err != nil {
			return err
		}
		return enc.Close()
	case "toon":
		// Round-trip through JSON to get a generic representation that
		// respects json struct tags, then encode as TOON.
		b, err := json.Marshal(l)
		if err != nil {
			return err
		}
		var generic interface{}
		if err := json.Unmarshal(b, &generic); err != nil {
			return err
		}
		data, err := toon.Marshal(generic, toon.WithLengthMarkers(true))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	default:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(l)
	}
}
