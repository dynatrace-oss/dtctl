package stability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/sdk/session"
)

// ManifestHeader is the preamble of the generated manifest. It states the
// promise the manifest encodes, because the file is the artifact reviewers and
// downstream consumers read.
const ManifestHeader = `# dtctl stability manifest

<!-- GENERATED FILE — do not edit. Regenerate with: make stability-manifest -->

Every command and flag dtctl exposes, with the contract it carries. This file is
generated from the live command tree and checked in, so a change to the
contract shows up as a reviewable diff and a failing build rather than as a
silent break.

| Tier | Promise |
|---|---|
| ` + "`stable`" + ` | Invocation and output contract are additive-only. Removal or an incompatible change requires a deprecation cycle. |
| ` + "`experimental`" + ` | May change or be removed in any release. Not covered by dtctl's stability guarantees. |
| ` + "`development`" + ` | Unfinished, no guarantees. Not registered unless opted in via ` + "`dtctl config set development.<feature> on`" + `. |

Deprecation is not a tier: a deprecated command is still stable in shape and is
merely scheduled for removal. It is recorded on the same line.

## Choosing what this environment accepts

The **stability floor** is the weakest contract a command or flag may offer and
still be usable. It defaults to ` + "`experimental`" + `, so interactive use keeps
getting new (badged) surface. Automation should pin ` + "`stable`" + `:

` + "```bash" + `
# Per context, persisted in the config file
dtctl config set-context prod-agent --min-stability stable

# Per process — this is what an embedding service sets, and it is deliberately
# not a command-line flag: the embedder controls argv, so a flag would hand the
# opt-in to exactly the party the floor exists to constrain.
DTCTL_MIN_STABILITY=stable dtctl get workflows
` + "```" + `

A single below-floor command or flag can be admitted without lowering the floor
for everything. Each entry is one audited risk acceptance. Naming a command
also admits the flags that are below the floor only because that command is;
a flag with a weaker contract of its own needs its own entry, because there
the flag — not the command — is the risk being accepted:

` + "```bash" + `
dtctl config set-context prod-agent --min-stability stable \
  --stability-exception 'inventory' \
  --stability-exception 'query --decode-snapshots'
` + "```" + `

Development-tier features are enabled individually and never by a floor:

` + "```bash" + `
dtctl config list-development                 # what this build carries
dtctl config set development.serve on         # persistent
DTCTL_DEVELOPMENT=serve dtctl serve http      # one process
` + "```" + `

### Embedded and service callers

When dtctl is embedded (` + "`pkg/engine`" + `, ` + "`cmd.Session`" + `) the floor is a
per-request field, and it defaults to ` + "`stable`" + ` rather than
` + "`experimental`" + `. A request is unattended automation: nobody reads the
` + "`[Experimental]`" + ` badge on its behalf. Widen it deliberately, and prefer
naming the individual commands the host has tested over granting the whole tier:

` + "```go" + `
engine.Request{
    Command:             "inventory --agent",
    MinStability:        "",                      // stable, the safe default
    StabilityExceptions: []string{"inventory"},   // just this one, audited
}
` + "```" + `

Environment variables do not reach a session-backed run: ` + "`DTCTL_MIN_STABILITY`" + `
and ` + "`DTCTL_DEVELOPMENT`" + ` are scrubbed, because which commands exist is the
request's decision and not the host process's. See
` + "`docs/dev/SERVICE_ENGINE_DESIGN.md`" + `.

> **Versioning caveat.** ` + "`release-please-config.json`" + ` sets
> ` + "`bump-minor-pre-major`" + `, so pre-1.0 a breaking change currently ships as a
> minor bump. The ` + "`stable`" + ` tier is only as strong as the carve-out that
> exempts it from that policy — see the design doc's "Versioning policy"
> section. Until that is written down, treat this file as the inventory, not
> yet as the guarantee.

`

