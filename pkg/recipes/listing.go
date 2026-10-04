package recipes

import (
	"sort"
	"strings"
)

// ListItem is one recipe in `dtctl get recipes`.
type ListItem struct {
	Name    string `json:"name" yaml:"name" table:"NAME"`
	Summary string `json:"summary" yaml:"summary" table:"SUMMARY"`
	// Args are the required params as typed: "<service>" for a positional
	// one, "--id <id>" for a flag.
	Args     string   `json:"args,omitempty" yaml:"args,omitempty" table:"ARGS"`
	Window   string   `json:"window" yaml:"window" table:"WINDOW"`
	Source   string   `json:"source" yaml:"source" table:"SOURCE"`
	Domain   string   `json:"-" yaml:"-" table:"-"`
	Tags     []string `json:"tags,omitempty" yaml:"tags,omitempty" table:"TAGS,wide"`
	Requires []string `json:"requires,omitempty" yaml:"requires,omitempty" table:"REQUIRES,wide"`
	// Deprecated names the replacement, or says "yes" when there is none.
	Deprecated string `json:"deprecated,omitempty" yaml:"deprecated,omitempty" table:"-"`
}

// DomainItem is one line of the domain index: the first thing an agent sees,
// one line per domain however large the book grows.
type DomainItem struct {
	Domain      string `json:"domain" yaml:"domain" table:"DOMAIN"`
	Description string `json:"description" yaml:"description" table:"DESCRIPTION"`
	// Available counts the recipes inventory did not hide; Total counts all.
	Available int      `json:"available" yaml:"available" table:"AVAILABLE"`
	Total     int      `json:"total" yaml:"total" table:"TOTAL"`
	Sources   []string `json:"sources,omitempty" yaml:"sources,omitempty" table:"SOURCES,wide"`
}

// Item renders a recipe for the listing.
func Item(r *Recipe) ListItem {
	it := ListItem{
		Name:     r.Name(),
		Summary:  r.Spec.Summary,
		Args:     RequiredArgs(r),
		Window:   shortWindow(r.Spec.Timeframe),
		Source:   r.Source.String(),
		Domain:   r.Domain(),
		Tags:     r.Metadata.Tags,
		Requires: r.Spec.Requires,
	}
	if d := r.Spec.Deprecated; d != nil {
		it.Deprecated = "yes"
		if d.ReplacedBy != "" {
			it.Deprecated = d.ReplacedBy
		}
	}
	return it
}

// RequiredArgs is the part of a command line a recipe cannot run without,
// plus an optional positional argument in brackets.
func RequiredArgs(r *Recipe) string {
	var parts []string
	for _, p := range r.Spec.Params {
		if p.Positional {
			arg := "<" + p.Name + ">"
			if !p.Required {
				arg = "[" + arg + "]"
			}
			parts = append([]string{arg}, parts...)
			continue
		}
		if !p.Required {
			continue
		}
		parts = append(parts, "--"+p.FlagName()+" <"+p.Name+">")
	}
	return strings.Join(parts, " ")
}

func shortWindow(t Timeframe) string {
	switch {
	case t.None:
		return "none"
	case t.Fixed:
		return FormatDuration(t.Default) + " (fixed)"
	case t.Align == AlignUTCDay:
		return FormatDuration(t.Default) + " (UTC days)"
	}
	return FormatDuration(t.Default)
}

// DomainIndex groups recipes by domain. hidden names recipes inventory hid;
// they count toward Total but not Available.
func (b *Book) DomainIndex(all []*Recipe, hidden map[string]bool) []DomainItem {
	byDomain := map[string]*DomainItem{}
	sources := map[string]map[string]bool{}
	for _, r := range all {
		d := r.Domain()
		it := byDomain[d]
		if it == nil {
			desc := ""
			if dom := b.Domains[d]; dom != nil {
				desc = dom.Description
			}
			it = &DomainItem{Domain: d, Description: desc}
			byDomain[d] = it
			sources[d] = map[string]bool{}
		}
		it.Total++
		if !hidden[r.Name()] {
			it.Available++
		}
		sources[d][string(r.Source.Layer)] = true
	}
	out := make([]DomainItem, 0, len(byDomain))
	for d, it := range byDomain {
		for _, l := range Precedence {
			if sources[d][string(l)] {
				it.Sources = append(it.Sources, string(l))
			}
		}
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

// VerifyItem is one line of `dtctl verify recipe`.
type VerifyItem struct {
	Name    string `json:"name" yaml:"name" table:"NAME"`
	Status  string `json:"status" yaml:"status" table:"STATUS"`
	Message string `json:"message,omitempty" yaml:"message,omitempty" table:"MESSAGE"`
	Source  string `json:"source,omitempty" yaml:"source,omitempty" table:"SOURCE,wide"`
}
