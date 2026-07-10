package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// ProfileError is returned when a command that is masked by the active profile
// is invoked. Because the command is hidden but still parseable by Cobra, the
// guard produces a specific, signposted error rather than a generic "unknown
// command". It is a surface (topic) error, distinct from a safety (permission)
// error — see docs/dev/COMMAND_PROFILES_DESIGN.md.
type ProfileError struct {
	// Command is the space-joined command path relative to root (e.g. "auth login").
	Command string
	// Profile is the name of the active profile that masks the command.
	Profile string
}

func (e *ProfileError) Error() string {
	return fmt.Sprintf("command %q is not available in profile %q\n\n"+
		"  This profile exposes a reduced command set. Run 'dtctl commands' to see\n"+
		"  what is available, or unset %s to use the full CLI.",
		e.Command, e.Profile, config.ProfileEnvVar)
}

// applyProfile masks every command outside the profile's allowlist. Masking
// means (1) setting Hidden — which removes the command from --help, the
// `dtctl commands` catalog, and shell completion in one shot, since all three
// already honor Hidden — and (2) wrapping any runnable command with a guard that
// returns a ProfileError, satisfying hard enforcement (a hidden command is still
// runnable by name in Cobra).
//
// A nil profile is the full command tree (no-op), preserving today's behavior.
// The filter runs once, after the whole command tree is registered and before
// Cobra dispatches — see execute() in root.go.
func applyProfile(root *cobra.Command, p *config.Profile) {
	if p == nil {
		return
	}
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd == root {
			return // never mask the root command itself
		}
		if p.Allows(commandPathRelative(cmd, root)) {
			return
		}
		cmd.Hidden = true
		if cmd.RunE != nil || cmd.Run != nil {
			cmd.RunE = blockedRunE(commandPathRelative(cmd, root), p.Name)
			cmd.Run = nil
		}
	})
}

// blockedRunE returns a RunE that reports the command as unavailable under the
// active profile. Cobra still parses the command and its args before invoking
// this, which is why masking wraps RunE instead of removing the command — it
// lets us return a specific "not available in profile X" message.
func blockedRunE(path, profileName string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		return &ProfileError{Command: path, Profile: profileName}
	}
}

// completeProfileNames provides shell completion for a --profile flag value:
// the built-in presets plus any user-defined profiles, always including "full".
func completeProfileNames(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	names := append([]string{config.ProfileFull}, config.BuiltinProfileNames()...)
	if cfg, err := config.Load(); err == nil {
		for name := range cfg.Profiles {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// walkCommands invokes fn for cmd and every command in its subtree.
func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walkCommands(sub, fn)
	}
}

// commandPathRelative returns a command's path relative to root, space-joined
// (e.g. "get workflows"). This matches the vocabulary the profile allowlist and
// the commands catalog use.
func commandPathRelative(cmd, root *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), root.Name()))
}

// extractFlagValue scans raw args for a value-taking flag written as either
// "--name value" or "--name=value" and returns its value. Cobra has not parsed
// flags yet when the profile is resolved (the filter must shape the tree before
// dispatch), so the --config and --context overrides that influence profile
// selection are read directly from the args here. Returns "" when absent.
func extractFlagValue(args []string, name string) string {
	long := "--" + name
	eq := long + "="
	for i := range args {
		arg := args[i]
		if v, ok := strings.CutPrefix(arg, eq); ok {
			return v
		}
		if arg == long && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// extractContextOverride returns the value of a --context flag in raw args.
func extractContextOverride(args []string) string {
	return extractFlagValue(args, "context")
}

// resolveActiveProfile loads config and resolves the active profile for the
// given (pre-parse) args, honoring --config (which config file to read) and
// --context (which context's binding to use) overrides. It returns (nil, nil)
// for the full command tree, and a non-nil error only when a referenced profile
// name does not exist.
func resolveActiveProfile(args []string) (*config.Profile, error) {
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
		// No usable config → full command tree. The real command will surface
		// any config error later with proper context.
		return nil, nil
	}
	if ctxOverride := extractContextOverride(args); ctxOverride != "" {
		cfg.CurrentContext = ctxOverride
	}
	return cfg.ResolveProfile()
}
