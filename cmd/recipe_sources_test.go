package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/recipes"
)

// fakeGitHub serves one repository whose "main" moves when the test says so.
type fakeGitHub struct {
	head     string
	archives map[string][]byte
	calls    int
}

func (f *fakeGitHub) ResolveGitRef(_ context.Context, _ recipes.GitHubRepo, ref string) (string, error) {
	f.calls++
	if ref != "main" {
		return "", fmt.Errorf("no ref %q", ref)
	}
	return f.head, nil
}

func (f *fakeGitHub) GitArchive(_ context.Context, _ recipes.GitHubRepo, commit string) ([]byte, error) {
	f.calls++
	a, ok := f.archives[commit]
	if !ok {
		return nil, fmt.Errorf("no commit %s", commit)
	}
	return a, nil
}

func (f *fakeGitHub) Archive(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("unused")
}

func recipeRepoArchive(t *testing.T, summary string) []byte {
	t.Helper()
	files := map[string]string{
		"repo-main/recipes/problems/problems-shared.yaml": `apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-shared, version: 1}
spec:
  summary: ` + summary + `
  timeframe: none
  dql: fetch dt.davis.problems | limit 7
  means: m
  emptyMeans: e
`,
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, _ = tw.Write([]byte(content))
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// sourcesSandbox points every piece of recipe host state at temp dirs.
type sourcesSandbox struct {
	config, store, state, work string
	gh                         *fakeGitHub
}

func newSourcesSandbox(t *testing.T) *sourcesSandbox {
	t.Helper()
	sb := &sourcesSandbox{config: t.TempDir(), store: t.TempDir(), state: t.TempDir(), work: t.TempDir()}
	sb.gh = &fakeGitHub{
		head: strings.Repeat("1", 40),
		archives: map[string][]byte{
			strings.Repeat("1", 40): recipeRepoArchive(t, "First"),
			strings.Repeat("2", 40): recipeRepoArchive(t, "Second"),
		},
	}
	saved := []any{recipeSourcesPath, recipeStoreDir, recipeStateDir, recipeWorkDir, recipeUserDir, recipeCacheRoot, recipeRemote}
	recipeSourcesPath = func() string { return filepath.Join(sb.config, userSourcesName) }
	recipeStoreDir = func() string { return sb.store }
	recipeStateDir = func() string { return sb.state }
	recipeWorkDir = func() (string, error) { return sb.work, nil }
	recipeUserDir = func() string { return filepath.Join(sb.config, "recipes") }
	cache := t.TempDir()
	recipeCacheRoot = func() string { return cache }
	recipeRemote = func() recipes.RemoteFetcher { return sb.gh }
	t.Cleanup(func() {
		recipeSourcesPath = saved[0].(func() string)
		recipeStoreDir = saved[1].(func() string)
		recipeStateDir = saved[2].(func() string)
		recipeWorkDir = saved[3].(func() (string, error))
		recipeUserDir = saved[4].(func() string)
		recipeCacheRoot = saved[5].(func() string)
		recipeRemote = saved[6].(func() recipes.RemoteFetcher)
	})
	isolatedConfig(t, "apiVersion: v1\nkind: Config\ncontexts: []\n")
	t.Setenv(recipePathEnv, "")
	t.Setenv("DTCTL_MIN_STABILITY", "")
	t.Cleanup(restorePristineTree)
	return sb
}

func (sb *sourcesSandbox) run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	return captureRun(t, args, RunOptions{})
}

func TestRecipeSourcesGitPinLifecycle(t *testing.T) {
	sb := newSourcesSandbox(t)

	code, _ := sb.run(t, "recipes", "add", "team", "--git", "github.com/o/repo-main", "--ref", "main", "--path", "recipes")
	require.Zero(t, code)
	lock, err := os.ReadFile(filepath.Join(sb.config, userSourcesLockName))
	require.NoError(t, err)
	assert.Contains(t, string(lock), strings.Repeat("1", 40), "add syncs and pins the commit main names")

	code, out := sb.run(t, "run", "problems-shared", "--dry-run", "--plain")
	require.Zero(t, code, out)
	assert.Contains(t, out, "limit 7")

	// Upstream moves. A run never sees it, and makes no request.
	sb.gh.head = strings.Repeat("2", 40)
	sb.gh.calls = 0
	code, out = sb.run(t, "describe", "recipe", "problems-shared", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "First")
	assert.Contains(t, out, "team@111111111111")
	assert.Zero(t, sb.gh.calls, "loading recipes never touches the network")

	code, out = sb.run(t, "recipes", "outdated", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "outdated")

	code, _ = sb.run(t, "recipes", "sync", "--plain")
	require.Zero(t, code)
	code, out = sb.run(t, "describe", "recipe", "problems-shared", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "First", "a plain sync keeps the pin")

	code, _ = sb.run(t, "recipes", "sync", "--update", "team", "--plain")
	require.Zero(t, code)
	code, out = sb.run(t, "describe", "recipe", "problems-shared", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "Second", "--update moves it")

	code, out = sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "team")
	assert.Contains(t, out, "222222222222")

	code, _ = sb.run(t, "recipes", "remove", "team")
	require.Zero(t, code)
	code, _ = sb.run(t, "run", "problems-shared", "--dry-run", "--plain")
	assert.NotZero(t, code, "a removed source contributes nothing")
}

