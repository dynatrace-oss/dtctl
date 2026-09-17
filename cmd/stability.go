package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// This file is the enforcement half of the stability axis; pkg/stability holds
// the declaration and resolution half. Together with the command profile
// (surface) and the safety level (permission) it forms a four-stage pipeline
// that can only ever *narrow* what an invocation can reach:
//
//  1. registration      development opt-in     applyDevelopmentRegistration
//  2. profile mask       topical allowlist      applyProfile
//  3. stability floor    contract filter        applyStabilityFloor
//  4. safety level       permission check       pkg/safety, at runtime
//
// No later stage can re-add what an earlier one removed. In particular a
// stability exception cannot resurrect a development command that was never
// registered, and lowering the floor cannot widen a profile.
//
// See dtctl-contrib dev/STABILITY_TIERS_DESIGN.md.

// StabilityError is returned when a command or flag whose contract is weaker
// than the active stability floor is invoked. It is a *contract* error, and so
// distinct from both ProfileError (the topical surface axis) and
// safety.SafetyError (the permission axis) — the three answer different
// questions and a caller resolves them differently.
type StabilityError struct {
	// Command is the space-joined command path relative to root ("get workflows").
	Command string
	// Flag is the flag name (without dashes) when a flag rather than the whole
	// command is below the floor, and "" otherwise.
	Flag string
	// Level is the target's effective stability tier.
	Level stability.Level
	// Floor is the tier the active context requires.
	Floor stability.Level
}

// target names what was blocked, in the vocabulary a user would use.
func (e *StabilityError) target() string {
	if e.Flag == "" {
		return fmt.Sprintf("command %q", e.Command)
	}
	return fmt.Sprintf("flag --%s of command %q", e.Flag, e.Command)
}

// exceptionEntry is the exact string a caller would add to stability-exceptions
// to admit this one target.
func (e *StabilityError) exceptionEntry() string {
	if e.Flag == "" {
		return e.Command
	}
	return e.Command + " --" + e.Flag
}

// Headline is the one-line statement of the block, shared by the human-readable
// Error() string and the structured agent-mode ErrorDetail (errorToDetail) so
// the two never drift.
func (e *StabilityError) Headline() string {
	return fmt.Sprintf("%s is %s; this context requires %s or stronger",
		e.target(), e.Level, e.Floor)
}

// Suggestions are the remediation hints, shared with the agent-mode ErrorDetail.
// The narrow opt-in is listed first deliberately: lowering the floor grants the
// entire below-floor surface, including flags added to already-allowed commands
// in a later release, which is almost never what the caller wanted.
func (e *StabilityError) Suggestions() []string {
	return []string{
		fmt.Sprintf("allow just this one: add %q to stability-exceptions on the active context",
			e.exceptionEntry()),
		fmt.Sprintf("allow the whole %s surface: set min-stability on the context, or %s=%s",
			e.Level, config.MinStabilityEnvVar, e.Level),
		"run 'dtctl commands' to see the surface available at this floor",
	}
}

func (e *StabilityError) Error() string {
	return e.Headline() + "\n\n  " +
		strings.ReplaceAll(stability.Guarantee(e.Level), ". ", ".\n  ")
}

// DevelopmentError reports that an invoked command exists in this binary but is
// a development-tier feature that was not opted into.
//
// It is produced only when the caller has already demonstrated knowledge of the
// mechanism (see developmentSignposting). Otherwise a disabled development
// command is indistinguishable from one that does not exist, which is the whole
// point of gating by *registration* rather than by hiding.
type DevelopmentError struct {
	// Command is the command path the caller typed.
	Command string
	// Feature is the opt-in key that would enable it.
	Feature string
}

// Headline is the one-line statement of the block.
func (e *DevelopmentError) Headline() string {
	return fmt.Sprintf("command %q is a development feature and is not enabled", e.Command)
}

