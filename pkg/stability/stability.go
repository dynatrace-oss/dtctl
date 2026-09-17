// Package stability declares and resolves dtctl's command stability contract:
// what we promise about a command's, flag's or output field's shape over time.
//
// Stability is a third axis, independent of the safety level ("what may this
// command do?") and the command profile ("which commands exist here?"). All
// three are filters in series, and every one of them can only *narrow* the
// surface:
//
//  1. registration      development opt-in     before the tree exists
//  2. profile mask      topical allowlist      startup, shapes the tree
//  3. stability floor   contract filter        startup, shapes the tree
//  4. safety level      permission check       runtime, per operation
//
// Levels are declared on the command as Cobra annotations rather than in a
// central table, so they travel with the command and cannot drift from it —
// unlike commands.MutatingVerbs and commands.ResourceAliases, both of which
// needed dedicated drift tests to stay honest.
//
// See dtctl-contrib dev/STABILITY_TIERS_DESIGN.md.
package stability

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtctl/sdk/session"
)

// Level is the ordered stability axis. Re-exported from sdk/session so callers
// need not import both packages for the common case.
type Level = session.StabilityLevel

// The three tiers, in ascending order of promise.
const (
	Development  = session.StabilityDevelopment
	Experimental = session.StabilityExperimental
	Stable       = session.StabilityStable

	// Default is the level of anything carrying no annotation.
	Default = session.DefaultStabilityLevel

	// DefaultFloor is the stability floor when a context sets none. It is
	// deliberately NOT Default: the floor admits experimental surface so humans
	// and interactive agents keep getting new commands (badged), while the
	// platform service and CI pin stable.
	DefaultFloor = session.DefaultMinStability
)

// ValidLevels returns the tiers in ascending order of promise.
func ValidLevels() []Level { return session.ValidStabilityLevels() }

// Annotation keys. Namespaced so they cannot collide with Cobra's own
// annotations (e.g. cobra.BashCompOneRequiredFlag) or with a future consumer's.
const (
	// AnnotationLevel holds the declared Level for a command or flag.
	AnnotationLevel = "dtctl.io/stability"
	// AnnotationSince holds the dtctl version at which the command or flag
	// entered its current level. It drives tier expiry: a feature that has sat
	// below stable for too many releases must be promoted or removed.
	AnnotationSince = "dtctl.io/stability-since"
	// AnnotationFeature holds the development-registry feature key for a
	// development-tier command, so the block message can name the opt-in.
	AnnotationFeature = "dtctl.io/development-feature"
	// AnnotationDeprecatedSince, AnnotationDeprecatedRemoveIn and
	// AnnotationDeprecatedReplacement carry deprecation metadata. Deprecation
	// is deliberately *not* a fourth tier: a deprecated command is still
	// stable in shape, it is merely scheduled for removal, so folding it into
	// the enum would make it look like a demotion.
	AnnotationDeprecatedSince       = "dtctl.io/deprecated-since"
	AnnotationDeprecatedRemoveIn    = "dtctl.io/deprecated-remove-in"
	AnnotationDeprecatedReplacement = "dtctl.io/deprecated-replacement"
)

// Deprecation describes a stable command's scheduled removal.
type Deprecation struct {
	// Since is the dtctl version that deprecated the command.
	Since string
	// RemoveIn is the version it is scheduled to disappear in.
	RemoveIn string
	// Replacement is the command to use instead, if there is one.
	Replacement string
}

// Note renders a deprecation as a single human-readable clause, shared by the
// help badge, the manifest and the commands catalog so the three never drift.
func (d Deprecation) Note() string {
	parts := []string{}
	if d.Since != "" {
		parts = append(parts, "deprecated since "+d.Since)
	} else {
		parts = append(parts, "deprecated")
	}
	if d.RemoveIn != "" {
		parts = append(parts, "removal planned in "+d.RemoveIn)
	}
	if d.Replacement != "" {
		parts = append(parts, "use `"+d.Replacement+"` instead")
	}
	return strings.Join(parts, "; ")
}

