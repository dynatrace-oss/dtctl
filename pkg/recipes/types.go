// Package recipes loads, validates and renders recipes: named, parameterized
// DQL that `dtctl run <recipe>` executes. A recipe is content, not code — it
// lives in YAML files (built-in, user, org) or in recipe bundles that Dynatrace
// apps ship as documents — so its DQL can evolve without a dtctl change.
//
// The package has no cobra and no output code. The command layer (cmd/) turns
// a loaded Book into commands and prints results; everything here is testable
// on its own. Design: docs/dev/RECIPES_DESIGN.md.
package recipes

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// APIVersion is the only apiVersion this dtctl understands for recipe content.
const APIVersion = "dtctl.dev/v1alpha1"

// Kinds of recipe content documents.
const (
	KindRecipe  = "Recipe"
	KindBundle  = "RecipeBundle"
	KindDomains = "RecipeDomains"
	KindScopes  = "RecipeScopes"
)

// Layer names where a recipe came from. Later layers override earlier ones
// (see Precedence).
type Layer string

const (
	LayerBuiltin     Layer = "builtin"
	LayerEnvironment Layer = "environment"
	LayerOrg         Layer = "org"
	LayerUser        Layer = "user"
)

// Precedence is the override order, weakest first: a recipe in a later layer
// replaces one of the same name in an earlier layer, as a whole.
var Precedence = []Layer{LayerBuiltin, LayerEnvironment, LayerOrg, LayerUser}

func layerRank(l Layer) int {
	for i, p := range Precedence {
		if p == l {
			return i
		}
	}
	return -1
}

// Source records where a piece of content was loaded from.
type Source struct {
	Layer Layer `json:"layer" yaml:"layer"`
	// Location is the file path for file layers, or "document <id>" for an
	// app-shipped bundle.
	Location string `json:"location,omitempty" yaml:"location,omitempty"`
	// AppID and BundleVersion identify an app-shipped bundle.
	AppID         string `json:"appId,omitempty" yaml:"appId,omitempty"`
	BundleVersion int    `json:"bundleVersion,omitempty" yaml:"bundleVersion,omitempty"`
}

// String is the short form reported as context.recipe.source: "builtin",
// "user", "org", or "app:<app-id>@<version>".
func (s Source) String() string {
	if s.Layer == LayerEnvironment && s.AppID != "" {
		return fmt.Sprintf("app:%s@%d", s.AppID, s.BundleVersion)
	}
	return string(s.Layer)
}

// Recipe is one recipe document.
type Recipe struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       Spec     `json:"spec" yaml:"spec"`

	// Source is where this recipe was loaded from; Shadows lists the recipes of
	// the same name it replaced from weaker layers.
	Source  Source   `json:"-" yaml:"-"`
	Shadows []Source `json:"-" yaml:"-"`
	// fragments is the fragment text visible to this recipe beyond the shared
	// set: an app bundle's own fragments.
	fragments []string
}

// Name returns the recipe's name.
func (r *Recipe) Name() string { return r.Metadata.Name }

// Domain returns the domain prefix of the recipe's name ("k8s" for
// "k8s-pod-restarts"). Domains may not contain a dash, so the first segment is
// the domain.
func (r *Recipe) Domain() string {
	name := r.Metadata.Name
	if i := strings.IndexByte(name, '-'); i > 0 {
		return name[:i]
	}
	return name
}