// Suggestions are the remediation hints, shared with the agent-mode ErrorDetail.
func (e *DevelopmentError) Suggestions() []string {
	return []string{
		fmt.Sprintf("enable it persistently: dtctl config set development.%s on", e.Feature),
		fmt.Sprintf("enable it for one process: %s=%s", config.DevelopmentEnvVar, e.Feature),
		"run 'dtctl config list-development' to see every development feature",
		"development features carry no guarantees and may change or disappear without notice",
	}
}

func (e *DevelopmentError) Error() string {
	return e.Headline() + "\n\n" +
		"  Development features are unfinished and unsupported. Enable it with\n" +
		"  'dtctl config set development." + e.Feature + " on' if you accept that.\n"
}

// --- Stage 1: registration ---------------------------------------------------

// developmentCommand is one gated command and the feature key that enables it.
type developmentCommand struct {
	parent  *cobra.Command
	cmd     *cobra.Command
	feature string
}

// developmentCommands is the table of development-tier commands, populated from
// init() wiring. Registration is decided per invocation rather than at init so
// that --config and --context are honored and so an embedded caller's request
// cannot inherit the previous request's surface.
var developmentCommands []developmentCommand

// AddDevelopmentCommand registers a top-level development-tier command. It is
// the gated counterpart of AddCommand and exists for the same reason: commands
// that live outside this package because they import packages that import cmd.
// `dtctl serve` (pkg/serve, wired in main) is the canonical case.
//
// The command is *declared* immediately — so a block message can name it — but
// only attached to the tree when its feature is enabled.
func AddDevelopmentCommand(c *cobra.Command, feature string) {
	addDevelopmentCommand(rootCmd, c, feature)
}

// addDevelopmentCommand declares a gated command under an arbitrary parent.
func addDevelopmentCommand(parent, c *cobra.Command, feature string) {
	stability.MarkDevelopment(c, feature)
	path := strings.TrimSpace(commandPathRelative(parent, rootCmd) + " " + c.Name())
	stability.DefaultRegistry().Declare(feature, path)
	developmentCommands = append(developmentCommands, developmentCommand{
		parent: parent, cmd: c, feature: feature,
	})
}

// applyDevelopmentRegistration attaches every enabled development command to its
// parent and detaches every disabled one. It is idempotent, and it is the only
// stage that changes the *shape* of the tree rather than masking within it:
// skipping registration is what makes a development feature unreachable by
// accident, since an invocation then yields the ordinary unknown-command error
// and the command is absent from help, completion and the catalog.
func applyDevelopmentRegistration(enabled map[string]bool) {
	for _, dc := range developmentCommands {
		if stability.Enabled(dc.feature, enabled) {
			if !hasSubcommand(dc.parent, dc.cmd) {
				dc.parent.AddCommand(dc.cmd)
			}
			continue
		}
		dc.parent.RemoveCommand(dc.cmd)
	}
}

// hasSubcommand reports whether child is already attached to parent, so
// re-registration across invocations cannot duplicate it.
func hasSubcommand(parent, child *cobra.Command) bool {
	for _, sub := range parent.Commands() {
		if sub == child {
			return true
		}
	}
	return false
}

// --- Resolution --------------------------------------------------------------

