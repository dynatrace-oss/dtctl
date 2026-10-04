package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	builtinrecipes "github.com/dynatrace-oss/dtctl/recipes"
)

// recipeEnv is a mock environment for recipe tests: a query endpoint that
// returns fixed records and records every statement it received, plus an
// optional Document API serving app bundles.
type recipeEnv struct {
	srv *httptest.Server

	mu       sync.Mutex
	queries  []string
	paths    []string
	records  string
	docs     string            // document list JSON; "" serves an empty list
	contents map[string]string // document id → content
	apps     []string          // Session.RecipeApps
}

func newRecipeEnv(t *testing.T) *recipeEnv {
	t.Helper()
	e := &recipeEnv{records: `[]`, contents: map[string]string{}}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		e.mu.Lock()
		defer e.mu.Unlock()
		e.paths = append(e.paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/platform/storage/query/v1/query:execute":
			var req struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(body, &req)
			e.queries = append(e.queries, req.Query)
			_, _ = io.WriteString(w, `{"state":"SUCCEEDED","result":{"records":`+e.records+`}}`)
		case r.URL.Path == "/platform/document/v1/documents":
			docs := e.docs
			if docs == "" {
				docs = `[]`
			}
			_, _ = io.WriteString(w, `{"documents":`+docs+`,"totalCount":0}`)
		case strings.HasPrefix(r.URL.Path, "/platform/document/v1/documents/") && strings.HasSuffix(r.URL.Path, "/content"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/platform/document/v1/documents/"), "/content")
			c, ok := e.contents[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, c)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *recipeEnv) lastQuery() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.queries) == 0 {
		return ""
	}
	return e.queries[len(e.queries)-1]
}

func (e *recipeEnv) fetched(path string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.paths {
		if p == path {
			return true
		}
	}
	return false
}

// run executes one session-backed invocation, which reads no recipe
// directory and caches bundles in memory, so the developer's own recipes
// and cache never leak into a test.
func (e *recipeEnv) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	clearAgentEnvVars(t)
	t.Cleanup(restorePristineTree)
	var stdout, stderr bytes.Buffer
	code := Run(args, RunOptions{
		Session: &Session{EnvironmentURL: e.srv.URL, Token: "t", MinStability: "experimental", RecipeApps: e.apps},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	return code, stdout.String(), stderr.String()
}

type recipeEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Context map[string]json.RawMessage `json:"context"`
}

func parseRecipeEnvelope(t *testing.T, out string) recipeEnvelope {
	t.Helper()
	var env recipeEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &env), "stdout: %s", out)
	return env
}

func TestRunRecipeDryRunSendsNothing(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, stderr := e.run(t, "run", "problems-active", "--category", "ERROR", "--from", "6h", "--dry-run", "--plain")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "fetch dt.davis.problems")
	assert.Contains(t, stdout, `| filter event.category == "ERROR"`)
	assert.Empty(t, e.queries, "a dry run executes no query")
}

func TestRunRecipeDryRunAgentEnvelope(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, stderr := e.run(t, "run", "problems-active", "--dry-run", "-A")
	require.Zero(t, code, "stderr: %s", stderr)
	env := parseRecipeEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Contains(t, string(env.Context["recipe"]), "problems-active")
	assert.Contains(t, string(env.Result), "fetch dt.davis.problems")
}

func TestRunRecipeAgentEnvelope(t *testing.T) {
	e := newRecipeEnv(t)
	e.records = `[{"display_id":"P-1","event.name":"High CPU","event.category":"RESOURCE"}]`
	code, stdout, stderr := e.run(t, "run", "problems-active", "--from", "6h", "-A")
	require.Zero(t, code, "stderr: %s", stderr)

	q := e.lastQuery()
	assert.Contains(t, q, "fetch dt.davis.problems")
	assert.Contains(t, q, `event.status == "ACTIVE"`)

	env := parseRecipeEnvelope(t, stdout)
	require.True(t, env.OK)
	assert.Contains(t, string(env.Context["recipe"]), "problems-active")
	assert.Contains(t, string(env.Context["query"]), "fetch dt.davis.problems")
	assert.Contains(t, string(env.Context["window"]), `"from"`)
	assert.Contains(t, string(env.Context["suggestions"]), "dtctl run problems-get P-1",
		"a next edge binds the result row's display_id")
}

func TestRunRecipeEmptyResultCarriesEmptyMeans(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, stderr := e.run(t, "run", "problems-active", "-A")
	require.Zero(t, code, "stderr: %s", stderr)
	env := parseRecipeEnvelope(t, stdout)
	require.True(t, env.OK)
	assert.Contains(t, string(env.Context["empty_reason"]), "recipe_empty_means")
	assert.Contains(t, string(env.Context["empty_reason"]), "widen --from")
}

func TestRunRecipeEmptyResultHintsInHumanMode(t *testing.T) {
	e := newRecipeEnv(t)
	code, _, stderr := e.run(t, "run", "problems-active")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Contains(t, stderr, "Hint: No problem")
	assert.Contains(t, stderr, "widen --from")
}

