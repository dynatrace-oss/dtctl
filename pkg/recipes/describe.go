package recipes

import (
	"strings"
	"time"
)

// Description is a recipe as `dtctl describe recipe` reports it: everything a
// caller needs to run it and read its result, without the YAML.
type Description struct {
	Name        string   `json:"name" yaml:"name"`
	Summary     string   `json:"summary" yaml:"summary"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Domain      string   `json:"domain" yaml:"domain"`
	Version     int      `json:"version" yaml:"version"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Source      string   `json:"source" yaml:"source"`
	Location    string   `json:"location,omitempty" yaml:"location,omitempty"`
	// Shadows are recipes of the same name this one replaces.
	Shadows  []string    `json:"shadows,omitempty" yaml:"shadows,omitempty"`
	Usage    string      `json:"usage" yaml:"usage"`
	Params   []ParamInfo `json:"params,omitempty" yaml:"params,omitempty"`
	Scope    []ScopeInfo `json:"scope,omitempty" yaml:"scope,omitempty"`
	Window   string      `json:"window" yaml:"window"`
	Segments bool        `json:"segments" yaml:"segments"`
	Requires []string    `json:"requires,omitempty" yaml:"requires,omitempty"`
	// Means says how to read the result; EmptyMeans what an empty one means.
	Means      string   `json:"means" yaml:"means"`
	EmptyMeans string   `json:"emptyMeans" yaml:"emptyMeans"`
	Next       []string `json:"next,omitempty" yaml:"next,omitempty"`
	Deprecated string   `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	// DQL is the query rendered with defaults, required params shown as
	// placeholders; RenderError says why it could not be rendered.
	DQL         string `json:"dql,omitempty" yaml:"dql,omitempty"`
	RenderError string `json:"renderError,omitempty" yaml:"renderError,omitempty"`
}

// ParamInfo is one param as a flag.
type ParamInfo struct {
	Flag        string   `json:"flag" yaml:"flag"`
	Type        string   `json:"type" yaml:"type"`
	Required    bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Positional  bool     `json:"positional,omitempty" yaml:"positional,omitempty"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
	Values      []string `json:"values,omitempty" yaml:"values,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
}

// ScopeInfo is one scope dimension the recipe accepts.
type ScopeInfo struct {
	Flag        string `json:"flag" yaml:"flag"`
	Field       string `json:"field" yaml:"field"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Describe assembles the description of r; now anchors the example window.
func (b *Book) Describe(r *Recipe, now time.Time) Description {
	d := Description{
		Name:        r.Name(),
		Summary:     r.Spec.Summary,
		Description: strings.TrimSpace(r.Spec.Description),
		Domain:      r.Domain(),
		Version:     r.Metadata.Version,
		Tags:        r.Metadata.Tags,
		Source:      r.Source.String(),
		Window:      r.Spec.Timeframe.Describe(),
		Segments:    !r.Spec.Segments.Off(),
		Requires:    r.Spec.Requires,
		Means:       strings.TrimSpace(r.Spec.Means),
		EmptyMeans:  strings.TrimSpace(r.Spec.EmptyMeans),
	}
	if r.Source.Layer != LayerBuiltin {
		d.Location = r.Source.Location
	}
	for _, s := range r.Shadows {
		d.Shadows = append(d.Shadows, s.String())
	}
	usage := []string{"dtctl run " + r.Name()}
	if a := RequiredArgs(r); a != "" {
		usage = append(usage, a)
	}
	d.Usage = strings.Join(usage, " ")
	for _, p := range r.Spec.Params {
		d.Params = append(d.Params, ParamInfo{
			Flag:        "--" + p.FlagName(),
			Type:        string(p.Type),
			Required:    p.Required,
			Positional:  p.Positional,
			Default:     p.DefaultString(),
			Values:      p.Values,
			Description: p.Description,
		})
	}
	for _, name := range r.Spec.Scope {
		if dim := b.Scopes[name]; dim != nil {
			field := dim.Field
			if field == "" {
				field = dim.FieldPattern
			}
			d.Scope = append(d.Scope, ScopeInfo{Flag: "--" + dim.FlagName(), Field: field, Description: dim.Description})
		}
	}
	for _, n := range r.Spec.Next {
		if b.Get(n.Recipe) != nil {
			d.Next = append(d.Next, n.Recipe)
		}
	}
	if r.Spec.Deprecated != nil {
		d.Deprecated = strings.TrimSpace(r.Spec.Deprecated.Message)
		if r.Spec.Deprecated.ReplacedBy != "" {
			d.Deprecated = strings.TrimSpace(d.Deprecated + " Replaced by " + r.Spec.Deprecated.ReplacedBy + ".")
		}
		if d.Deprecated == "" {
			d.Deprecated = "yes"
		}
	}
	rendered, err := b.Example(r, now)
	if err != nil {
		d.RenderError = err.Error()
	} else {
		d.DQL = rendered.DQL
	}
	return d
}

// Example renders r with defaults and placeholders, over its default window.
func (b *Book) Example(r *Recipe, now time.Time) (*Rendered, error) {
	w, err := ResolveWindow(r.Spec.Timeframe, "", "", now)
	if err != nil {
		return nil, err
	}
	return b.Render(r, Input{Window: w, Placeholders: true})
}