// entry is one manifest line: a command, or a flag under a command.
type entry struct {
	path        string
	flag        string // "" for a command line
	level       Level
	since       string
	feature     string // development feature key
	deprecation *Deprecation
}

// Manifest renders the checked-in stability manifest for a command tree.
//
// Only non-default declarations and deprecations are listed in the per-tier
// detail sections; the stable surface is enumerated in full so that *removing*
// a stable command or flag is also a diff. That asymmetry is the point: the
// file exists to make breaking a promise visible.
func Manifest(root *cobra.Command) string {
	entries := collect(root)

	var b strings.Builder
	b.WriteString(ManifestHeader)

	counts := map[Level]int{}
	for _, e := range entries {
		if e.flag == "" {
			counts[e.level]++
		}
	}
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- commands: %d stable, %d experimental, %d development\n",
		counts[Stable], counts[Experimental], counts[Development]))
	b.WriteString(fmt.Sprintf("- entries below (commands + flags): %d\n\n", len(entries)))

	b.WriteString("## Surface\n\n")
	b.WriteString("```\n")
	width := 0
	for _, e := range entries {
		if n := len(e.label()); n > width {
			width = n
		}
	}
	for _, e := range entries {
		b.WriteString(e.render(width))
		b.WriteByte('\n')
	}
	b.WriteString("```\n")
	return b.String()
}

// label is the left-hand column: a command path, or an indented flag name.
func (e entry) label() string {
	if e.flag == "" {
		return e.path
	}
	return "  --" + e.flag
}

// render formats one manifest line, padded to align the tier column.
func (e entry) render(width int) string {
	line := fmt.Sprintf("%-*s  %s", width, e.label(), e.level)
	if e.since != "" {
		line += "  since " + e.since
	}
	if e.feature != "" {
		line += "  (opt-in key: " + e.feature + ")"
	}
	if e.deprecation != nil {
		d := *e.deprecation
		line += fmt.Sprintf("  deprecated %s", d.Since)
		if d.RemoveIn != "" {
			line += " → remove " + d.RemoveIn
		}
		if d.Replacement != "" {
			line += ", use `" + d.Replacement + "`"
		}
	}
	return strings.TrimRight(line, " ")
}

// collect walks the tree and returns every command and local flag, sorted by
// command path with each command's flags immediately beneath it.
//
// The caller is responsible for handing in a *complete* tree: the generator
// enables every development feature first, so development commands appear even
// though a released build does not register them. The manifest is a
// maintainer-facing inventory checked into the repo, not a runtime discovery
// surface, so it does not participate in the non-disclosure rules that govern
// help and the catalog.
func collect(root *cobra.Command) []entry {
	var entries []entry
	Walk(root, func(cmd *cobra.Command) {
		path := Path(cmd, root)
		if path == "" {
			return // the root command itself carries no contract
		}
		if cmd.Hidden && Of(cmd) == Default {
			// Hidden-and-unmarked commands are internal plumbing (e.g. the
			// hidden `exec dql` alias), not part of the promised surface.
			return
		}
		e := entry{
			path:    path,
			level:   Effective(cmd),
			since:   Since(cmd),
			feature: Feature(cmd),
		}
		if d, ok := DeprecationOf(cmd); ok {
			e.deprecation = &d
		}
		entries = append(entries, e)

		visitFlags(cmd, func(f flagInfo) {
			entries = append(entries, entry{
				path: path,
				flag: f.name,
				// The flag's own promise is capped by its command's: a stable
				// flag on an experimental command is stable in name only.
				level: session.Weakest(e.level, f.level),
				since: f.since,
			})
		})
	})

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].path != entries[j].path {
			return entries[i].path < entries[j].path
		}
		// Command line first, then its flags alphabetically.
		if (entries[i].flag == "") != (entries[j].flag == "") {
			return entries[i].flag == ""
		}
		return entries[i].flag < entries[j].flag
	})
	return entries
}
