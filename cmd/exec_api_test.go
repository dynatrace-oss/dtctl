package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// runAPI invokes dtctl through the embedding entrypoint with both streams
// captured, which is what these tests need: a refusal is written to stderr while
// a response goes to stdout, and several assertions here are about which stream
// carried what.
func runAPI(t *testing.T, argv []string, opts RunOptions) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	opts.Stdout, opts.Stderr = &out, &errOut
	code = Run(argv, opts)
	return code, out.String(), errOut.String()
}

// TestExecAPIReadIsPermittedInAReadonlyContext is the baseline: a GET is a read
// by every measure available, so the strictest context still allows it.
func TestExecAPIReadIsPermittedInAReadonlyContext(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")
	env.respond(http.MethodGet, "/platform/widget/v1/widgets", mockResponse{
		status:      200,
		contentType: "application/json",
		body:        `{"widgets":[{"name":"one"}]}`,
	})

	code, out, errOut := runAPI(t, []string{"exec", "api", "/platform/widget/v1/widgets"}, RunOptions{})
	require.Zero(t, code, errOut)
	require.Contains(t, out, `"name": "one"`, "the response body is the output")
}

// TestExecAPIMethodDoesNotDecideTheGate is the load-bearing test of the whole
// classification design.
//
// Both requests below are POSTs to the same API in the same readonly context. One
// is permitted and one is refused, and the only thing that separates them is the
// scope their operation declares in the environment's own specification. A gate
// keyed on the HTTP method could not produce this result in either direction: it
// would refuse the search (breaking reads) or permit the create (breaking the
// point of readonly).
func TestExecAPIMethodDoesNotDecideTheGate(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")
	env.respond(http.MethodPost, "/platform/widget/v1/widgets:search", mockResponse{
		status:      200,
		contentType: "application/json",
		body:        `{"matches":[]}`,
	})
	env.respond(http.MethodPost, "/platform/widget/v1/widgets", mockResponse{
		status: 201,
		body:   `{"name":"new"}`,
	})

	t.Run("a POST declaring a read scope is a read", func(t *testing.T) {
		code, out, errOut := runAPI(t, []string{
			"exec", "api", "/platform/widget/v1/widgets:search",
			"-X", "POST", "-d", `{"match":"o"}`,
		}, RunOptions{})
		require.Zero(t, code, errOut)
		require.Contains(t, out, "matches")
		require.Equal(t, `{"match":"o"}`,
			env.bodyOf(http.MethodPost, "/platform/widget/v1/widgets:search"),
			"the body must reach the endpoint unaltered")
	})

	t.Run("a POST declaring a write scope is a write", func(t *testing.T) {
		code, _, errOut := runAPI(t, []string{
			"exec", "api", "/platform/widget/v1/widgets",
			"-X", "POST", "-d", `{"name":"new"}`,
		}, RunOptions{})
		require.NotZero(t, code, "a readonly context must refuse a declared write")
		require.Contains(t, errOut, "widget:widgets:write",
			"the refusal must name the scope it classified from")
		require.False(t, env.called(http.MethodPost, "/platform/widget/v1/widgets"),
			"a refused request must never reach the wire")
	})
}

// TestExecAPIUnresolvedRequestIsGatedAsTheStrictestOperation pins the fail-closed
// fallback, and pins that the message does not advertise a way around it.
func TestExecAPIUnresolvedRequestIsGatedAsTheStrictestOperation(t *testing.T) {
	env := newMockEnvironmentAt(t, "readwrite-mine")

	code, _, errOut := runAPI(t, []string{
		"exec", "api", "/platform/sprocket/v1/sprockets", "-X", "POST", "-d", `{}`,
	}, RunOptions{})

	require.NotZero(t, code)
	require.Contains(t, errOut, "cannot tell what POST does")
	require.Contains(t, errOut, "no flag to assert the operation",
		"the refusal must say the escape hatch has no escape hatch")
	require.NotContains(t, errOut, "--op",
		"the message must not name an override that does not exist")
	require.False(t, env.called(http.MethodPost, "/platform/sprocket/v1/sprockets"))
}

