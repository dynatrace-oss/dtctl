package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newDynatraceMock is a minimal fake Dynatrace environment for handler tests.
func newDynatraceMock(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"buckets":[{"bucketName":"serve_bucket","table":"logs","status":"active","retentionDays":35,"version":1,"updatable":true}]}`))
		case r.URL.Path == "/platform/storage/management/v1/bucket-definitions" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"bucketName":"new_bucket","table":"logs","status":"creating","retentionDays":35,"version":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func postExecute(t *testing.T, srv *httptest.Server, body string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/v1/execute", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	return resp, buf.Bytes()
}

func TestHandler_Execute(t *testing.T) {
	dt := newDynatraceMock(t)
	srv := httptest.NewServer(Handler(10 << 20))
	t.Cleanup(srv.Close)

	resp, body := postExecute(t, srv,
		`{"command":"get buckets --plain","environmentUrl":"`+dt.URL+`","token":"tenant-token"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out executeResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.Zero(t, out.ExitCode, "stderr: %s", out.Stderr)
	require.Contains(t, out.Stdout, "serve_bucket")
}

func TestHandler_FilesRoundTrip(t *testing.T) {
	dt := newDynatraceMock(t)
	srv := httptest.NewServer(Handler(10 << 20))
	t.Cleanup(srv.Close)

	req, err := json.Marshal(executeRequest{
		Command:        "create bucket -f bucket.yaml --plain",
		EnvironmentURL: dt.URL,
		Token:          "tenant-token",
		Files:          map[string]string{"bucket.yaml": "bucketName: new_bucket\ntable: logs\nretentionDays: 35\n"},
	})
	require.NoError(t, err)

	resp, body := postExecute(t, srv, string(req))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out executeResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.Zero(t, out.ExitCode, "stderr: %s", out.Stderr)
	require.Contains(t, out.Files, "bucket.yaml",
		"the virtual filesystem's final state travels back to the caller")
}

// TestHandler_CommandFailureIsHTTP200: a command that fails is still a
// successful *execution* — the failure lives in exitCode/stderr, exactly as
// on a local shell. HTTP error statuses are reserved for requests that never
// ran.
func TestHandler_CommandFailureIsHTTP200(t *testing.T) {
	srv := httptest.NewServer(Handler(10 << 20))
	t.Cleanup(srv.Close)

	resp, body := postExecute(t, srv,
		`{"command":"config","environmentUrl":"https://x.example.invalid","token":"t"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out executeResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.NotZero(t, out.ExitCode)
	require.Contains(t, out.Stderr, "not supported in this environment")
}

func TestHandler_RequestErrors(t *testing.T) {
	srv := httptest.NewServer(Handler(1024))
	t.Cleanup(srv.Close)

	t.Run("invalid JSON", func(t *testing.T) {
		resp, _ := postExecute(t, srv, `{not json`)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("missing token", func(t *testing.T) {
		resp, body := postExecute(t, srv,
			`{"command":"get buckets","environmentUrl":"https://x.example.invalid"}`)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, string(body), "Token is required")
	})

	t.Run("body too large", func(t *testing.T) {
		resp, _ := postExecute(t, srv,
			`{"command":"get buckets","stdin":"`+strings.Repeat("x", 2048)+`"}`)
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	})

	t.Run("method not allowed", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/execute")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}

func TestHandler_Healthz(t *testing.T) {
	srv := httptest.NewServer(Handler(1024))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestNewCommand_Registered pins the command surface: `serve` is a protocol
// parent, not a server itself, so a future `serve mcp` lands beside `serve
// http` instead of competing with an incumbent default.
func TestNewCommand_Registered(t *testing.T) {
	c := NewCommand()
	require.Equal(t, "serve", c.Name())
	require.Equal(t, []string{"http"}, protocolNames(c),
		"every server is a named protocol under serve, so a future `serve mcp` "+
			"lands beside `serve http` rather than competing with a default")

	httpCmd, _, err := c.Find([]string{"http"})
	require.NoError(t, err)
	require.NotNil(t, httpCmd.Flags().Lookup("addr"))
	require.NotNil(t, httpCmd.Flags().Lookup("max-request-bytes"))
}

// TestBareServePrintsHelp: naming the protocol is mandatory, and omitting it is
// not an error — help on stdout, exit 0 (Run returns the process exit code).
func TestBareServePrintsHelp(t *testing.T) {
	c := NewCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(nil)

	require.NoError(t, c.Execute())
	require.Contains(t, out.String(), "http")
	require.Contains(t, out.String(), "Available Commands:")
}

// TestUnknownProtocolFails: cobra prints help and exits 0 for an unmatched name
// under a parent with no Run. A supervisor would read that as a started server,
// so serve validates the protocol name itself.
func TestUnknownProtocolFails(t *testing.T) {
	c := NewCommand()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"grpc"})

	err := c.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown protocol "grpc"`)
	require.Contains(t, err.Error(), "http", "the error must list what is available")
}

// TestExperimentalGate: server mode is opt-in. The gate reads the environment
// the same way the account surface does, so operators learn one convention.
func TestExperimentalGate(t *testing.T) {
	for _, c := range []struct {
		value string
		want  bool
	}{
		{"", false}, {"0", false}, {"false", false}, {"no", false}, {"off", false},
		{"OFF", false}, {" false ", false},
		{"1", true}, {"true", true}, {"yes", true}, {"on", true}, {"anything", true},
	} {
		t.Setenv(ExperimentalEnvVar, c.value)
		require.Equalf(t, c.want, Experimental(), "%s=%q", ExperimentalEnvVar, c.value)
	}

	// Unset is off — the released-build default.
	t.Setenv(ExperimentalEnvVar, "")
	require.NoError(t, os.Unsetenv(ExperimentalEnvVar))
	require.False(t, Experimental())
}

// TestExperimentalGateHidesCommand builds the real binary and checks the gate
// end to end: the wiring lives in main (both the dispatch and the registration),
// which no unit test in this package can reach. Without the opt-in, `serve` must
// be an ordinary unknown command — not hidden-but-runnable, and not advertised
// in help or the command catalog.
func TestExperimentalGateHidesCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the dtctl binary; skipped in -short mode")
	}
	name := "dtctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Dir = filepath.Join("..", "..") // module root, where main lives
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", out)

	run := func(env []string, args ...string) (int, string) {
		c := exec.Command(exe, args...)
		c.Env = append(os.Environ(), env...)
		combined, _ := c.CombinedOutput()
		return c.ProcessState.ExitCode(), string(combined)
	}

	off := []string{ExperimentalEnvVar + "="}
	code, output := run(off, "serve", "http", "--addr", "127.0.0.1:0")
	require.NotZero(t, code, "serve must not run without the opt-in")
	require.Contains(t, output, `unknown command "serve"`)

	// Matched as a command entry ("  serve   Run dtctl as a server"), so the
	// assertion does not trip over unrelated prose containing "server".
	_, output = run(off, "--help")
	require.NotRegexp(t, `(?m)^\s+serve\s`, output,
		"an off-by-default surface must not be advertised in help")

	// With the opt-in the command exists again; bare `serve` is discovery.
	code, output = run([]string{ExperimentalEnvVar + "=1"}, "serve")
	require.Zero(t, code, "bare serve prints help and exits 0: %s", output)
	require.Contains(t, output, "http")
}
