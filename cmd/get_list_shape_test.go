package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
)

// newListShapeEnv serves three buckets, enough to observe --limit cutting a list.
func newListShapeEnv(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform/storage/management/v1/bucket-definitions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"buckets":[` +
			`{"bucketName":"b_one","table":"logs","status":"active","retentionDays":35,"version":1},` +
			`{"bucketName":"b_two","table":"spans","status":"active","retentionDays":10,"version":2},` +
			`{"bucketName":"b_three","table":"events","status":"active","retentionDays":7,"version":3}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runListShape(t *testing.T, srv *httptest.Server, argv ...string) (int, string) {
	t.Helper()
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")
	// Run resets the tree before an invocation, not after; tests that call
	// RunE directly afterwards must not inherit this run's -o/--fields/--agent.
	t.Cleanup(restorePristineTree)
	return captureRun(t, argv, RunOptions{
		// --limit/--fields are experimental; a session defaults to a stable floor.
		Session: &Session{EnvironmentURL: srv.URL, Token: "t", MinStability: "experimental"},
		Stderr:  stderrLog{t},
	})
}

func TestGetListShape_LimitTruncatesList(t *testing.T) {
	srv := newListShapeEnv(t)

	code, out := runListShape(t, srv, "get", "buckets", "--limit", "2", "-o", "json")
	require.Zero(t, code, out)
	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &items), out)
	require.Len(t, items, 2)
	require.Equal(t, "b_one", items[0]["bucketName"])
	require.Contains(t, items[0], "table", "--limit alone must not reshape items")

	// Flag values are reset between invocations: the next run lists everything.
	code, out = runListShape(t, srv, "get", "buckets", "-o", "json")
	require.Zero(t, code, out)
	require.NoError(t, json.Unmarshal([]byte(out), &items), out)
	require.Len(t, items, 3)
}

func TestGetListShape_FieldsCSVColumnOrder(t *testing.T) {
	srv := newListShapeEnv(t)

	code, out := runListShape(t, srv, "get", "buckets", "--fields", "retentionDays,bucketName", "-o", "csv")
	require.Zero(t, code, out)
	require.Equal(t, "retentionDays,bucketName\n35,b_one\n10,b_two\n7,b_three\n", out)
}

func TestGetListShape_AgentEnvelopeReportsTruncation(t *testing.T) {
	srv := newListShapeEnv(t)

	code, out := runListShape(t, srv, "-A", "get", "buckets", "--limit", "1", "--fields", "bucketName")
	require.Zero(t, code, out)
	var resp output.Response
	require.NoError(t, json.Unmarshal([]byte(out), &resp), out)
	require.True(t, resp.OK)
	require.Equal(t, []any{map[string]any{"bucketName": "b_one"}}, resp.Result)
	require.NotNil(t, resp.Context)
	require.NotNil(t, resp.Context.Total)
	require.Equal(t, 3, *resp.Context.Total)
	require.True(t, resp.Context.HasMore)
	require.True(t, strings.Contains(strings.Join(resp.Context.Suggestions, " "), "--limit"), resp.Context.Suggestions)
}

func TestGetListShape_FieldsRejectedForChart(t *testing.T) {
	srv := newListShapeEnv(t)

	code, _ := runListShape(t, srv, "get", "buckets", "--fields", "bucketName", "-o", "chart")
	require.NotZero(t, code)
}

// A list verb that already had a server-side --limit keeps its own flag; the
// get-wide flag only reaches subcommands that lacked one.
func TestGetListShape_LocalLimitFlagsWin(t *testing.T) {
	require.Equal(t, "int64", getWorkflowsCmd.Flags().Lookup("limit").Value.Type())
	require.Equal(t, "int64", getSchedulingRulesCmd.Flags().Lookup("limit").Value.Type())

	require.NotNil(t, getBucketsCmd.InheritedFlags().Lookup("limit"))
	require.NotNil(t, getDashboardsCmd.InheritedFlags().Lookup("fields"))
	require.NotNil(t, getWorkflowsCmd.InheritedFlags().Lookup("fields"))
}

func TestEnrichAgent_UnwrapsShapingPrinter(t *testing.T) {
	ap := output.NewAgentPrinter(&bytes.Buffer{}, nil)
	wrapped := output.NewShapingPrinter(ap, output.ShapeOptions{Limit: 1})
	require.Same(t, ap, enrichAgent(wrapped, "get", "bucket"))
	require.Equal(t, "get", ap.Context().Verb)
}

type stderrLog struct{ t *testing.T }

func (s stderrLog) Write(p []byte) (int, error) {
	s.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

// get breakpoints builds its own printer (it writes to rootCmd's writer), so
// it has to opt into the shaping explicitly rather than via NewPrinter.
func TestGetBreakpoints_HonorsLimitAndFields(t *testing.T) {
	origFormat, origAgent, origLimit, origFields := outputFormat, agentMode, getListLimit, getListFields
	origOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		outputFormat, agentMode, getListLimit, getListFields = origFormat, origAgent, origLimit, origFields
		rootCmd.SetOut(origOut)
	})
	outputFormat, agentMode = "csv", false
	getListLimit, getListFields = 1, "lineNumber,id"

	rule := func(id string, line float64) map[string]interface{} {
		return map[string]interface{}{
			"id": id, "is_disabled": false,
			"aug_json": map[string]interface{}{
				"location": map[string]interface{}{"filename": "OrderController.java", "lineno": line},
			},
		}
	}
	deps := liveDebuggerDeps{}
	deps.loadConfig = func() (*config.Config, error) {
		cfg := config.NewConfig()
		cfg.SetContext("test", "https://example.invalid", "token")
		cfg.CurrentContext = "test"
		return cfg, nil
	}
	deps.newClient = func(cfg *config.Config) (*client.Client, error) { return nil, nil }
	deps.newHandler = func(c *client.Client, environment string) (*livedebugger.Handler, error) { return nil, nil }
	deps.getOrCreateWorkspace = func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
		return map[string]interface{}{"data": map[string]interface{}{}}, "ws-1", nil
	}
	deps.getWorkspaceRules = func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
		return map[string]interface{}{"data": map[string]interface{}{"org": map[string]interface{}{
			"workspace": map[string]interface{}{"rules": []interface{}{rule("bp-1", 306), rule("bp-2", 42)}},
		}}}, nil
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	require.NoError(t, runGetBreakpointsWithDeps(nil, nil, deps))
	// Rows come out sorted by file and line, so bp-2 (line 42) is first.
	require.Equal(t, "lineNumber,id\n42,bp-2\n", out.String())
}

// get snapshots runs a DQL query and prints through the query executor, which
// has no --fields projection; the flag must be refused, not silently ignored.
func TestGetSnapshots_RejectsFields(t *testing.T) {
	srv := newListShapeEnv(t)

	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")
	t.Cleanup(restorePristineTree)
	var stderr bytes.Buffer
	code, _ := captureRun(t, []string{"get", "snapshots", "OrderController.java:306", "--fields", "id"}, RunOptions{
		Session: &Session{EnvironmentURL: srv.URL, Token: "t", MinStability: "experimental"},
		Stderr:  &stderr,
	})
	require.NotZero(t, code)
	require.Contains(t, stderr.String(), "--fields", "must fail on the flag, before any request")
}
