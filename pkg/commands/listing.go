// Package commands builds a machine-readable catalog of dtctl's command tree.
// It walks the Cobra command hierarchy and produces a structured listing that
// AI agents and MCP servers can use for automated tool registration.
package commands

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// SchemaVersion is incremented on breaking changes to the listing structure.
const SchemaVersion = 1

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
	TimeFormats   *TimeFormats      `json:"time_formats,omitempty" yaml:"time_formats,omitempty"`
	Patterns      []string          `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	Antipatterns  []string          `json:"antipatterns,omitempty" yaml:"antipatterns,omitempty"`
}

// Verb represents a top-level verb (get, describe, apply, ...).
type Verb struct {
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	Mutating     bool             `json:"mutating" yaml:"mutating"`
	SafetyOp     string           `json:"safety_operation,omitempty" yaml:"safety_operation,omitempty"`
	Resources    []string         `json:"resources,omitempty" yaml:"resources,omitempty"`
	Flags        map[string]*Flag `json:"flags,omitempty" yaml:"flags,omitempty"`
	RequiredArgs []string         `json:"required_args,omitempty" yaml:"required_args,omitempty"`
	Subcommands  map[string]*Verb `json:"subcommands,omitempty" yaml:"subcommands,omitempty"`
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
// This map must be kept in sync with actual NewSafetyChecker calls in cmd/.
// The TestMutatingVerbsMatchSafetyCheckerUsage test in cmd/commands_test.go
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
	listing := &Listing{
		SchemaVersion: SchemaVersion,
		Tool:          "dtctl",
		Version:       version.Version,
		Description:   "kubectl-inspired CLI for the Dynatrace platform",
		CommandModel:  "verb-noun",
		GlobalFlags:   buildGlobalFlags(root),
		Verbs:         buildVerbs(root),
		Aliases:       ResourceAliases,
		TimeFormats:   defaultTimeFormats,
		Patterns:      defaultPatterns,
		Antipatterns:  defaultAntipatterns,
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

		verbs[name] = verb
	}
	return verbs
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
	}

	for name, verb := range l.Verbs {
		bv := &Verb{
			Mutating:  verb.Mutating,
			Resources: verb.Resources,
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
					Mutating:  sub.Mutating,
					Resources: sub.Resources,
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
		SchemaVersion: l.SchemaVersion,
		Tool:          l.Tool,
		Version:       l.Version,
		Description:   l.Description,
		CommandModel:  l.CommandModel,
		GlobalFlags:   l.GlobalFlags,
		Verbs:         filteredVerbs,
		Aliases:       l.Aliases,
		TimeFormats:   l.TimeFormats,
		Patterns:      l.Patterns,
		Antipatterns:  l.Antipatterns,
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

// WriteTo writes the listing to w in the given format ("json" or "yaml"/"yml").
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
	default:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(l)
	}
}
