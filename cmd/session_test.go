package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

func TestApplyRunEnvironment_ScrubAndRestore(t *testing.T) {
	t.Setenv("DTCTL_TOKEN", "host-secret")
	t.Setenv("DTCTL_CONFIG", "/host/config.yaml")
	t.Setenv("DTCTL_PROFILE", "host-profile")
	os.Unsetenv("DTCTL_DISABLE_KEYRING")

	cleanup, err := applyRunEnvironment(RunOptions{
		Session: &Session{EnvironmentURL: "https://x.example.invalid", Token: "t"},
		Env:     map[string]string{"DTCTL_PROFILE": "query"},
	})
	require.NoError(t, err)

	// Scrubbed for the invocation; explicit Env applied on top.
	_, present := os.LookupEnv("DTCTL_TOKEN")
	require.False(t, present, "session run must scrub host credential env")
	_, present = os.LookupEnv("DTCTL_CONFIG")
	require.False(t, present, "session run must scrub host config selection")
	require.Equal(t, "query", os.Getenv("DTCTL_PROFILE"))
	require.Equal(t, "1", os.Getenv(config.EnvDisableKeyring))
	require.NotNil(t, runSession)

	cleanup()

	require.Equal(t, "host-secret", os.Getenv("DTCTL_TOKEN"))
	require.Equal(t, "/host/config.yaml", os.Getenv("DTCTL_CONFIG"))
	require.Equal(t, "host-profile", os.Getenv("DTCTL_PROFILE"))
	_, present = os.LookupEnv(config.EnvDisableKeyring)
	require.False(t, present, "cleanup must restore prior unset-ness")
	require.Nil(t, runSession)
}

func TestRunWithSession_ValidationErrors(t *testing.T) {
	for name, s := range map[string]*Session{
		"missing url":      {Token: "t"},
		"missing token":    {EnvironmentURL: "https://x.example.invalid"},
		"bad safety level": {EnvironmentURL: "https://x.example.invalid", Token: "t", SafetyLevel: "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			code := Run([]string{"get", "buckets"}, RunOptions{Session: s})
			require.NotZero(t, code)
		})
	}
}

func TestSessionSyntheticConfig(t *testing.T) {
	s := &Session{EnvironmentURL: "https://x.example.invalid", Token: "tok"}
	cfg := s.syntheticConfig()

	ctx, err := cfg.CurrentContextObj()
	require.NoError(t, err)
	require.Equal(t, "https://x.example.invalid", ctx.Environment)
	require.Equal(t, config.DefaultSafetyLevel, ctx.SafetyLevel,
		"empty safety level must select the dtctl default")

	token, err := cfg.GetToken(ctx.TokenRef)
	require.NoError(t, err)
	require.Equal(t, "tok", token)
}

// sessionMockEnv is a fake Dynatrace environment that records the
// Authorization header and whether any mutating request arrived.
type sessionMockEnv struct {
	*httptest.Server
	mu        sync.Mutex
	authSeen  []string
	mutations int
}

