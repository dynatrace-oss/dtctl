package cmd

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// withAgentMode sets agentMode for one test and restores it.
func withAgentMode(t *testing.T, on bool) {
	t.Helper()
	orig := agentMode
	agentMode = on
	t.Cleanup(func() { agentMode = orig })
}

func TestDryRunReport_HumanOutputIsPlainLines(t *testing.T) {
	withAgentMode(t, false)
	cmd, _, err := rootCmd.Find([]string{"create", "bucket"})
	require.NoError(t, err)

	out := captureScopeStdout(t, func() {
		require.NoError(t, newDryRunReport(cmd).
			Linef("Dry run: would create bucket").
			Field("Name", "%s", "my_bucket").
			Field("Retention", "%d days", 35).
			Payload([]byte(`{"bucketName":"my_bucket"}`)).
			Print())
	})

	// One line per entry, in order, and nothing else: no JSON, and the payload
	// stays out of the human rendering unless a line asked for it.
	require.Equal(t, "Dry run: would create bucket\nName: my_bucket\nRetention: 35 days\n", out)
}

func TestDryRunReport_AgentOutputIsAnEnvelope(t *testing.T) {
	withAgentMode(t, true)
	cmd, _, err := rootCmd.Find([]string{"create", "bucket"})
	require.NoError(t, err)

	out := captureScopeStdout(t, func() {
		require.NoError(t, newDryRunReport(cmd).
			Linef("Dry run: would create bucket").
			Field("Display Name", "%s", "My Bucket").
			Detail("table", "%s", "logs").
			Payload([]byte(`{"bucketName":"my_bucket"}`)).
			Print())
	})

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			DryRun   bool              `json:"dry_run"`
			Verb     string            `json:"verb"`
			Resource string            `json:"resource"`
			Details  map[string]string `json:"details"`
			Payload  json.RawMessage   `json:"payload"`
			Message  string            `json:"message"`
		} `json:"result"`
		Context struct {
			Verb     string `json:"verb"`
			Resource string `json:"resource"`
		} `json:"context"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp), "agent output must be a single JSON document")

	require.True(t, resp.OK, "a dry run succeeded; it is not an error")
	require.True(t, resp.Result.DryRun)
	require.Equal(t, "create", resp.Result.Verb)
	require.Equal(t, "bucket", resp.Result.Resource)
	require.Equal(t, "create", resp.Context.Verb)
	require.Equal(t, "bucket", resp.Context.Resource)

	// Field labels become snake_case keys; Detail contributes without a line.
	require.Equal(t, map[string]string{"display_name": "My Bucket", "table": "logs"}, resp.Result.Details)

	// The payload is JSON, not a string, so it can be diffed against the real request.
	require.JSONEq(t, `{"bucketName":"my_bucket"}`, string(resp.Result.Payload))

	// The human text is preserved verbatim, so nothing the prose says is lost.
	require.Equal(t, "Dry run: would create bucket\nDisplay Name: My Bucket", resp.Result.Message)

	// And it goes out compact, like every other envelope on a non-terminal
	// stdout. Print() routes through output.EncodeEnvelope for exactly this: a
	// local encoder with SetIndent would have spent a third more tokens on
	// whitespace for the audience the envelope exists to serve.
	require.NotContains(t, out, "\n  ", "a piped envelope must be compact, not indented")
	require.Equal(t, 1, strings.Count(out, "\n"), "compact JSON is one line plus its terminator")
}

func TestDryRunReport_InvalidPayloadIsDropped(t *testing.T) {
	withAgentMode(t, true)
	cmd, _, err := rootCmd.Find([]string{"create", "workflow"})
	require.NoError(t, err)

	out := captureScopeStdout(t, func() {
		require.NoError(t, newDryRunReport(cmd).
			Linef("Dry run: would create workflow").
			Payload([]byte("not json")).
			Print())
	})

	// An unparseable payload must not make the envelope unparseable.
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	result, ok := resp["result"].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, result, "payload")
}

func TestDetailKey(t *testing.T) {
	for label, want := range map[string]string{
		"Name":                     "name",
		"Display Name":             "display_name",
		"Lookup Field":             "lookup_field",
		"Config ID":                "config_id",
		"File Size":                "file_size",
		"Referenced by context(s)": "referenced_by_contexts",
	} {
		require.Equal(t, want, detailKey(label), "label %q", label)
	}
}

// dryRunStdoutAllowlist records files whose dry-run branch may still write prose
// straight to stdout, with the reason. A file not listed here must route its
// dry-run output through dryRunReport, so that agent mode receives an envelope
// on the stream it parses as JSON instead of a sentence.
var dryRunStdoutAllowlist = map[string]string{
	// exec api renders an aligned key-value preview through output.DescribeKV
	// (bold labels, fixed column width), which dryRunReport's line model does not
	// express. It is also the one command that must never become an integration
	// target (AGENTS.md, "Generic API Access"), so it is tracked in issue #514
	// rather than reshaped here.
	"exec_api.go": "aligned DescribeKV preview; tracked in #514",
}

// TestDryRunBranchesDoNotPrintToStdout guards the invariant this replaced: a
// dry-run branch that prints with fmt.Print* puts prose on stdout, which in
// agent mode is the stream the caller decodes as JSON — so the dry run became
// the one outcome an agent could not read, while errors from the same command
// arrived correctly enveloped.
//
// The scan is lexical: it finds `if dryRun { ... }` blocks and the print calls
// written inside them. A branch that calls a helper which prints (exec api's
// printAPIDryRun) is not reachable this way, so this catches the regression
// shape that produced the original 15 sites, not every conceivable one.
func TestDryRunBranchesDoNotPrintToStdout(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if reason, allowed := dryRunStdoutAllowlist[name]; allowed {
			require.NotEmpty(t, reason, "%s: an allowlist entry must carry a reason", name)
			continue
		}

		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, filepath.Clean(name), nil, 0)
		require.NoError(t, perr)

		ast.Inspect(file, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok || !mentionsDryRun(ifStmt.Cond) {
				return true
			}
			ast.Inspect(ifStmt.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				if callee := stdoutPrintCallee(call); callee != "" {
					t.Errorf("%s:%d: dry-run branch calls %s — build a dryRunReport and return its Print() so agent mode gets an envelope",
						name, fset.Position(call.Pos()).Line, callee)
				}
				return true
			})
			return true
		})
	}
}

// mentionsDryRun reports whether an expression reads the dryRun flag.
func mentionsDryRun(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == "dryRun" {
			found = true
		}
		return !found
	})
	return found
}

// stdoutPrintCallee names the call if it writes to stdout, and returns "" if it
// does not. Writes to stderr (output.PrintInfo, output.PrintWarning) are fine:
// stdout is the stream an agent decodes, and diagnostics have always gone to
// stderr.
func stdoutPrintCallee(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	qualified := pkg.Name + "." + sel.Sel.Name
	switch qualified {
	case "fmt.Print", "fmt.Printf", "fmt.Println", "output.DescribeKV":
		return qualified
	case "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln":
		if len(call.Args) > 0 && isOsStdout(call.Args[0]) {
			return qualified
		}
	}
	return ""
}

// isOsStdout reports whether an expression is os.Stdout.
func isOsStdout(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Stdout" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os"
}