// configForArgs loads the config that shapes this invocation's surface,
// honoring the pre-parse --config (which file to read) and --context (whose
// binding to use) overrides. Cobra has not parsed flags yet at this point — the
// surface filters must shape the tree before dispatch — so the two overrides
// are read straight from the raw args.
//
// Returns nil when there is no usable config, which every caller treats as "no
// constraint": the real command surfaces the config error later, with context.
func configForArgs(args []string) *config.Config {
	// Session-backed invocations resolve against the synthetic config: no
	// user-defined profiles and no context binding, but the per-request
	// environment (RunOptions.Env) still applies.
	if runSession != nil {
		return runSession.syntheticConfig()
	}
	var (
		cfg *config.Config
		err error
	)
	if cfgPath := extractFlagValue(args, "config"); cfgPath != "" {
		cfg, err = config.LoadFrom(cfgPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil
	}
	if ctxOverride := extractContextOverride(args); ctxOverride != "" {
		cfg.CurrentContext = ctxOverride
	}
	return cfg
}

// surfaceConfig is configForArgs with an empty config substituted for "no
// usable config", for the two resolvers whose inputs also come from the
// environment. Without the substitution a machine with no config file at all —
// a fresh install, a container, CI — would silently ignore DTCTL_DEVELOPMENT
// and DTCTL_MIN_STABILITY, since both are read through a Config. The legacy
// DTCTL_EXPERIMENTAL_* variables never had that dependency, so honoring only
// them would also make the deprecated spelling the more reliable one.
func surfaceConfig(args []string) *config.Config {
	if cfg := configForArgs(args); cfg != nil {
		return cfg
	}
	return config.NewConfig()
}

// resolveDevelopmentFeatures returns the development features this invocation
// has opted into, plus whether a disabled development command may explain
// itself.
func resolveDevelopmentFeatures(args []string) (map[string]bool, bool) {
	cfg := surfaceConfig(args)
	return legacyDevelopmentFeatures(cfg.EnabledDevelopmentFeatures()),
		developmentSignposting(cfg)
}

// DevelopmentFeatureEnabled reports whether one development feature is opted
// into, resolving from the ambient config and environment.
//
// It exists for the one caller that cannot wait for the registration stage:
// main must decide whether to dispatch `dtctl serve` before the command
// pipeline runs at all, because a server has to start outside the
// per-invocation lock. Everything inside the pipeline uses the resolved set.
func DevelopmentFeatureEnabled(feature string) bool {
	// No args: main's dispatch requires `serve` to be argv[1], so no --config
	// can precede it, and this is also reachable from a library caller inside a
	// test binary, whose os.Args holds -test.* flags rather than CLI argv.
	enabled, _ := resolveDevelopmentFeatures(nil)
	return stability.Enabled(feature, enabled)
}

// legacyDevelopmentEnvVars maps the per-feature environment variables that
// gated these features before the stability tiers existed onto their feature
// keys. Honored as deprecated aliases so an existing script or deployment does
// not break on upgrade; drop them a release after the tiers ship.
var legacyDevelopmentEnvVars = map[string]string{
	"DTCTL_EXPERIMENTAL_ACCOUNT": accountDevelopmentFeature,
	"DTCTL_EXPERIMENTAL_SERVE":   "serve",
}

// legacyDevelopmentFeatures folds the deprecated per-feature environment
// variables into an enabled set. They can only ever *add* a feature: an unset
// legacy variable is silence, not an explicit off, so it must not override an
// opt-in expressed the current way.
func legacyDevelopmentFeatures(enabled map[string]bool) map[string]bool {
	for envVar, feature := range legacyDevelopmentEnvVars {
		if !truthyEnv(envVar) {
			continue
		}
		if enabled == nil {
			enabled = make(map[string]bool)
		}
		enabled[feature] = true
	}
	return enabled
}

// truthyEnv reports whether an environment variable holds anything other than
// the falsy set. Preserves the matrix the retired ExperimentalEnabled used, so
// a deployment that wrote "1", "true" or "yes" keeps working unchanged.
func truthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// developmentSignposting decides whether a disabled development command names
// itself or stays silent.
//
// The signal is whether this caller has *already demonstrated knowledge of the
// mechanism*: they set DTCTL_DEVELOPMENT (even to an empty or off value), or
// their config carries a development key. For them, silence is the unhelpful
// answer — they are mid-setup and a plain "unknown command" sends them reading
// source. For everyone else the feature genuinely should not exist yet, and
// advertising it would undo the point of gating by registration.
//
// Agent mode is an absolute override. An agent cannot weigh "unfinished, may
// disappear" against its task; naming an opt-in in a machine-readable envelope
// invites it to take the opt-in, and the resulting automation would be built on
// surface with no contract at all.
func developmentSignposting(cfg *config.Config) bool {
	if agentMode {
		return false
	}
	if cfg.DevelopmentEnvSet() {
		return true
	}
	return len(cfg.Development) > 0
}

// resolveStabilityPolicy resolves the active stability floor and its per-target
// exceptions. Both live on the context, and the floor additionally honors the
// DTCTL_MIN_STABILITY environment variable.
//
// The floor is deliberately *not* exposed as a command-line flag. In an
// embedding — the platform service running dtctl for a workflow action — the
// caller controls argv, so a flag would hand the opt-in to exactly the party
// the floor exists to constrain.
func resolveStabilityPolicy(args []string, devEnabled map[string]bool) (stability.Policy, error) {
	granted := stability.DefaultRegistry().EnabledPaths(devEnabled)
	cfg := surfaceConfig(args)
	floor, err := cfg.ResolveMinStability()
	if err != nil {
		return stability.Policy{}, err
	}
	exceptions, err := stability.ParseExceptions(cfg.StabilityExceptions())
	if err != nil {
		return stability.Policy{}, err
	}
	return stability.Policy{
		Floor:       floor,
		Exceptions:  exceptions,
		Development: granted,
	}, nil
}

// --- Stage 3: the stability floor -------------------------------------------

// applyStabilityFloor masks every command and flag whose contract is weaker
// than the policy's floor, unless an exception names it.
//
// Command masking mirrors applyProfile exactly — Hidden, plus a guard RunE with
// arg validation and flag parsing neutralized — so a below-floor command cannot
// leak its arg or flag shape through a generic "accepts 1 arg(s)" error. Flag
// masking hides the flag and rejects its *use* after parsing, which is the only
// point at which "was this flag actually passed" is known.
//
// A policy that cannot block anything is a no-op, preserving today's behavior
// for every caller that has not set a floor.
func applyStabilityFloor(root *cobra.Command, p stability.Policy) {
	if !p.Restricts() {
		return
	}
	floor := p.EffectiveFloor()
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd == root {
			return // never mask the root command itself
		}
		path := commandPathRelative(cmd, root)
		if lvl := stability.Effective(cmd); !p.AllowsCommand(path, lvl) {
			maskBelowFloor(cmd, &StabilityError{Command: path, Level: lvl, Floor: floor})
			return
		}
		blockBelowFloorFlags(cmd, root, path, p, floor)
	})
}