func newSessionMockEnv(t *testing.T) *sessionMockEnv {
	t.Helper()
	m := &sessionMockEnv{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.authSeen = append(m.authSeen, r.Header.Get("Authorization"))
		if r.Method != http.MethodGet {
			m.mutations++
		}
		m.mu.Unlock()

		switch {
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"buckets":[{"bucketName":"session_bucket","table":"logs","status":"active","retentionDays":35,"version":1,"updatable":true}]}`))
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"bucketName":"new_bucket","table":"logs","status":"creating","retentionDays":35,"version":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
		}
	}))
	t.Cleanup(m.Server.Close)
	return m
}

// TestRunWithSession_TargetsInjectedEnvironment is the E4 core assertion: a
// session-backed invocation runs against exactly the injected environment and
// token, ignoring host credential env vars and host config entirely.
func TestRunWithSession_TargetsInjectedEnvironment(t *testing.T) {
	env := newSessionMockEnv(t)

	// Hostile host state: none of this may reach the request.
	t.Setenv("DTCTL_TOKEN", "host-secret")
	t.Setenv("DTCTL_CONFIG", "/nonexistent/host/config.yaml")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	code, out := captureRun(t, []string{"get", "buckets", "--plain"}, RunOptions{
		Session: &Session{EnvironmentURL: env.URL, Token: "tenant-token"},
	})
	require.Zero(t, code)
	require.Contains(t, out, "session_bucket")

	env.mu.Lock()
	defer env.mu.Unlock()
	require.NotEmpty(t, env.authSeen)
	for _, auth := range env.authSeen {
		require.Contains(t, auth, "tenant-token", "request must carry the session token")
		require.NotContains(t, auth, "host-secret", "host credential env must never leak into a session request")
	}
}

// TestRunWithSession_ReadOnlyBlocksMutation: the per-session safety level must
// stop a mutating command before any request is sent.
func TestRunWithSession_ReadOnlyBlocksMutation(t *testing.T) {
	env := newSessionMockEnv(t)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	code, _ := captureRun(t,
		[]string{"create", "bucket", "--name", "b", "--table", "logs", "--retention", "35", "--plain"},
		RunOptions{Session: &Session{
			EnvironmentURL: env.URL,
			Token:          "tenant-token",
			SafetyLevel:    config.SafetyLevelReadOnly,
		}})
	require.NotZero(t, code, "readonly session must refuse a create")

	env.mu.Lock()
	defer env.mu.Unlock()
	require.Zero(t, env.mutations, "no mutating request may reach the environment")
}

// TestRunWithSession_WriteAllowedByDefault: the default safety level permits
// mutations, so the same create goes through.
func TestRunWithSession_WriteAllowedByDefault(t *testing.T) {
	env := newSessionMockEnv(t)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	code, _ := captureRun(t,
		[]string{"create", "bucket", "--name", "b", "--table", "logs", "--retention", "35", "--plain"},
		RunOptions{Session: &Session{EnvironmentURL: env.URL, Token: "tenant-token"}})
	require.Zero(t, code)

	env.mu.Lock()
	defer env.mu.Unlock()
	require.Equal(t, 1, env.mutations)
}

// TestRunWithSession_ProfileViaEnv: RunOptions.Env selects a built-in command
// profile per request, and the host's DTCTL_PROFILE is restored afterwards.
func TestRunWithSession_ProfileViaEnv(t *testing.T) {
	t.Setenv("DTCTL_PROFILE", "host-profile-value")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	code, out := captureRun(t, []string{"get", "--help"}, RunOptions{
		Session: &Session{EnvironmentURL: "https://x.example.invalid", Token: "t"},
		Env:     map[string]string{"DTCTL_PROFILE": "query"},
	})
	require.Zero(t, code)
	require.NotContains(t, availableCommandsSection(t, out), "workflows",
		"per-request profile must mask the command surface")

	require.Equal(t, "host-profile-value", os.Getenv("DTCTL_PROFILE"))
}

// TestRunWithSession_VirtualFile is the E6 core assertion: -f resolves
// against the request's virtual filesystem, so a file that exists nowhere on
// the host still applies — exactly the service scenario ("dtctl apply -f
// x.yaml" where x.yaml lives in a LangChain-style virtual file system).
func TestRunWithSession_VirtualFile(t *testing.T) {
	env := newSessionMockEnv(t)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	virtual := vfs.NewMapFS(map[string][]byte{
		"bucket.yaml": []byte("bucketName: virtual_bucket\ntable: logs\nretentionDays: 35\n"),
	})

	code, _ := captureRun(t,
		[]string{"create", "bucket", "-f", "bucket.yaml", "--plain"},
		RunOptions{
			Session: &Session{EnvironmentURL: env.URL, Token: "tenant-token"},
			FS:      virtual,
		})
	require.Zero(t, code, "create from a virtual file must succeed")

	env.mu.Lock()
	mutations := env.mutations
	env.mu.Unlock()
	require.Equal(t, 1, mutations)

	// Without the FS the same invocation must fail: the file is virtual only.
	code, _ = captureRun(t,
		[]string{"create", "bucket", "-f", "bucket.yaml", "--plain"},
		RunOptions{Session: &Session{EnvironmentURL: env.URL, Token: "tenant-token"}})
	require.NotZero(t, code, "the virtual file must not exist on the host")
}

// TestRunWithSession_HostAliasesIgnored: aliases are host-config convenience;
// a tenant request must not expand through them even if the host config
// defines one.
func TestRunWithSession_HostAliasesIgnored(t *testing.T) {
	env := newSessionMockEnv(t)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE", "")

	// Without a session this config would expand `bkts` to `get buckets`.
	isolatedConfig(t, "aliases:\n  bkts: get buckets\ncontexts: []\n")

	code, _ := captureRun(t, []string{"bkts"}, RunOptions{
		Session: &Session{EnvironmentURL: env.URL, Token: "t"},
	})
	require.NotZero(t, code, "alias must not expand for a session-backed invocation")

	env.mu.Lock()
	defer env.mu.Unlock()
	require.Empty(t, env.authSeen, "no request may be sent for an unexpanded alias")
}
