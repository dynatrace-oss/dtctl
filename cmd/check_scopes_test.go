package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/commands"
)

// withScopeState overrides grantedScopesFunc and the relevant global flags for a
// test, restoring them afterwards.
func withScopeState(t *testing.T, check, agent bool, format string, granted []string, known bool) {
	t.Helper()
	origCheck, origAgent, origFormat, origFunc := checkScopes, agentMode, outputFormat, grantedScopesFunc
	checkScopes, agentMode, outputFormat = check, agent, format
	grantedScopesFunc = func() ([]string, bool) { return granted, known }
	t.Cleanup(func() {
		checkScopes, agentMode, outputFormat, grantedScopesFunc = origCheck, origAgent, origFormat, origFunc
	})
}

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureScopeStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestSubtractScopes(t *testing.T) {
	got := subtractScopes(
		[]string{"a", "b", "c"},
		[]string{"b"},
	)
	require.Equal(t, []string{"a", "c"}, got)

	require.Empty(t, subtractScopes([]string{"a"}, []string{"a", "x"}))
	require.Empty(t, subtractScopes(nil, []string{"a"}))
}

// TestMutatingCommandsUseRunE guards the auto-preflight's coverage: it wraps
// only commands with a non-nil RunE (see installScopePreflight). A mutating
// command implemented with Run instead of RunE would silently bypass the
// scope preflight, so fail the build if one ever appears.
func TestMutatingCommandsUseRunE(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		// Only runnable leaves matter.
		if c.Run == nil && c.RunE == nil {
			return
		}
		verb, _ := verbResource(c)
		if _, mutating := commands.MutatingVerbs[verb]; !mutating {
			return
		}
		require.Nil(t, c.Run,
			"mutating command %q uses Run; it must use RunE so installScopePreflight covers it", c.CommandPath())
		require.NotNil(t, c.RunE,
			"mutating command %q has no RunE; the scope preflight cannot wrap it", c.CommandPath())
	}
	walk(rootCmd)
}

func TestVerbResource(t *testing.T) {
	cases := []struct {
		args     []string
		wantVerb string
		wantRes  string
	}{
		{[]string{"delete", "workflow"}, "delete", "workflow"},
		{[]string{"get", "workflows"}, "get", "workflows"},
		{[]string{"query"}, "query", ""},
		{[]string{"wait", "query"}, "wait", "query"},
	}
	for _, c := range cases {
		cmd, _, err := rootCmd.Find(c.args)
		require.NoError(t, err, "find %v", c.args)
		verb, res := verbResource(cmd)
		require.Equal(t, c.wantVerb, verb, "verb for %v", c.args)
		require.Equal(t, c.wantRes, res, "resource for %v", c.args)
	}
}

func TestRequiredScopesFor(t *testing.T) {
	got, ok := requiredScopesFor("delete", "workflow")
	require.True(t, ok)
	require.Equal(t, []string{"automation:workflows:write"}, got)

	got, ok = requiredScopesFor("get", "workflows")
	require.True(t, ok)
	require.Equal(t, []string{"automation:workflows:read"}, got)

	// DQL verbs carry flat scopes at the verb level.
	got, ok = requiredScopesFor("query", "")
	require.True(t, ok)
	require.Contains(t, got, "storage:logs:read")

	got, ok = requiredScopesFor("wait", "query")
	require.True(t, ok)
	require.Contains(t, got, "storage:logs:read")

	// Local commands have no scope requirement.
	_, ok = requiredScopesFor("ctx", "set")
	require.False(t, ok)
}

func TestComputeScopeVerdict(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		withScopeState(t, true, false, "json", []string{"automation:workflows:write", "x"}, true)
		r := computeScopeVerdict("delete", "workflow", []string{"automation:workflows:write"}, true)
		require.Equal(t, scopeStatusOK, r.Status)
		require.Empty(t, r.MissingScopes)
	})

	t.Run("insufficient", func(t *testing.T) {
		withScopeState(t, true, false, "json", []string{"automation:workflows:read"}, true)
		r := computeScopeVerdict("delete", "workflow", []string{"automation:workflows:write"}, true)
		require.Equal(t, scopeStatusInsufficient, r.Status)
		require.Equal(t, []string{"automation:workflows:write"}, r.MissingScopes)
		require.NotEmpty(t, r.Suggestions)
	})

	t.Run("unknown", func(t *testing.T) {
		withScopeState(t, true, false, "json", nil, false)
		r := computeScopeVerdict("delete", "workflow", []string{"automation:workflows:write"}, true)
		require.Equal(t, scopeStatusUnknown, r.Status)
		require.Empty(t, r.GrantedScopes)
		require.NotEmpty(t, r.Suggestions)
	})

	t.Run("no scopes required", func(t *testing.T) {
		withScopeState(t, true, false, "json", nil, false)
		r := computeScopeVerdict("ctx", "set", nil, false)
		require.Equal(t, scopeStatusOK, r.Status)
		require.Empty(t, r.RequiredScopes)
	})
}