func TestRunRecipeScopeRendersIntoQuery(t *testing.T) {
	e := newRecipeEnv(t)
	code, _, stderr := e.run(t, "run", "problems-active", "--cluster", "prod-eu", "--plain", "-o", "json")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Contains(t, e.lastQuery(), `"prod-eu"`)
}

func TestRunUnknownRecipe(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, _ := e.run(t, "run", "problems-nope", "-A")
	assert.NotZero(t, code)
	env := parseRecipeEnvelope(t, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "unknown_command", env.Error.Code)
	assert.Contains(t, env.Error.Message, "problems-nope")
	assert.Empty(t, e.queries)
}

func TestRunRecipeMissingRequiredParam(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, _ := e.run(t, "run", "problems-get", "-A")
	assert.Equal(t, client.ExitUsageError, code)
	env := parseRecipeEnvelope(t, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "validation_error", env.Error.Code)
	assert.Empty(t, e.queries)
}

func TestRunRecipeRejectsBadEnum(t *testing.T) {
	e := newRecipeEnv(t)
	code, _, stderr := e.run(t, "run", "problems-active", "--category", "NOPE")
	assert.Equal(t, client.ExitUsageError, code)
	assert.Contains(t, stderr, `"NOPE" is not one of AVAILABILITY`)
	assert.Empty(t, e.queries)
}

func TestGetRecipesListsBuiltins(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, stderr := e.run(t, "get", "recipes", "--plain", "--no-inventory")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "problems-active")
	assert.Contains(t, stdout, "builtin")
}

func TestGetRecipesAgentModeShowsDomainIndex(t *testing.T) {
	e := newRecipeEnv(t)
	code, stdout, stderr := e.run(t, "get", "recipes", "-A", "--no-inventory")
	require.Zero(t, code, "stderr: %s", stderr)
	env := parseRecipeEnvelope(t, stdout)
	require.True(t, env.OK)
	assert.Contains(t, string(env.Result), `"problems"`)
	assert.NotContains(t, string(env.Result), "problems-active",
		"an unfiltered agent listing is the domain index, not every recipe")

	code, stdout, stderr = e.run(t, "get", "recipes", "-A", "--no-inventory", "--domain", "problems")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Contains(t, string(parseRecipeEnvelope(t, stdout).Result), "problems-active")
}

const testBundle = `apiVersion: dtctl.dev/v1alpha1
kind: RecipeBundle
metadata:
  name: demo
  version: 1
spec:
  recipes:
    - metadata: {name: problems-from-app, version: 1}
      spec:
        summary: A recipe an app ships
        timeframe: 1h
        dql: fetch dt.davis.problems | limit 3
        means: m
        emptyMeans: e
`

func TestRunRecipeFromAppBundle(t *testing.T) {
	e := newRecipeEnv(t)
	e.docs = `[
	  {"id":"doc-app","name":"/recipes/demo.yaml","type":"ai-agent-resource","version":1,"originAppId":"my.demo.app"},
	  {"id":"doc-hand","name":"/recipes/hand.yaml","type":"ai-agent-resource","version":1}
	]`
	e.contents["doc-app"] = testBundle
	e.contents["doc-hand"] = strings.ReplaceAll(testBundle, "problems-from-app", "problems-hand-made")

	// Nothing is enabled: a request sees the built-in recipes only, and
	// does not even list the environment's bundles.
	code, _, _ := e.run(t, "run", "problems-from-app", "--plain")
	assert.NotZero(t, code)
	assert.False(t, e.fetched("/platform/document/v1/documents"), "no app enabled, no listing")

	e.apps = []string{"my.demo.app"}
	code, _, stderr := e.run(t, "run", "problems-from-app", "--plain", "-o", "json")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Equal(t, "fetch dt.davis.problems | limit 3", strings.TrimSpace(e.lastQuery()))
	assert.True(t, e.fetched("/platform/document/v1/documents/doc-app/content"))
	assert.False(t, e.fetched("/platform/document/v1/documents/doc-hand/content"),
		"a document no app installed is never downloaded")

	// A request pins a version; a bundle at another version is skipped.
	e.apps = []string{"my.demo.app@2"}
	code, _, _ = e.run(t, "run", "problems-from-app", "--plain")
	assert.NotZero(t, code, "the pinned version is not what the environment serves")
}

// TestSessionBundleCacheIsPerEnvironment: two environments serving a bundle
// under the same document ID and version must each get their own content.
// A document ID is no secret, so a cache keyed on it alone would let one
// tenant decide what another tenant's request runs.
func TestSessionBundleCacheIsPerEnvironment(t *testing.T) {
	const docs = `[{"id":"doc-same","name":"/recipes/demo.yaml","type":"ai-agent-resource","version":1,"originAppId":"my.demo.app"}]`
	a, b := newRecipeEnv(t), newRecipeEnv(t)
	a.docs, b.docs = docs, docs
	a.contents["doc-same"] = testBundle
	b.contents["doc-same"] = strings.ReplaceAll(testBundle, "limit 3", "limit 7")
	a.apps, b.apps = []string{"my.demo.app"}, []string{"my.demo.app"}

	code, _, stderr := a.run(t, "run", "problems-from-app", "--plain", "-o", "json")
	require.Zero(t, code, "stderr: %s", stderr)
	code, _, stderr = b.run(t, "run", "problems-from-app", "--plain", "-o", "json")
	require.Zero(t, code, "stderr: %s", stderr)
	assert.Equal(t, "fetch dt.davis.problems | limit 7", strings.TrimSpace(b.lastQuery()))
	assert.True(t, b.fetched("/platform/document/v1/documents/doc-same/content"))
}

