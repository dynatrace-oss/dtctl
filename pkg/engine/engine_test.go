package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/engine"
)

// mockEnv is a fake Dynatrace environment recording the Authorization header
// of every request and how many mutating requests arrived.
type mockEnv struct {
	*httptest.Server
	mu        sync.Mutex
	authSeen  []string
	mutations int
}

func newMockEnv(t *testing.T) *mockEnv {
	t.Helper()
	m := &mockEnv{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.authSeen = append(m.authSeen, r.Header.Get("Authorization"))
		if r.Method != http.MethodGet {
			m.mutations++
		}
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"buckets":[{"bucketName":"engine_bucket","table":"logs","status":"active","retentionDays":35,"version":1,"updatable":true}]}`))
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"bucketName":"new_bucket","table":"logs","status":"creating","retentionDays":35,"version":1}`))
		case r.URL.Path == "/platform/automation/v1/workflows" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"wf-generated-1","title":"engine-created"}`))
		case r.URL.Path == "/platform/openpipeline/v1/matcher/lqlToDql" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"query":"matchesValue(log.source, \"stdin-test\")"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
		}
	}))
	t.Cleanup(m.Server.Close)
	return m
}

func TestExecute_RequestValidation(t *testing.T) {
	for name, req := range map[string]engine.Request{
		"no command":       {EnvironmentURL: "https://x.example.invalid", Token: "t"},
		"unparsable quote": {Command: `create bucket -f "unclosed`, EnvironmentURL: "https://x.example.invalid", Token: "t"},
		"missing url":      {Command: "get buckets", Token: "t"},
		"missing token":    {Command: "get buckets", EnvironmentURL: "https://x.example.invalid"},
		"bad safety level": {Command: "get buckets", EnvironmentURL: "https://x.example.invalid", Token: "t", SafetyLevel: "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := engine.Execute(context.Background(), req)
			require.Error(t, err, "malformed requests must fail before running")
			require.Nil(t, res)
		})
	}
}

// TestExecute_CommandString is the core service scenario: a command string
// exactly as typed locally, executed against the request's tenant — while the
// host process carries hostile credentials and its own AI-agent environment,
// none of which may shape the request.
func TestExecute_CommandString(t *testing.T) {
	env := newMockEnv(t)
	t.Setenv("DTCTL_TOKEN", "host-secret")
	t.Setenv("DTCTL_CONFIG", "/nonexistent/host/config.yaml")
	// Deliberately NOT scrubbing AI-agent env vars (CLAUDECODE, AI_AGENT, ...):
	// session-backed runs skip agent auto-detection, so plain table output
	// here proves host env does not leak into request output.

	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "get buckets --plain",
		EnvironmentURL: env.URL,
		Token:          "tenant-token",
	})
	require.NoError(t, err)
	require.Zero(t, res.ExitCode, "stderr: %s", res.Stderr)
	require.Contains(t, string(res.Stdout), "engine_bucket")
	require.NotContains(t, string(res.Stdout), `"ok":`,
		"host AI-agent env must not flip output into envelopes")

	env.mu.Lock()
	defer env.mu.Unlock()
	require.NotEmpty(t, env.authSeen)
	for _, auth := range env.authSeen {
		require.Contains(t, auth, "tenant-token")
		require.NotContains(t, auth, "host-secret")
	}
}

// TestExecute_QuotedFilename: the command string is split with shell quoting
// rules, so a filename with spaces resolves against the virtual filesystem.
func TestExecute_QuotedFilename(t *testing.T) {
	env := newMockEnv(t)

	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        `create bucket -f "my bucket.yaml" --plain`,
		EnvironmentURL: env.URL,
		Token:          "tenant-token",
		Files: map[string][]byte{
			"my bucket.yaml": []byte("bucketName: new_bucket\ntable: logs\nretentionDays: 35\n"),
		},
	})
	require.NoError(t, err)
	require.Zero(t, res.ExitCode, "stderr: %s", res.Stderr)

	env.mu.Lock()
	defer env.mu.Unlock()
	require.Equal(t, 1, env.mutations)
}

// TestExecute_WritebackReachesResultFiles: `apply --write-id` stamps the
// generated ID into the request's virtual file, and the caller receives the
// updated file in Result.Files — the full round-trip a LangChain-style
// virtual filesystem needs.
func TestExecute_WritebackReachesResultFiles(t *testing.T) {
	env := newMockEnv(t)

	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "apply -f workflow.yaml --write-id --plain",
		EnvironmentURL: env.URL,
		Token:          "tenant-token",
		Files: map[string][]byte{
			"workflow.yaml": []byte("title: engine-created\ntasks: {}\n"),
		},
	})
	require.NoError(t, err)
	require.Zero(t, res.ExitCode, "stderr: %s", res.Stderr)
	require.Contains(t, string(res.Files["workflow.yaml"]), "wf-generated-1",
		"the generated workflow id must be stamped into the virtual file")
}