// Metadata identifies a recipe.
type Metadata struct {
	Name    string   `json:"name" yaml:"name"`
	Version int      `json:"version" yaml:"version"`
	Tags    []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// Spec is what a recipe does.
type Spec struct {
	Summary     string       `json:"summary" yaml:"summary"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Requires    []string     `json:"requires,omitempty" yaml:"requires,omitempty"`
	Scope       []string     `json:"scope,omitempty" yaml:"scope,omitempty"`
	Segments    SegmentsMode `json:"segments,omitempty" yaml:"segments,omitempty"`
	Params      Params       `json:"params,omitempty" yaml:"params,omitempty"`
	Timeframe   Timeframe    `json:"timeframe" yaml:"timeframe"`
	DQL         string       `json:"dql" yaml:"dql"`
	Means       string       `json:"means" yaml:"means"`
	EmptyMeans  string       `json:"emptyMeans" yaml:"emptyMeans"`
	Next        []Next       `json:"next,omitempty" yaml:"next,omitempty"`
	Deprecated  *Deprecated  `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// SegmentsMode says whether filter segments may narrow a recipe.
type SegmentsMode string

const (
	SegmentsOn  SegmentsMode = "on"
	SegmentsOff SegmentsMode = "off"
)

// Off reports whether the recipe opted out of filter segments.
func (m SegmentsMode) Off() bool { return m == SegmentsOff }

// UnmarshalYAML accepts on/off and the YAML booleans they look like
// (`segments: off` decodes as a bool in YAML 1.1 readers).
func (m *SegmentsMode) UnmarshalYAML(n *yaml.Node) error {
	switch strings.ToLower(strings.TrimSpace(n.Value)) {
	case "", "on", "true":
		*m = SegmentsOn
	case "off", "false":
		*m = SegmentsOff
	default:
		return fmt.Errorf("segments must be on or off, got %q", n.Value)
	}
	return nil
}

// Param types.
const (
	TypeString = "string"
	TypeInt    = "int"
	TypeBool   = "bool"
	TypeEnum   = "enum"
	TypeList   = "list"
)

// Param is one recipe parameter. It becomes a flag of the recipe's command.
type Param struct {
	Name        string    `json:"name" yaml:"-"`
	Type        string    `json:"type" yaml:"type"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool      `json:"required,omitempty" yaml:"required,omitempty"`
	Positional  bool      `json:"positional,omitempty" yaml:"positional,omitempty"`
	Default     yaml.Node `json:"-" yaml:"default,omitempty"`
	Min         *int64    `json:"min,omitempty" yaml:"min,omitempty"`
	Max         *int64    `json:"max,omitempty" yaml:"max,omitempty"`
	Values      []string  `json:"values,omitempty" yaml:"values,omitempty"`
	Pattern     string    `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	// Render "identifier" renders an enum value verbatim (a field name, a sort
	// direction) instead of as a string literal; the value must be one of
	// Values exactly.
	Render string `json:"render,omitempty" yaml:"render,omitempty"`
}

// FlagName is the param's flag spelling: snake_case becomes kebab-case.
func (p *Param) FlagName() string { return strings.ReplaceAll(p.Name, "_", "-") }

// HasDefault reports whether the param declares a default.
func (p *Param) HasDefault() bool { return p.Default.Kind != 0 }

// DefaultString is the default as written in the file ("" when none).
func (p *Param) DefaultString() string {
	if !p.HasDefault() {
		return ""
	}
	if p.Default.Kind == yaml.SequenceNode {
		var items []string
		for _, c := range p.Default.Content {
			items = append(items, c.Value)
		}
		return strings.Join(items, ",")
	}
	return p.Default.Value
}

// Params is the ordered set of a recipe's params. Order is the order in the
// file, which is the order flags are documented in.
type Params []*Param

// UnmarshalYAML decodes a mapping of name → param, keeping file order.
func (ps *Params) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("params must be a mapping of name to definition")
	}
	out := make(Params, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		p := &Param{}
		if err := n.Content[i+1].Decode(p); err != nil {
			return fmt.Errorf("param %q: %w", name, err)
		}
		p.Name = name
		out = append(out, p)
	}
	*ps = out
	return nil
}

