package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/aidetect"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/plugin"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// tryPluginDispatch gives an unknown top-level command one shot at plugin
// dispatch (docs/dev/PLUGIN_CONVENTIONS.md): `dtctl foo bar` execs
// the longest dash-joined dtctl-<name> match on PATH (dtctl-foo-bar, then
// dtctl-foo). Built-in commands always win — this only runs from the
// unknown-command error path, and the first word is re-checked against the
// built-in names so a plugin can never capture a built-in's subcommand
// typos (e.g. `dtctl get nosuch` must not exec a dtctl-get binary).
//
// Returns (exitCode, true) when a plugin handled the invocation. On Unix a
// successful dispatch never returns (process replacement); the code path
// returning here means Windows, or an exec failure.
func tryPluginDispatch(args []string) (int, bool) {
	words, rest := splitPluginArgs(args)
	if len(words) == 0 || isBuiltinCommandName(words[0]) {
		return 0, false
	}
	inv, ok := plugin.Resolve(words, rest, exec.LookPath)
	if !ok {
		return 0, false
	}
	code, err := execForward(inv.Path, inv.Args, pluginEnv(args))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1, true
	}
	return code, true
}

// splitPluginArgs separates dtctl's own leading flags from the command words
// that name a plugin, and everything after them. Leading flags are consumed
// by dtctl (they are reflected into the plugin env contract, not passed
// through); the contiguous run of non-flag tokens that follows are the
// command words; the remainder is passed to the plugin verbatim.
//
//	dtctl --context prod foo bar --baz  →  words [foo bar], rest [--baz]
func splitPluginArgs(args []string) (words, rest []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" || a == "--" {
			break
		}
		i++
		if strings.HasPrefix(a, "--") {
			flagName := a
			if eq := strings.Index(a, "="); eq >= 0 {
				flagName = a[:eq]
			}
			if flagsTakingValues[flagName] && !strings.Contains(a, "=") &&
				i < len(args) && !strings.HasPrefix(args[i], "-") {
				i++ // skip the flag's value token
			}
		} else if shortFlagsTakingValues[a] &&
			i < len(args) && !strings.HasPrefix(args[i], "-") {
			i++ // skip the short flag's value token
		}
	}
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		words = append(words, args[i])
		i++
	}
	return words, args[i:]
}

// isBuiltinCommandName reports whether name is a built-in dtctl command or
// one of its aliases. Plugins can never shadow these.
func isBuiltinCommandName(name string) bool {
	for _, c := range rootCmd.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	return false
}

// builtinCommandNames returns the set of built-in command names and aliases,
// for plugin-discovery shadow warnings.
func builtinCommandNames() map[string]bool {
	names := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
		for _, a := range c.Aliases {
			names[a] = true
		}
	}
	return names
}

// pluginEnv builds the environment for a plugin exec: the inherited
// environment plus the dtctl→plugin contract variables. No tokens are ever
// passed — plugins resolve credentials themselves through the sdk (the same
// config file and keyring service).
//
// Because dispatch happens on cobra's unknown-command error path, dtctl's
// persistent flags were never parsed — the contract values are derived from
// the raw args (and auto-detection), mirroring initConfig.
func pluginEnv(args []string) []string {
	env := os.Environ()
	if ctx := rawFlagValue(args, "--context"); ctx != "" {
		env = overrideEnv(env, "DTCTL_CONTEXT", ctx)
	}
	env = overrideEnv(env, "DTCTL_CONFIG", effectiveConfigPath(rawFlagValue(args, "--config")))
	agent := hasRawFlag(args, "--agent") || hasShortFlagLetter(args, 'A') ||
		(aidetect.Detect().Detected && !hasRawFlag(args, "--no-agent"))
	if agent {
		env = overrideEnv(env, "DTCTL_AGENT", "1")
	}
	if agent || hasRawFlag(args, "--plain") {
		env = overrideEnv(env, "DTCTL_PLAIN", "1")
	}
	env = overrideEnv(env, "DTCTL_CALLER_VERSION", version.Version)
	return env
}

// effectiveConfigPath resolves the config file path in effect for the plugin
// contract: the explicit --config value, else a discovered local .dtctl.yaml,
// else the default global path.
func effectiveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if local := config.FindLocalConfig(); local != "" {
		return local
	}
	return config.DefaultConfigPath()
}

// overrideEnv returns env with key set to value, replacing any existing
// entries (duplicate keys in an exec environment are resolved
// inconsistently across platforms, so never append blindly).
func overrideEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+value)
}

// rawFlagValue extracts a long flag's value from unparsed args, supporting
// both "--flag value" and "--flag=value".
func rawFlagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, flag+"=") {
			return a[len(flag)+1:]
		}
	}
	return ""
}

// hasShortFlagLetter reports whether the unparsed args carry the given
// letter in any short-flag cluster (e.g. 'A' in "-A" or "-Av").
func hasShortFlagLetter(args []string, letter rune) bool {
	for _, a := range args {
		if len(a) < 2 || a[0] != '-' || a[1] == '-' {
			continue
		}
		if strings.ContainsRune(a[1:], letter) {
			return true
		}
	}
	return false
}

// hasRawFlag reports whether the unparsed args carry the given long flag,
// either as "--flag" or "--flag=value".
func hasRawFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}