// TestExecute_StdinDash: a stdin-capable command (`-f -`) reads the request's
// stdin bytes, not the host process stdin.
func TestExecute_StdinDash(t *testing.T) {
	env := newMockEnv(t)

	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "translate lql-to-dql -f - --plain",
		EnvironmentURL: env.URL,
		Token:          "tenant-token",
		Stdin:          []byte(`log.source="stdin-test"`),
	})
	require.NoError(t, err)
	require.Zero(t, res.ExitCode, "stderr: %s", res.Stderr)
	require.Contains(t, string(res.Stdout), "matchesValue",
		"the translation of the stdin-fed expression must be printed")
}

// TestExecute_UnsupportedCommands: every policy-blocked command must refuse to
// run. Registered commands answer with the signposted unsupported error;
// either way the exit code is non-zero and nothing executes.
func TestExecute_UnsupportedCommands(t *testing.T) {
	for name := range engine.UnsupportedCommands() {
		t.Run(name, func(t *testing.T) {
			res, err := engine.Execute(context.Background(), engine.Request{
				Argv:           []string{name},
				EnvironmentURL: "https://x.example.invalid",
				Token:          "t",
			})
			require.NoError(t, err)
			require.NotZero(t, res.ExitCode)
		})
	}

	// With --agent the block renders as a structured envelope with the
	// stable code, so machine callers can branch on it.
	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "config --agent",
		EnvironmentURL: "https://x.example.invalid",
		Token:          "t",
	})
	require.NoError(t, err)
	require.NotZero(t, res.ExitCode)
	require.Contains(t, string(res.Stdout), `"code":"unsupported_in_service"`)
}

// TestExecute_BlockedHiddenFromCatalog: an agent bootstrapping from the
// `dtctl commands` catalog must not see commands the service cannot run.
func TestExecute_BlockedHiddenFromCatalog(t *testing.T) {
	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "commands",
		EnvironmentURL: "https://x.example.invalid",
		Token:          "t",
	})
	require.NoError(t, err)
	require.Zero(t, res.ExitCode, "stderr: %s", res.Stderr)
	out := string(res.Stdout)
	require.Contains(t, out, "get", "the resource surface stays visible")
	// "skills:" (with colon) is the catalog entry for the top-level command;
	// the bare word also appears in the unrelated "copilot-skills" resource.
	for _, blocked := range []string{"doctor", "skills:", "completion"} {
		require.NotContains(t, out, blocked)
	}
}

// TestExecute_ReadonlyBlocksMutation: the per-request safety level stops a
// mutating command before any API request leaves the process.
func TestExecute_ReadonlyBlocksMutation(t *testing.T) {
	env := newMockEnv(t)

	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "create bucket --name b --table logs --retention 35 --plain",
		EnvironmentURL: env.URL,
		Token:          "tenant-token",
		SafetyLevel:    "readonly",
	})
	require.NoError(t, err)
	require.NotZero(t, res.ExitCode)

	env.mu.Lock()
	defer env.mu.Unlock()
	require.Zero(t, env.mutations)
}

// TestExecute_ProfileMasksSurface: a per-request profile reduces the command
// surface with the stable profile_blocked code.
func TestExecute_ProfileMasksSurface(t *testing.T) {
	res, err := engine.Execute(context.Background(), engine.Request{
		Command:        "get buckets --agent",
		EnvironmentURL: "https://x.example.invalid",
		Token:          "t",
		Profile:        "query",
	})
	require.NoError(t, err)
	require.NotZero(t, res.ExitCode)
	require.Contains(t, string(res.Stdout), `"code":"profile_blocked"`)
}

// TestExecute_ContextCancelledBeforeStart: a request whose context is already
// done never runs.
func TestExecute_ContextCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := engine.Execute(ctx, engine.Request{
		Command:        "get buckets",
		EnvironmentURL: "https://x.example.invalid",
		Token:          "t",
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, res)
}

// TestExecute_ConcurrentRequests: Execute is goroutine-safe (requests
// serialize internally); every caller gets its own complete result.
func TestExecute_ConcurrentRequests(t *testing.T) {
	env := newMockEnv(t)

	var wg sync.WaitGroup
	results := make([]*engine.Result, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := engine.Execute(context.Background(), engine.Request{
				Command:        "get buckets --plain",
				EnvironmentURL: env.URL,
				Token:          "tenant-token",
			})
			require.NoError(t, err)
			results[i] = res
		}(i)
	}
	wg.Wait()

	for i, res := range results {
		require.Zero(t, res.ExitCode, "request %d failed: %s", i, res.Stderr)
		require.Contains(t, string(res.Stdout), "engine_bucket", "request %d", i)
	}
}
