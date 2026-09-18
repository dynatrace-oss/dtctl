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

	// Undeclared is the absence of a declaration -- the node states nothing
	// about its own contract. It is not a tier and never renders as one.
	//
	// For a *flag* it means inheritance: the flag stands or falls with its
	// command. For a *command* it is an error the build rejects, and at
	// runtime it resolves to Fallback rather than to a promise nobody made.
	Undeclared = session.StabilityLevel("")

	// Fallback is what an undeclared command resolves to. See
	// session.FallbackStabilityLevel for why it is not stable.
	Fallback = session.FallbackStabilityLevel

	// DefaultFloor is the stability floor when a context sets none. It is
	// deliberately NOT Stable: the floor admits experimental surface so humans
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
// Stable is recorded like any other level. It used to be a no-op here, on the
// reasoning that stable was the default and writing it down only added noise.
// That had it backwards: it made the strongest promise dtctl offers the one
// tier nobody had to choose, so a command shipped it by omission. Every
// command now declares, and Lint fails the build for one that does not.
func Mark(cmd *cobra.Command, level Level, since string) {
	if cmd == nil || level == Undeclared {
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
// an annotation is Undeclared -- the command states nothing, which is not the
// same as stating stable. See Effective for what that resolves to.
func Of(cmd *cobra.Command) Level {
	if cmd == nil {
		return Undeclared
	}
	lvl := Level(cmd.Annotations[AnnotationLevel])
	if !lvl.IsValid() || lvl == "" {
		return Undeclared
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
	lvl := Stable
	for c := cmd; c != nil; c = c.Parent() {
		own := Of(c)
		if own == Undeclared {
			// The root is not a command and carries no contract of its own,
			// so its silence is correct rather than an omission.
			if c.Parent() == nil {
				continue
			}
			// Nothing here declared anything. Fall back instead of inheriting
			// the strongest promise by default.
			own = Fallback
		}
		lvl = session.Weakest(lvl, own)
	}
	return lvl
}

// MarkStable declares a command stable: its invocation and output contract are
// additive-only from here on.
//
// It takes no since-version. Since drives tier expiry -- how long a feature has
// sat below stable before it must be promoted or dropped -- and stable is the
// terminus, so there is nothing left to measure. It would also be a fiction for
// the surface that predates this declaration, which has been stable for many
// releases and not since the one that wrote the annotation down.
func MarkStable(cmd *cobra.Command) { Mark(cmd, Stable, "") }

// Declared reports whether a command states a tier of its own.
func Declared(cmd *cobra.Command) bool { return Of(cmd) != Undeclared }

// DeclarationRequired reports whether a command must declare a tier of its own
// for the build to pass.
//
// Two exemptions, both because the command is already outside the promised
// surface rather than silently inside it:
//
//   - An ancestor declared a level below stable. That is one explicit demotion
//     covering a subtree -- `account` marked development speaks for every
//     `account *` -- not an omission, and making each child repeat it would
//     turn a single reviewed decision into eight.
//
// Hidden commands are *not* exempt. Hiding a command removes it from help, not
// from the tree: `dtctl exec dql` still runs, so something still has to decide
// whether a stable floor accepts it. Leaving that to the fallback would demote
// three working commands as a side effect of a docs decision.
func DeclarationRequired(cmd *cobra.Command, root *cobra.Command) bool {
	if cmd == nil || cmd == root || Path(cmd, root) == "" {
		return false
	}
	for a := cmd.Parent(); a != nil; a = a.Parent() {
		if lvl := Of(a); lvl != Undeclared && lvl != Stable {
			return false
		}
	}
	return true
}

// OfFlag returns a flag's own declared level, ignoring its command. Absence of
// an annotation is Undeclared, which for a flag means it inherits its
// command's level.
func OfFlag(cmd *cobra.Command, name string) Level {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return Undeclared
	}
	return flagLevel(f.Annotations)
}

// OfFlagValue returns a flag's own declared level from the flag itself, for
// callers that already hold it.
//
// OfFlag cannot serve a global flag: before cobra merges persistent flags,
// cmd.Flags() on a subcommand does not see them, so the lookup returns
// Undeclared -- indistinguishable from a global flag that simply inherits.
// Walking ancestors by hand and reading the flag directly is the only way to
// ask about a global flag without triggering that merge.
func OfFlagValue(f *pflag.Flag) Level {
	if f == nil {
		return Undeclared
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
		return Undeclared
	}
	lvl := Level(vals[0])
	if !lvl.IsValid() || lvl == "" {
		return Undeclared
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
		return "Development features are unfinished, carry no guarantees of any " +
			"kind -- including the backend's, which may not be deployed on this " +
			"environment at all -- and may change or be removed without notice."
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

		// Every command states its own tier. Stable is the strongest promise
		// dtctl makes, and it used to be what a command got for saying
		// nothing -- so a command shipped an additive-only contract because
		// nobody chose otherwise. Declaring is now the author's job, and this
		// is where forgetting costs a red build instead of a promise.
		if own == Undeclared && DeclarationRequired(cmd, root) {
			problems = append(problems, fmt.Errorf(
				"%s: declares no stability tier; add stability.MarkStable(cmd) if "+
					"its invocation and output contract are additive-only from now on, "+
					"or stability.Mark(cmd, stability.Experimental, \"<version>\") if not "+
					"(it resolves to %s until then)", path, Fallback))
		}

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