func TestScopePreflight_CheckScopes_OKSkipsAndPrints(t *testing.T) {
	withScopeState(t, true, false, "json", []string{"automation:workflows:write"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	var skip bool
	var preErr error
	out := captureScopeStdout(t, func() { skip, preErr = scopePreflight(cmd, nil) })

	require.True(t, skip, "command body must be skipped")
	require.NoError(t, preErr)

	var res ScopeCheckResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Equal(t, scopeStatusOK, res.Status)
	require.Equal(t, "delete", res.Verb)
	require.Equal(t, "workflow", res.Resource)
}

func TestScopePreflight_CheckScopes_MissingExitsNonZero(t *testing.T) {
	withScopeState(t, true, false, "json", []string{"automation:workflows:read"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	var skip bool
	var preErr error
	_ = captureScopeStdout(t, func() { skip, preErr = scopePreflight(cmd, nil) })

	require.True(t, skip)
	var silent *silentExitError
	require.ErrorAs(t, preErr, &silent)
	require.Equal(t, client.ExitPermissionError, silent.code)
}

func TestScopePreflight_AgentBlocksMutatingWhenMissing(t *testing.T) {
	withScopeState(t, false, true, "json", []string{"automation:workflows:read"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	var scopeErr *ScopeError
	require.ErrorAs(t, preErr, &scopeErr)
	require.Equal(t, []string{"automation:workflows:write"}, scopeErr.Missing)

	// And it maps to the insufficient_scope envelope + ExitPermissionError.
	detail := errorToDetail(preErr)
	require.Equal(t, "insufficient_scope", detail.Code)
	require.Equal(t, []string{"automation:workflows:write"}, detail.MissingScopes)
	require.Equal(t, client.ExitPermissionError, exitCodeForError(preErr))
}

func TestScopePreflight_AgentProceedsWhenScopePresent(t *testing.T) {
	withScopeState(t, false, true, "json", []string{"automation:workflows:write"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	require.NoError(t, preErr, "command should proceed when scope is present")
}

func TestScopePreflight_AgentProceedsWhenUnknown(t *testing.T) {
	// Opaque token: granted scopes not introspectable -> do not block.
	withScopeState(t, false, true, "json", nil, false)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	require.NoError(t, preErr)
}

func TestScopePreflight_AgentIgnoresReadVerbs(t *testing.T) {
	// get is non-mutating: agent auto-preflight must not block it even if a
	// (hypothetical) scope were missing.
	withScopeState(t, false, true, "json", []string{}, true)
	cmd, _, err := rootCmd.Find([]string{"get", "workflows"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	require.NoError(t, preErr)
}

func TestScopePreflight_CheckScopes_AgentWrapsOKInEnvelope(t *testing.T) {
	// --check-scopes in agent mode must emit the standard {ok,result,context}
	// envelope, not a bare verdict object.
	withScopeState(t, true, true, "json", []string{"automation:workflows:write"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	var skip bool
	var preErr error
	out := captureScopeStdout(t, func() { skip, preErr = scopePreflight(cmd, nil) })
	require.True(t, skip)
	require.NoError(t, preErr)

	var resp struct {
		OK     bool             `json:"ok"`
		Result ScopeCheckResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	require.True(t, resp.OK)
	require.Equal(t, scopeStatusOK, resp.Result.Status)
	require.Equal(t, "delete", resp.Result.Verb)
}

func TestScopePreflight_CheckScopes_AgentMissingUsesErrorEnvelope(t *testing.T) {
	// --check-scopes in agent mode with a missing scope must route through the
	// ScopeError path: insufficient_scope error envelope + ExitPermissionError,
	// identical to the auto-preflight contract.
	withScopeState(t, true, true, "json", []string{"automation:workflows:read"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	var scopeErr *ScopeError
	require.ErrorAs(t, preErr, &scopeErr)
	require.Equal(t, []string{"automation:workflows:write"}, scopeErr.Missing)

	detail := errorToDetail(preErr)
	require.Equal(t, "insufficient_scope", detail.Code)
	require.Equal(t, client.ExitPermissionError, exitCodeForError(preErr))
}

func TestScopePreflight_DisabledIsNoOp(t *testing.T) {
	withScopeState(t, false, false, "json", []string{"automation:workflows:read"}, true)
	cmd, _, err := rootCmd.Find([]string{"delete", "workflow"})
	require.NoError(t, err)

	skip, preErr := scopePreflight(cmd, nil)
	require.False(t, skip)
	require.NoError(t, preErr)
}