// Mark declares a command's stability level. `since` is the dtctl version at
// which it entered that level and is required for any level below stable, so
// tier expiry has something to measure.
//
// Marking a command Stable is a no-op on the annotations: stable is the
// default, and writing it explicitly would make the manifest noisier without
// changing any behaviour.
func Mark(cmd *cobra.Command, level Level, since string) {
	if cmd == nil || level == Stable || level == "" {
		return
	}
	setAnnotation(cmd, AnnotationLevel, string(level))
	if since != "" {
		setAnnotation(cmd, AnnotationSince, since)
	}
}

// MarkDevelopment declares a command as development-tier and binds it to a
// registry feature key. The key is what a caller enables
// (`dtctl config set development.<feature> on`), and it is what the block
// message names.
//
// Marking alone does not hide anything: a development command must additionally
// not be registered on the tree unless its feature is enabled. Register does
// both — prefer it.
func MarkDevelopment(cmd *cobra.Command, feature string) {
	if cmd == nil {
		return
	}
	setAnnotation(cmd, AnnotationLevel, string(Development))
	setAnnotation(cmd, AnnotationFeature, feature)
}

// MarkFlag declares a flag's stability level. A flag may make a *weaker*
// promise than its command — an experimental flag on a stable command is how a
// new idea ships without inventing a new command — but never a stronger one;
// Lint reports the inverted case.
func MarkFlag(cmd *cobra.Command, name string, level Level, since string) {
	if cmd == nil || level == Stable || level == "" {
		return
	}
	f := cmd.Flags().Lookup(name)
	if f == nil {
		f = cmd.PersistentFlags().Lookup(name)
	}
	if f == nil {
		return
	}
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[AnnotationLevel] = []string{string(level)}
	if since != "" {
		f.Annotations[AnnotationSince] = []string{since}
	}
}

// Deprecate records a scheduled removal. It does not change the command's
// level: the only exit from stable is deprecation, and a deprecated command
// keeps working exactly as documented until it is removed.
func Deprecate(cmd *cobra.Command, d Deprecation) {
	if cmd == nil {
		return
	}
	setAnnotation(cmd, AnnotationDeprecatedSince, d.Since)
	setAnnotation(cmd, AnnotationDeprecatedRemoveIn, d.RemoveIn)
	setAnnotation(cmd, AnnotationDeprecatedReplacement, d.Replacement)
}

// Of returns a command's own declared level, ignoring its ancestors. Absence of
// an annotation means Default (stable).
func Of(cmd *cobra.Command) Level {
	if cmd == nil {
		return Default
	}
	lvl := Level(cmd.Annotations[AnnotationLevel])
	if !lvl.IsValid() || lvl == "" {
		return Default
	}
	return lvl
}

// Since returns the version at which a command entered its current level.
func Since(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Annotations[AnnotationSince]
}

// Feature returns the development-registry feature key bound to a command, or
// "" when it is not development-tier.
func Feature(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Annotations[AnnotationFeature]
}

// DeprecationOf returns a command's deprecation metadata, and false when it is
// not deprecated.
func DeprecationOf(cmd *cobra.Command) (Deprecation, bool) {
	if cmd == nil {
		return Deprecation{}, false
	}
	d := Deprecation{
		Since:       cmd.Annotations[AnnotationDeprecatedSince],
		RemoveIn:    cmd.Annotations[AnnotationDeprecatedRemoveIn],
		Replacement: cmd.Annotations[AnnotationDeprecatedReplacement],
	}
	return d, d.Since != "" || d.RemoveIn != ""
}

// Effective returns a command's stability as callers actually experience it:
// the weakest level along the path from root down to the command. A stable
// subcommand under an experimental verb is stable in name only.
func Effective(cmd *cobra.Command) Level {
	lvl := Default
	for c := cmd; c != nil; c = c.Parent() {
		lvl = session.Weakest(lvl, Of(c))
	}
	return lvl
}

// OfFlag returns a flag's own declared level, ignoring its command. Absence of
// an annotation means Default.
func OfFlag(cmd *cobra.Command, name string) Level {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return Default
	}
	return flagLevel(f.Annotations)
}

// EffectiveFlag returns a flag's stability as callers experience it: the weakest
// of the flag's own level and its command's effective level.
func EffectiveFlag(cmd *cobra.Command, name string) Level {
	return session.Weakest(Effective(cmd), OfFlag(cmd, name))
}

// SinceFlag returns the version at which a flag entered its current level.
func SinceFlag(cmd *cobra.Command, name string) string {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return ""
	}
	return flagSince(f.Annotations)
}

