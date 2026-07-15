package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/version"
)

func TestSplitPluginArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantWords []string
		wantRest  []string
	}{
		{
			name:      "bare plugin command",
			args:      []string{"foo", "bar", "baz"},
			wantWords: []string{"foo", "bar", "baz"},
		},
		{
			name:      "leading value-taking flag is consumed by dtctl",
			args:      []string{"--context", "prod", "foo", "bar"},
			wantWords: []string{"foo", "bar"},
		},
		{
			name:      "leading inline flag",
			args:      []string{"--context=prod", "foo"},
			wantWords: []string{"foo"},
		},
		{
			name:      "words stop at the first trailing flag",
			args:      []string{"foo", "bar", "--baz", "qux"},
			wantWords: []string{"foo", "bar"},
			wantRest:  []string{"--baz", "qux"},
		},
		{
			name:      "leading boolean short flags",
			args:      []string{"-v", "foo"},
			wantWords: []string{"foo"},
		},
		{
			name:      "value-taking short flag consumes its value",
			args:      []string{"-o", "json", "foo"},
			wantWords: []string{"foo"},
		},
		{
			name:     "flags only, no command words",
			args:     []string{"--plain"},
			wantRest: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words, rest := splitPluginArgs(tt.args)
			if strings.Join(words, " ") != strings.Join(tt.wantWords, " ") {
				t.Errorf("words = %v, want %v", words, tt.wantWords)
			}
			if strings.Join(rest, " ") != strings.Join(tt.wantRest, " ") {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestIsBuiltinCommandName(t *testing.T) {
	if !isBuiltinCommandName("get") {
		t.Error("get must be recognized as built-in")
	}
	if isBuiltinCommandName("no-such-command") {
		t.Error("unknown names must not be built-in")
	}
}

func TestPluginEnv(t *testing.T) {
	t.Setenv("DTCTL_CONTEXT", "inherited")

	env := pluginEnv([]string{"--context", "prod", "--plain", "myplug"})

	get := func(key string) (string, int) {
		val, count := "", 0
		for _, e := range env {
			if strings.HasPrefix(e, key+"=") {
				val = strings.TrimPrefix(e, key+"=")
				count++
			}
		}
		return val, count
	}

	if v, n := get("DTCTL_CONTEXT"); v != "prod" || n != 1 {
		t.Errorf("DTCTL_CONTEXT = %q (%d entries), want prod exactly once (flag overrides inherited env)", v, n)
	}
	if v, _ := get("DTCTL_CALLER_VERSION"); v != version.Version {
		t.Errorf("DTCTL_CALLER_VERSION = %q, want %q", v, version.Version)
	}
	if v, _ := get("DTCTL_PLAIN"); v != "1" {
		t.Errorf("DTCTL_PLAIN = %q, want 1", v)
	}
	if v, _ := get("DTCTL_CONFIG"); v == "" {
		t.Error("DTCTL_CONFIG must always be set")
	}
	// No secrets: the environment must not gain token material beyond what
	// was already inherited.
	for _, e := range env {
		if strings.HasPrefix(e, "DTCTL_TOKEN") {
			t.Errorf("unexpected token material in plugin env: %s", e)
		}
	}
}

func TestPluginEnv_AgentImpliesPlain(t *testing.T) {
	env := pluginEnv([]string{"-A", "myplug"})
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "DTCTL_AGENT=1") {
		t.Error("DTCTL_AGENT=1 missing for -A")
	}
	if !strings.Contains(joined, "DTCTL_PLAIN=1") {
		t.Error("agent mode must imply DTCTL_PLAIN=1")
	}
}

func TestOverrideEnv_ReplacesExisting(t *testing.T) {
	env := overrideEnv([]string{"A=1", "B=2", "A=3"}, "A", "9")
	joined := strings.Join(env, ";")
	if strings.Count(joined, "A=") != 1 || !strings.Contains(joined, "A=9") {
		t.Errorf("overrideEnv = %v, want single A=9", env)
	}
}

func TestTryPluginDispatch_BuiltinAndUnknownAreNotDispatched(t *testing.T) {
	// Point PATH at an empty dir: no plugins can be found, and built-in names
	// must be rejected before lookup anyway.
	t.Setenv("PATH", t.TempDir())
	if _, handled := tryPluginDispatch([]string{"get", "nosuchresource"}); handled {
		t.Error("built-in command names must never dispatch to plugins")
	}
	if _, handled := tryPluginDispatch([]string{"definitely-not-a-plugin"}); handled {
		t.Error("unknown names without a matching binary must not dispatch")
	}
}
