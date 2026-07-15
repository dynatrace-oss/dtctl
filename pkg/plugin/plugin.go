// Package plugin implements dtctl's kubectl-style exec plugin convention:
// an executable named dtctl-<name> on PATH extends dtctl with the
// `dtctl <name>` command. This package only resolves and discovers plugins;
// dispatch (the exec) lives in cmd, and built-in commands always win because
// dispatch is only attempted for names cobra does not know.
//
// See docs/dev/PLUGIN_CONVENTIONS.md for the author-facing contract.
package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Prefix is the file-name prefix that marks an executable as a dtctl plugin.
const Prefix = "dtctl-"

// Invocation is a resolved plugin call.
type Invocation struct {
	// CommandWords are the argv words consumed by the plugin name,
	// e.g. ["foo", "bar"] for dtctl-foo-bar.
	CommandWords []string
	// Path is the resolved binary path.
	Path string
	// Args are the remaining arguments to pass to the plugin.
	Args []string
}

// Resolve maps leading command words to the longest matching dtctl-<a-b-c>
// binary on PATH (kubectl semantics): words [foo bar baz] tries
// dtctl-foo-bar-baz, then dtctl-foo-bar (arg baz), then dtctl-foo
// (args bar baz). rest is appended after the unconsumed words. lookPath is
// exec.LookPath in production; injected for tests.
//
// A candidate name containing a path separator is never looked up: a
// separator would turn the name into a relative path, and exec.LookPath
// resolves those against the working directory without the ErrDot guard
// that otherwise keeps dispatch off cwd-relative binaries. Shorter
// separator-free prefixes still resolve, so a path may appear as a plugin
// argument (`dtctl foo some/file.yaml`), just never in the plugin name.
func Resolve(words, rest []string, lookPath func(string) (string, error)) (*Invocation, bool) {
	for n := len(words); n > 0; n-- {
		name := Prefix + strings.Join(words[:n], "-")
		if strings.ContainsAny(name, `/\`) {
			continue
		}
		path, err := lookPath(name)
		if err != nil {
			continue
		}
		args := append(append([]string{}, words[n:]...), rest...)
		return &Invocation{
			CommandWords: words[:n],
			Path:         path,
			Args:         args,
		}, true
	}
	return nil, false
}

// Plugin is a plugin binary discovered on PATH.
type Plugin struct {
	Name    string `json:"name" yaml:"name" table:"NAME"`
	Binary  string `json:"binary" yaml:"binary" table:"BINARY"`
	Path    string `json:"path" yaml:"path" table:"PATH"`
	Warning string `json:"warning,omitempty" yaml:"warning,omitempty" table:"WARNING"`
}

// Discover scans the directories of pathEnv (a PATH-formatted list) for
// dtctl-* executables. The first occurrence of a binary name on PATH wins,
// matching exec semantics. builtins is the set of built-in dtctl command
// names (including aliases); a plugin whose first command word collides gets
// a Warning, because built-ins always win at dispatch time.
func Discover(pathEnv string, builtins map[string]bool) []Plugin {
	seen := make(map[string]bool)
	var plugins []Plugin
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			// An empty PATH entry means the working directory, but dispatch
			// refuses cwd-relative binaries (exec.LookPath's ErrDot guard) —
			// don't list plugins that would not run.
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), Prefix) {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if !isExecutable(full) {
				continue
			}
			if seen[e.Name()] {
				continue // an earlier PATH entry shadows this one
			}
			seen[e.Name()] = true

			words := strings.Split(strings.TrimPrefix(stripExecSuffix(e.Name()), Prefix), "-")
			p := Plugin{
				Name:   strings.Join(words, " "),
				Binary: e.Name(),
				Path:   full,
			}
			if builtins[words[0]] {
				p.Warning = "shadowed by the built-in '" + words[0] + "' command (built-ins always win)"
			}
			plugins = append(plugins, p)
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins
}

// isExecutable reports whether the path is a plausible plugin executable:
// the executable bit on Unix, a recognized extension on Windows. os.Stat
// (not DirEntry.Info) so symlinked plugins — which exec.LookPath follows and
// dispatch runs — are discovered too.
func isExecutable(path string) bool {
	if runtime.GOOS == "windows" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".bat", ".cmd", ".com":
			return true
		}
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

// stripExecSuffix removes the Windows executable extension so plugin names
// stay platform-neutral.
func stripExecSuffix(name string) string {
	if runtime.GOOS != "windows" {
		return name
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".bat", ".cmd", ".com":
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}