// flagLevel extracts a level from a pflag annotation map.
func flagLevel(annotations map[string][]string) Level {
	vals := annotations[AnnotationLevel]
	if len(vals) == 0 {
		return Default
	}
	lvl := Level(vals[0])
	if !lvl.IsValid() || lvl == "" {
		return Default
	}
	return lvl
}

// flagSince extracts a since-version from a pflag annotation map.
func flagSince(annotations map[string][]string) string {
	vals := annotations[AnnotationSince]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Badge returns the help-text badge for a level, or "" for stable. Badges never
// stack: Badge is only ever called with the single winning marker, whose
// precedence is deprecated > development > experimental > upstream preview.
func Badge(level Level) string {
	switch level {
	case Development:
		return "[Development]"
	case Experimental:
		return "[Experimental]"
	default:
		return ""
	}
}

// Guarantee is the one-line statement of what a level promises. It is emitted
// alongside every badge rather than left to be inferred from the tier's name:
// AIP-181 defines "experimental" as the *weakest* level, so a reader may
// otherwise read the middle tier as weaker than we intend.
func Guarantee(level Level) string {
	switch level {
	case Development:
		return "Development features are unfinished, carry no stability guarantees, " +
			"and may change or be removed without notice."
	case Experimental:
		return "Experimental commands and flags may change or be removed in any " +
			"release and are not covered by dtctl's stability guarantees."
	default:
		return "Stable commands and flags change additively only; removal requires " +
			"a deprecation cycle."
	}
}

// setAnnotation writes a command annotation, allocating the map on first use.
// An empty value is skipped so absent metadata never materializes as "".
func setAnnotation(cmd *cobra.Command, key, value string) {
	if value == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[key] = value
}

// Walk invokes fn for cmd and every command in its subtree.
func Walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		Walk(sub, fn)
	}
}

// Path returns a command's path relative to root, space-joined ("get
// workflows"). This is the vocabulary the profile allowlist, the commands
// catalog and the stability exception list all share.
func Path(cmd, root *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), root.Name()))
}

// Lint reports declarations that are internally inconsistent. These are author
// errors rather than user errors, so the manifest test fails on them.
func Lint(root *cobra.Command) []error {
	var problems []error
	Walk(root, func(cmd *cobra.Command) {
		path := Path(cmd, root)
		if path == "" {
			return
		}
		own := Of(cmd)

		// A level below stable must say when it got there, or expiry has
		// nothing to measure. Development is exempt: it is unreleased by
		// definition, so "since" would be meaningless.
		if own == Experimental && Since(cmd) == "" {
			problems = append(problems, fmt.Errorf(
				"%s: declared %s without a since-version", path, own))
		}
		if own == Development && Feature(cmd) == "" {
			problems = append(problems, fmt.Errorf(
				"%s: declared %s without a development feature key", path, own))
		}

		effective := Effective(cmd)
		visitFlags(cmd, func(f flagInfo) {
			// Only an *explicit* declaration can be inconsistent. An
			// unannotated flag has no opinion of its own and simply inherits
			// the command's level — otherwise every flag on an experimental
			// command would have to repeat the annotation to stay silent.
			if !f.declared {
				return
			}
			// A flag may be weaker than its command, never stronger.
			if f.level.Rank() > effective.Rank() {
				problems = append(problems, fmt.Errorf(
					"%s --%s: flag declared %s under a %s command; a flag may be "+
						"weaker than its command, never stronger", path, f.name, f.level, effective))
			}
		})
	})
	return problems
}

// flagInfo is what a flag declares about itself.
type flagInfo struct {
	name  string
	level Level
	since string
	// declared distinguishes "explicitly marked stable" from "carries no
	// annotation at all". The two resolve to the same level but mean different
	// things to the lint.
	declared bool
}

// visitFlags invokes fn for each of a command's local (non-inherited) flags.
// Inherited persistent flags belong to the ancestor that declared them and are
// visited there.
func visitFlags(cmd *cobra.Command, fn func(flagInfo)) {
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if cmd.InheritedFlags().Lookup(f.Name) != nil {
			return
		}
		fn(flagInfo{
			name:     f.Name,
			level:    flagLevel(f.Annotations),
			since:    flagSince(f.Annotations),
			declared: len(f.Annotations[AnnotationLevel]) > 0,
		})
	})
}