// maskBelowFloor hides a command and replaces its body with a guard returning
// blocked. Non-runnable commands (pure groups) need only the hide: Cobra will
// print their help, and their runnable children are masked in their own right.
func maskBelowFloor(cmd *cobra.Command, blocked *StabilityError) {
	cmd.Hidden = true
	if cmd.RunE == nil && cmd.Run == nil {
		return
	}
	cmd.Args = cobra.ArbitraryArgs
	cmd.DisableFlagParsing = true
	cmd.Run = nil
	cmd.RunE = func(*cobra.Command, []string) error { return blocked }
}

// blockBelowFloorFlags hides each of a command's below-floor local flags and
// wraps the command body with a check that rejects their use.
//
// The flag stays *parseable* on purpose. Removing it would make `--spill` an
// "unknown flag" — indistinguishable from a typo, and a worse answer than
// naming the contract that excludes it. Hiding keeps it out of help and
// completion while the guard keeps it unusable.
func blockBelowFloorFlags(cmd, root *cobra.Command, path string, p stability.Policy, floor stability.Level) {
	if cmd.DisableFlagParsing {
		return // flags are the command's own business; nothing to check
	}
	blocked := make(map[string]stability.Level)
	visitOwnFlags(cmd, func(f *pflag.Flag) {
		lvl := stability.EffectiveFlag(cmd, f.Name)
		if p.AllowsFlag(path, f.Name, lvl) {
			return
		}
		blocked[f.Name] = lvl
		f.Hidden = true
	})
	if len(blocked) == 0 {
		return
	}

	orig := cmd.RunE
	if orig == nil {
		if cmd.Run == nil {
			return
		}
		run := cmd.Run
		orig = func(c *cobra.Command, args []string) error { run(c, args); return nil }
		cmd.Run = nil
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// Sorted so a command with several below-floor flags always reports the
		// same one, making the error reproducible.
		for _, name := range sortedKeys(blocked) {
			if f := c.Flags().Lookup(name); f != nil && f.Changed {
				return &StabilityError{
					Command: path, Flag: name, Level: blocked[name], Floor: floor,
				}
			}
		}
		return orig(c, args)
	}
}