func TestRecipeSourcesMissingStoreSaysSync(t *testing.T) {
	sb := newSourcesSandbox(t)
	code, _ := sb.run(t, "recipes", "add", "team", "--git", "github.com/o/repo-main", "--ref", "main", "--path", "recipes")
	require.Zero(t, code)
	require.NoError(t, os.RemoveAll(sb.store))

	code, out := sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "missing")

	code, _ = sb.run(t, "recipes", "sync")
	require.Zero(t, code)
	code, out = sb.run(t, "run", "problems-shared", "--dry-run", "--plain")
	require.Zero(t, code, "sync restores the locked content: %s", out)
}

func TestProjectRecipeSourcesNeedTrust(t *testing.T) {
	sb := newSourcesSandbox(t)
	recipesDir := filepath.Join(sb.work, "my-recipes", "problems")
	require.NoError(t, os.MkdirAll(recipesDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(recipesDir, "problems-local.yaml"), []byte(`apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-local, version: 1}
spec:
  summary: Local
  timeframe: none
  dql: fetch dt.davis.problems | limit 9
  means: m
  emptyMeans: e
`), 0o600))
	// A cloned repository carries a project sources file.
	require.NoError(t, os.MkdirAll(filepath.Join(sb.work, projectRecipeDir), 0o700))
	project := filepath.Join(sb.work, projectRecipeDir, projectSourcesName)
	require.NoError(t, os.WriteFile(project, []byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nsources:\n  - {name: local, dir: my-recipes}\n"), 0o600))

	code, _ := sb.run(t, "run", "problems-local", "--dry-run", "--plain")
	assert.NotZero(t, code, "an untrusted project file contributes nothing")
	code, out := sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "untrusted")

	code, _ = sb.run(t, "recipes", "sync")
	require.Zero(t, code)
	code, out = sb.run(t, "run", "problems-local", "--dry-run", "--plain")
	require.Zero(t, code, out)
	assert.Contains(t, out, "limit 9")

	// The file changes (a pull, a planted edit): trust lapses until the next sync.
	require.NoError(t, os.WriteFile(project, []byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nsources:\n  - {name: local, dir: my-recipes}\n  - {name: more, dir: elsewhere}\n"), 0o600))
	code, _ = sb.run(t, "run", "problems-local", "--dry-run", "--plain")
	assert.NotZero(t, code)
}

func TestRecipeSourcesBuiltinOff(t *testing.T) {
	sb := newSourcesSandbox(t)
	require.NoError(t, os.WriteFile(filepath.Join(sb.config, userSourcesName), []byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nbuiltin: false\nsources: []\n"), 0o600))
	code, _ := sb.run(t, "run", "problems-active", "--dry-run", "--plain")
	assert.NotZero(t, code, "builtin: false turns the built-in recipes off")
	code, out := sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "off")
}

func TestRecipeSourcesAddRejectsBadSpecs(t *testing.T) {
	sb := newSourcesSandbox(t)
	code, _ := sb.run(t, "recipes", "add", "x", "--git", "gitlab.com/o/r")
	assert.NotZero(t, code)
	code, _ = sb.run(t, "recipes", "add", "x", "--git", "github.com/o/r", "--app", "a")
	assert.NotZero(t, code)
	_, err := os.Stat(filepath.Join(sb.config, userSourcesName))
	assert.True(t, os.IsNotExist(err), "a rejected source writes nothing")
}

// TestProjectEditDoesNotTrustForeignSources: adding a source to an untrusted
// project file must not enable the sources someone else already put there.
func TestProjectEditDoesNotTrustForeignSources(t *testing.T) {
	sb := newSourcesSandbox(t)
	require.NoError(t, os.MkdirAll(filepath.Join(sb.work, projectRecipeDir), 0o700))
	project := filepath.Join(sb.work, projectRecipeDir, projectSourcesName)
	require.NoError(t, os.WriteFile(project, []byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nsources:\n  - {name: planted, dir: planted}\n"), 0o600))

	code, _ := sb.run(t, "recipes", "add", "mine", "--dir", "mine", "--project", "--no-sync")
	require.Zero(t, code)
	code, out := sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "untrusted", "the planted source stays off until a sync shows it")

	code, _ = sb.run(t, "recipes", "remove", "mine", "--project")
	require.Zero(t, code)
	code, out = sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.Contains(t, out, "untrusted")

	// A file this command creates is the user's own and trusted at once.
	require.NoError(t, os.RemoveAll(filepath.Join(sb.work, projectRecipeDir)))
	code, _ = sb.run(t, "recipes", "add", "mine", "--dir", "mine", "--project", "--no-sync")
	require.Zero(t, code)
	code, out = sb.run(t, "get", "recipe-sources", "--plain")
	require.Zero(t, code)
	assert.NotContains(t, out, "untrusted")
}