// TestRecipeUserDirectoryLoads covers the CLI path (no session): the user
// layer is read from disk and shadows nothing it does not name.
func TestRecipeUserDirectoryLoads(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "problems-mine.yaml"), []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-mine, version: 1}
spec:
  summary: Mine
  timeframe: none
  dql: fetch dt.davis.problems | limit 1
  means: m
  emptyMeans: e
`), 0o600))
	origUser, origCache := recipeUserDir, recipeCacheRoot
	recipeUserDir = func() string { return dir }
	cacheDir := t.TempDir()
	recipeCacheRoot = func() string { return cacheDir }
	t.Cleanup(func() { recipeUserDir, recipeCacheRoot = origUser, origCache })
	isolatedConfig(t, "apiVersion: v1\nkind: Config\ncontexts: []\n")
	t.Setenv(recipePathEnv, "")
	t.Cleanup(restorePristineTree)

	code, out := captureRun(t, []string{"run", "problems-mine", "--dry-run", "--plain"}, RunOptions{})
	require.Zero(t, code)
	assert.Contains(t, out, "fetch dt.davis.problems | limit 1")
}

func TestVerifyRecipeOfflineChecksAFile(t *testing.T) {
	e := newRecipeEnv(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(good, []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-good, version: 1}
spec:
  summary: Good
  timeframe: 1h
  dql: fetch dt.davis.problems
  means: m
  emptyMeans: e
`), 0o600))
	require.NoError(t, os.WriteFile(bad, []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-bad, version: 1}
spec:
  summary: Bad
  timeframe: 1h
  dql: fetch logs | filter x == {{.service}}
  means: m
`), 0o600))

	code, stdout, stderr := e.run(t, "verify", "recipe", "-f", good, "--offline", "--plain")
	require.Zero(t, code, "stdout: %s stderr: %s", stdout, stderr)
	assert.Empty(t, e.queries, "--offline sends no query")

	code, stdout, _ = e.run(t, "verify", "recipe", "-f", bad, "--offline", "--plain")
	assert.NotZero(t, code)
	assert.Contains(t, stdout, "emptyMeans is required")
}

// TestVerifyRecipeFileSeesOtherLayers: a file's next targets and domain may
// come from any loaded layer, not only the built-in one.
func TestVerifyRecipeFileSeesOtherLayers(t *testing.T) {
	org := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(org, "_domains.yaml"), []byte(
		"apiVersion: dtctl.dev/v1alpha1\nkind: RecipeDomains\ndomains:\n  payments: Payment flows\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(org, "payments-list.yaml"), []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: payments-list, version: 1}
spec:
  summary: List
  timeframe: none
  dql: fetch dt.davis.problems
  means: m
  emptyMeans: e
`), 0o600))
	file := filepath.Join(t.TempDir(), "payments-slow.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: payments-slow, version: 1}
spec:
  summary: Slow
  timeframe: 1h
  dql: fetch spans
  means: m
  emptyMeans: e
  next:
    - recipe: payments-list
`), 0o600))
	origUser, origCache := recipeUserDir, recipeCacheRoot
	empty, cacheDir := t.TempDir(), t.TempDir()
	recipeUserDir = func() string { return empty }
	recipeCacheRoot = func() string { return cacheDir }
	t.Cleanup(func() { recipeUserDir, recipeCacheRoot = origUser, origCache })
	isolatedConfig(t, "apiVersion: v1\nkind: Config\ncontexts: []\n")
	t.Setenv(recipePathEnv, org)
	t.Cleanup(restorePristineTree)

	code, out := captureRun(t, []string{"verify", "recipe", "-f", file, "--offline", "--plain", "-o", "json"}, RunOptions{})
	assert.Zero(t, code, out)
	assert.Contains(t, out, "valid (offline)")
}

func TestHiddenBetterMatchesNamesWhatInventoryHid(t *testing.T) {
	book := recipes.Loader{Builtin: recipes.FileLayer{Layer: recipes.LayerBuiltin, FS: builtinrecipes.FS(), Root: "builtin"}}.Load()
	hidden, hiddenFor := map[string]bool{}, map[string]string{}
	for _, r := range book.Sorted() {
		if r.Domain() == "k8s" {
			hidden[r.Name()], hiddenFor[r.Name()] = true, "k8s"
		}
	}
	w := hiddenBetterMatches(book, "pods restarting oom killed", book.Sorted(), hidden, hiddenFor)
	assert.Contains(t, w, "k8s-pod-restarts (needs k8s)")

	assert.Empty(t, hiddenBetterMatches(book, "slow endpoints", book.Sorted(), hidden, hiddenFor),
		"a search whose best match is listed warns about nothing")
}