// TestExecAPIDoesNotUndercutTheCommandItShadows pins the reason the curated
// destructive table exists: without it this request would pass at readwrite-all
// while `dtctl delete bucket` demands dangerously-unrestricted, making the
// passthrough a weaker gate than the native command.
func TestExecAPIDoesNotUndercutTheCommandItShadows(t *testing.T) {
	const path = "/platform/storage/management/v1/bucket-definitions/custom_example"
	env := newMockEnvironmentAt(t, "readwrite-all")

	code, _, errOut := runAPI(t, []string{"exec", "api", path, "-X", "DELETE"}, RunOptions{})

	require.NotZero(t, code)
	require.Contains(t, errOut, "dangerously-unrestricted")
	require.Contains(t, errOut, "dtctl get buckets",
		"a refusal must point at the native command")
	require.False(t, env.called(http.MethodDelete, path))
}

// TestExecAPIRefusesToInferAMethod pins that a body does not promote a request to
// POST the way curl does. Inferring the method would infer the safety operation,
// which is the one thing this command must never do.
func TestExecAPIRefusesToInferAMethod(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")

	code, _, errOut := runAPI(t, []string{
		"exec", "api", "/platform/widget/v1/widgets", "-d", `{"name":"x"}`,
	}, RunOptions{})

	require.NotZero(t, code)
	require.Contains(t, errOut, "needs an explicit method")
	require.Zero(t, env.requestCount(), "the refusal must not cost a request")
}

// TestExecAPIRefusesToLeaveTheEnvironment pins that the passthrough cannot be
// pointed at another host. dtctl attaches the context's credentials to whatever it
// sends, so following an absolute URL would hand them to an arbitrary server.
func TestExecAPIRefusesToLeaveTheEnvironment(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")

	for _, target := range []string{"https://example.invalid/x", "widgets"} {
		code, _, errOut := runAPI(t, []string{"exec", "api", target}, RunOptions{})
		require.NotZero(t, code, "target %q", target)
		require.NotEmpty(t, errOut)
	}
	require.Zero(t, env.requestCount(), "validation must precede every request")
}

// TestExecAPIBodyFileComesFromTheRequestFilesystem is the embedding guard for
// `-d @file`.
//
// The path names a file that exists only in the invocation's filesystem — there is
// no such file on the host disk. An os.ReadFile in this path would fail here, and
// in a service it would read the *server's* disk on a caller's behalf.
func TestExecAPIBodyFileComesFromTheRequestFilesystem(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")
	env.respond(http.MethodPost, "/platform/widget/v1/widgets:search", mockResponse{
		status:      200,
		contentType: "application/json",
		body:        `{"matches":[]}`,
	})

	code, _, errOut := runAPI(t, []string{
		"exec", "api", "/platform/widget/v1/widgets:search", "-X", "POST", "-d", "@search.json",
	}, RunOptions{
		FS: vfs.NewMapFS(map[string][]byte{"search.json": []byte(`{"match":"from-vfs"}`)}),
	})

	require.Zero(t, code, errOut)
	require.Equal(t, `{"match":"from-vfs"}`,
		env.bodyOf(http.MethodPost, "/platform/widget/v1/widgets:search"),
		"the body must come from the request's filesystem, not the host's")
}

// TestExecAPIBodyFromStdinUsesTheStreamSeam pins the other half of the same rule:
// "@-" must resolve to the invocation's stdin, not to /dev/stdin as a path.
func TestExecAPIBodyFromStdinUsesTheStreamSeam(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")
	env.respond(http.MethodPost, "/platform/widget/v1/widgets:search", mockResponse{
		status:      200,
		contentType: "application/json",
		body:        `{"matches":[]}`,
	})

	code, _, errOut := runAPI(t, []string{
		"exec", "api", "/platform/widget/v1/widgets:search", "-X", "POST", "-d", "@-",
	}, RunOptions{Stdin: strings.NewReader(`{"match":"from-stdin"}`)})

	require.Zero(t, code, errOut)
	require.Equal(t, `{"match":"from-stdin"}`,
		env.bodyOf(http.MethodPost, "/platform/widget/v1/widgets:search"))
}

// TestExecAPIDryRunShowsTheVerdictWithoutSendingOrLeaking pins the two properties
// of --dry-run: nothing is sent, and the output is safe to paste into a bug report
// even when the caller passed a credential in a header.
func TestExecAPIDryRunShowsTheVerdictWithoutSendingOrLeaking(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")

	code, out, errOut := runAPI(t, []string{
		"exec", "api", "/platform/widget/v1/widgets", "-X", "POST", "-d", `{"name":"x"}`,
		"-H", "Authorization: Bearer super-secret", "--dry-run",
	}, RunOptions{})

	require.Zero(t, code, errOut)
	require.Contains(t, out, "Nothing was sent")
	require.Contains(t, out, "create", "the dry run reports the gate it would apply")
	require.Contains(t, out, "BLOCKED in this context",
		"a dry run reports the verdict rather than hiding the composed request behind it")
	require.Contains(t, out, "<redacted>")
	require.NotContains(t, out, "super-secret")
	require.False(t, env.called(http.MethodPost, "/platform/widget/v1/widgets"))
}