// Get returns the named param, or nil.
func (ps Params) Get(name string) *Param {
	for _, p := range ps {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Positional returns the positional param, or nil.
func (ps Params) Positional() *Param {
	for _, p := range ps {
		if p.Positional {
			return p
		}
	}
	return nil
}

// Timeframe is a recipe's window policy (design §3).
type Timeframe struct {
	// Declared is false when the field was missing, which validation rejects.
	Declared bool `json:"-" yaml:"-"`
	// None marks a state query: no window is sent and no --from/--to offered.
	None    bool          `json:"none,omitempty" yaml:"-"`
	Default time.Duration `json:"default,omitempty" yaml:"-"`
	// Min rejects a shorter window (a trend needs history).
	Min time.Duration `json:"min,omitempty" yaml:"-"`
	// Max rejects a longer window: a scan-heavy recipe bounds what one
	// invocation can cost. Use dtctl query with context.query to go further.
	Max time.Duration `json:"max,omitempty" yaml:"-"`
	// Fixed rejects --from/--to: the recipe is a snapshot of current state.
	Fixed bool `json:"fixed,omitempty" yaml:"-"`
	// Inline hands the window to the template as .window; the DQL writes its
	// own from:/to: and no default timeframe is sent.
	Inline bool `json:"inline,omitempty" yaml:"-"`
	// Align rounds the resolved window ("utc-day").
	Align string `json:"align,omitempty" yaml:"-"`
}

// AlignUTCDay rounds a window to UTC midnights.
const AlignUTCDay = "utc-day"

// UnmarshalYAML accepts `2h`, `none`, or a map {default, min, max, fixed, inline, align}.
func (t *Timeframe) UnmarshalYAML(n *yaml.Node) error {
	*t = Timeframe{Declared: true}
	switch n.Kind {
	case yaml.ScalarNode:
		if strings.EqualFold(strings.TrimSpace(n.Value), "none") {
			t.None = true
			return nil
		}
		d, err := ParseDuration(n.Value)
		if err != nil {
			return fmt.Errorf("timeframe: %w", err)
		}
		t.Default = d
		return nil
	case yaml.MappingNode:
		var raw struct {
			Default string `yaml:"default"`
			Min     string `yaml:"min"`
			Max     string `yaml:"max"`
			Fixed   bool   `yaml:"fixed"`
			Inline  bool   `yaml:"inline"`
			Align   string `yaml:"align"`
		}
		if err := n.Decode(&raw); err != nil {
			return fmt.Errorf("timeframe: %w", err)
		}
		if raw.Default == "" {
			return fmt.Errorf("timeframe: a map needs a default")
		}
		d, err := ParseDuration(raw.Default)
		if err != nil {
			return fmt.Errorf("timeframe.default: %w", err)
		}
		t.Default = d
		if raw.Min != "" {
			if t.Min, err = ParseDuration(raw.Min); err != nil {
				return fmt.Errorf("timeframe.min: %w", err)
			}
		}
		if raw.Max != "" {
			if t.Max, err = ParseDuration(raw.Max); err != nil {
				return fmt.Errorf("timeframe.max: %w", err)
			}
		}
		t.Fixed, t.Inline, t.Align = raw.Fixed, raw.Inline, raw.Align
		return nil
	}
	return fmt.Errorf("timeframe must be a duration, none, or a map")
}

// Describe renders the timeframe policy for help and describe output.
func (t Timeframe) Describe() string {
	if t.None {
		return "none (state query; no window)"
	}
	parts := []string{"default " + FormatDuration(t.Default)}
	if t.Min > 0 {
		parts = append(parts, "min "+FormatDuration(t.Min))
	}
	if t.Max > 0 {
		parts = append(parts, "max "+FormatDuration(t.Max))
	}
	if t.Fixed {
		parts = append(parts, "fixed")
	}
	if t.Align != "" {
		parts = append(parts, "aligned to "+t.Align)
	}
	if t.Inline {
		parts = append(parts, "written into the query")
	}
	return strings.Join(parts, ", ")
}

// Next is a follow-up the envelope suggests after a run.
type Next struct {
	Recipe string `json:"recipe" yaml:"recipe"`
	// With binds params of the next recipe from this invocation's params:
	// {service: "{{.service}}"}.
	With map[string]string `json:"with,omitempty" yaml:"with,omitempty"`
	// Bind binds params of the next recipe from a field of the first result
	// row: {service: dt.service.name}.
	Bind map[string]string `json:"bind,omitempty" yaml:"bind,omitempty"`
	// When is always (default), empty, or nonempty.
	When string `json:"when,omitempty" yaml:"when,omitempty"`
}

// Deprecated marks a recipe scheduled for removal.
type Deprecated struct {
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty" yaml:"replacedBy,omitempty"`
}

// Domain is one entry of the domain registry.
type Domain struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Source      Source `json:"-" yaml:"-"`
}

// ScopeDim is one scope dimension (design §10): a cross-cutting filter that
// every recipe declaring it accepts under the same flag with the same meaning.
type ScopeDim struct {
	Name         string `json:"name" yaml:"-"`
	Field        string `json:"field,omitempty" yaml:"field,omitempty"`
	FieldPattern string `json:"fieldPattern,omitempty" yaml:"fieldPattern,omitempty"`
	Form         string `json:"form,omitempty" yaml:"form,omitempty"`
	Description  string `json:"description" yaml:"description"`
	Source       Source `json:"-" yaml:"-"`
}

// FlagName is the dimension's flag spelling.
func (d *ScopeDim) FlagName() string { return strings.ReplaceAll(d.Name, "_", "-") }

// KeyValue reports whether the dimension takes key=value (primary tags).
func (d *ScopeDim) KeyValue() bool { return d.Form == "key=value" }

// domainsDoc is the _domains.yaml shape.
type domainsDoc struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Domains    map[string]string `yaml:"domains"`
}

// scopesDoc is the _scopes.yaml shape.
type scopesDoc struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Scopes     map[string]*ScopeDim `yaml:"scopes"`
}

// Bundle is the content of an app-shipped recipe bundle document (an ai-agent-resource named recipes/<name>.yaml) (design §13).
type Bundle struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   BundleMetadata `yaml:"metadata"`
	Spec       BundleSpec     `yaml:"spec"`
}

// BundleMetadata identifies a bundle.
type BundleMetadata struct {
	Name            string `yaml:"name"`
	Version         int    `yaml:"version"`
	MinDtctlVersion string `yaml:"minDtctlVersion"`
}

// BundleSpec is what a bundle carries.
type BundleSpec struct {
	Domains      map[string]string                   `yaml:"domains"`
	Capabilities map[string]*inventory.CapabilityDef `yaml:"capabilities"`
	// Fragments is template text with {{define}} blocks, visible only to this
	// bundle's recipes.
	Fragments string         `yaml:"fragments"`
	Recipes   []bundleRecipe `yaml:"recipes"`
}

// bundleRecipe is a recipe inside a bundle: the same schema without
// apiVersion/kind.
type bundleRecipe struct {
	Metadata Metadata `yaml:"metadata"`
	Spec     Spec     `yaml:"spec"`
}