// sortedKeys returns a level map's keys in lexical order.
func sortedKeys(m map[string]stability.Level) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// --- Help badges -------------------------------------------------------------

// applyStabilityBadges prefixes a below-stable command's Short with its tier and
// appends the guarantee to its Long, so the absence of a promise is stated
// rather than left to be inferred from the tier's name. AIP-181 defines
// "experimental" as the *weakest* level of all, so a reader may otherwise take
// the middle tier to mean less than we intend.
//
// Badges never stack: precedence is deprecated > development > experimental.
func applyStabilityBadges(root *cobra.Command) {
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd == root || cmd.Hidden {
			return
		}
		// Flags are badged even on a stable command: a flag may make a weaker
		// promise than the command that carries it, which is how a new idea
		// ships without inventing a new command.
		badgeFlags(cmd)

		badge, note := badgeFor(cmd)
		if badge == "" {
			return
		}
		cmd.Short = badge + " " + cmd.Short
		if note != "" {
			cmd.Long = strings.TrimRight(cmd.Long, "\n") + "\n\n" + badge + " " + note
		}
	})
}

// badgeFor returns the winning badge and its one-line guarantee for a command.
func badgeFor(cmd *cobra.Command) (badge, note string) {
	if d, ok := stability.DeprecationOf(cmd); ok {
		return "[Deprecated]", d.Note() + "."
	}
	lvl := stability.Effective(cmd)
	return stability.Badge(lvl), stability.Guarantee(lvl)
}

// badgeFlags prefixes each below-stable flag's usage string with its tier. A
// flag may be weaker than its command, so the command's badge does not cover
// it: an experimental flag on a stable command is how a new idea ships without
// inventing a new command, and it must say so where it is read.
func badgeFlags(cmd *cobra.Command) {
	visitOwnFlags(cmd, func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if badge := stability.Badge(stability.OfFlag(cmd, f.Name)); badge != "" {
			f.Usage = badge + " " + f.Usage
		}
	})
}

// visitOwnFlags invokes fn for each flag a command declares itself, skipping
// persistent flags inherited from an ancestor (which are badged and checked on
// the ancestor that declared them).
//
// It deliberately avoids cobra's LocalFlags()/InheritedFlags(), which call
// mergePersistentFlags and thereby fold every parent's persistent flags
// permanently into this command's own flag set. These stages walk the entire
// tree on every invocation, so that merge would be a global, irreversible side
// effect of merely rendering help — and the pristine-tree restore resets flags
// per command, so a merged-in --config would start being reset by the wrong
// command.
func visitOwnFlags(cmd *cobra.Command, fn func(*pflag.Flag)) {
	inherited := func(name string) bool {
		for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
			if parent.PersistentFlags().Lookup(name) != nil {
				return true
			}
		}
		return false
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if inherited(f.Name) {
			return
		}
		fn(f)
	})
}

// --- Signposting on the unknown-command path ---------------------------------

// developmentHint turns an unknown-command error into a DevelopmentError when
// the typed command is a disabled development feature *and* this caller has
// earned the explanation. It returns nil otherwise, leaving the ordinary
// suggestion enhancer to answer.
func developmentHint(errStr string, signpost bool) error {
	if !signpost {
		return nil
	}
	m := unknownCmdRe.FindStringSubmatch(errStr)
	if len(m) != 2 {
		return nil
	}
	feature, ok := stability.DefaultRegistry().FeatureForCommand(m[1])
	if !ok {
		return nil
	}
	return &DevelopmentError{Command: m[1], Feature: feature}
}