// TestExecAPIOutputProtocolIsDeclaredNotSniffed pins the contract that makes the
// passthrough usable from a program: a JSON body goes through the envelope like
// every other command, anything else is passed through byte-for-byte even under
// --agent, and a failure is always structured.
func TestExecAPIOutputProtocolIsDeclaredNotSniffed(t *testing.T) {
	t.Run("json is wrapped in the envelope", func(t *testing.T) {
		env := newMockEnvironmentAt(t, "readonly")
		env.respond(http.MethodGet, "/platform/widget/v1/widgets", mockResponse{
			status:      200,
			contentType: "application/json",
			body:        `{"widgets":[{"name":"one"}]}`,
		})

		code, out, errOut := runAPI(t, []string{
			"exec", "api", "/platform/widget/v1/widgets", "--agent",
		}, RunOptions{})
		require.Zero(t, code, errOut)

		var resp struct {
			OK     bool           `json:"ok"`
			Result map[string]any `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &resp), "stdout must be one envelope: %s", out)
		require.True(t, resp.OK)
		require.Contains(t, resp.Result, "widgets")
	})

	t.Run("a non-json body is passed through verbatim", func(t *testing.T) {
		const csv = "name,colour\none,red\n"
		env := newMockEnvironmentAt(t, "readonly")
		env.respond(http.MethodGet, "/platform/widget/v1/widgets", mockResponse{
			status:      200,
			contentType: "text/csv",
			body:        csv,
		})

		code, out, errOut := runAPI(t, []string{
			"exec", "api", "/platform/widget/v1/widgets", "--agent",
		}, RunOptions{})
		require.Zero(t, code, errOut)
		require.Equal(t, csv, out,
			"wrapping a CSV export in an envelope would corrupt it; refusing to emit it "+
				"would make the command useless for the exports it exists to reach")
	})

	t.Run("a failure keeps the platform's own message", func(t *testing.T) {
		env := newMockEnvironmentAt(t, "readonly")
		env.respond(http.MethodGet, "/platform/widget/v1/widgets", mockResponse{
			status:      400,
			contentType: "application/json",
			body:        `{"error":{"message":"page-size must be positive"}}`,
		})

		code, out, _ := runAPI(t, []string{
			"exec", "api", "/platform/widget/v1/widgets", "--agent",
		}, RunOptions{})
		require.NotZero(t, code)

		var resp struct {
			OK    bool `json:"ok"`
			Error struct {
				Message     string   `json:"message"`
				StatusCode  int      `json:"status_code"`
				Suggestions []string `json:"suggestions"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &resp), "stdout: %s", out)
		require.False(t, resp.OK)
		require.Equal(t, 400, resp.Error.StatusCode)
		require.Contains(t, resp.Error.Message, "page-size must be positive",
			"the platform's explanation is the authoritative one and must survive")
		require.Contains(t, strings.Join(resp.Error.Suggestions, "\n"), "describe api",
			"a failed guess must come back with the operation's schema, not just a status")
	})
}

// TestExecAPIWarnsWhenANativeCommandAlreadyCoversThePath pins the notice that
// keeps the escape hatch self-deprecating, and pins that it goes to stderr so a
// program parsing stdout is unaffected.
func TestExecAPIWarnsWhenANativeCommandAlreadyCoversThePath(t *testing.T) {
	env := newMockEnvironmentAt(t, "readonly")
	env.respond(http.MethodGet, "/platform/document/v1/documents", mockResponse{
		status:      200,
		contentType: "application/json",
		body:        `{"documents":[]}`,
	})

	code, out, errOut := runAPI(t, []string{
		"exec", "api", "/platform/document/v1/documents", "--agent",
	}, RunOptions{})

	require.Zero(t, code, errOut)
	require.Contains(t, errOut, "dtctl get documents")
	require.NotContains(t, out, "Warning",
		"the notice must not contaminate the stream a program parses")
	require.NoError(t, json.Unmarshal([]byte(out), new(map[string]any)))
}
