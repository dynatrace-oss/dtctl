package recipes

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarGz builds an archive; a value of "->target" makes a symlink.
func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if target, ok := strings.CutPrefix(content, "->"); ok {
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeSymlink, Linkname: target}))
			continue
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

const syncRecipe = `apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata: {name: problems-%s, version: 1}
spec:
  summary: s
  timeframe: none
  dql: fetch dt.davis.problems
  means: m
  emptyMeans: e
`

// fakeRemote serves a GitHub repo whose refs move when the test says so.
type fakeRemote struct {
	refs     map[string]string // ref -> commit
	commits  map[string][]byte // commit -> archive
	archives map[string][]byte // url -> archive
	calls    []string
}

func (f *fakeRemote) ResolveGitRef(_ context.Context, repo GitHubRepo, ref string) (string, error) {
	f.calls = append(f.calls, "resolve "+ref)
	c, ok := f.refs[ref]
	if !ok {
		return "", fmt.Errorf("no ref %q", ref)
	}
	return c, nil
}

func (f *fakeRemote) GitArchive(_ context.Context, repo GitHubRepo, commit string) ([]byte, error) {
	f.calls = append(f.calls, "archive "+commit)
	a, ok := f.commits[commit]
	if !ok {
		return nil, fmt.Errorf("no commit %s", commit)
	}
	return a, nil
}

func (f *fakeRemote) Archive(_ context.Context, url string) ([]byte, error) {
	f.calls = append(f.calls, "get "+url)
	a, ok := f.archives[url]
	if !ok {
		return nil, fmt.Errorf("404")
	}
	return a, nil
}

func commit(n int) string { return strings.Repeat(fmt.Sprint(n%10), 40) }

func repoArchive(t *testing.T, names ...string) []byte {
	files := map[string]string{"repo-x/README.md": "readme", "repo-x/recipes/_domains.yaml": "apiVersion: dtctl.dev/v1alpha1\nkind: Domains\ndomains: {}\n"}
	for _, n := range names {
		files["repo-x/recipes/problems/problems-"+n+".yaml"] = fmt.Sprintf(syncRecipe, n)
	}
	return tarGz(t, files)
}

func TestSyncGitPinsAndOnlyMovesOnUpdate(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	remote := &fakeRemote{
		refs:    map[string]string{"main": commit(1)},
		commits: map[string][]byte{commit(1): repoArchive(t, "a"), commit(2): repoArchive(t, "a", "b")},
	}
	s := Syncer{Store: store, Remote: remote, Now: func() time.Time { return time.Unix(0, 0) }}
	src := []SourceSpec{{Name: "team", Git: "github.com/o/repo-x", Ref: "main", Path: "recipes"}}

	lock, res := s.Sync(context.Background(), src, nil, SyncOptions{})
	require.Equal(t, SyncPinned, res[0].Status, res[0].Error)
	e := lock.Find("team")
	require.Equal(t, commit(1), e.Commit)
	tree, ok := store.Tree(e.Digest)
	require.True(t, ok)
	_, err := fs.Stat(tree, "problems/problems-a.yaml")
	require.NoError(t, err, "the tree is re-rooted at the source's path")
	_, err = fs.Stat(tree, "README.md")
	assert.Error(t, err, "only recipe content is stored")

	// Upstream moves; a plain sync keeps the pin and asks GitHub nothing.
	remote.refs["main"] = commit(2)
	remote.calls = nil
	lock2, res := s.Sync(context.Background(), src, lock, SyncOptions{})
	assert.Equal(t, SyncUnchanged, res[0].Status)
	assert.Equal(t, commit(1), lock2.Find("team").Commit)
	assert.Empty(t, remote.calls, "a pinned, stored source costs no request")

	// --update moves it.
	lock3, res := s.Sync(context.Background(), src, lock2, SyncOptions{UpdateAll: true})
	assert.Equal(t, SyncUpdated, res[0].Status)
	assert.Equal(t, shortHash(commit(1), 12), res[0].From)
	assert.Equal(t, commit(2), lock3.Find("team").Commit)

	// A new machine (empty store) restores exactly the locked commit.
	fresh := Syncer{Store: Store{Dir: t.TempDir()}, Remote: remote}
	remote.calls = nil
	lock4, res := fresh.Sync(context.Background(), src, lock3, SyncOptions{})
	assert.Equal(t, SyncRestored, res[0].Status, res[0].Error)
	assert.Equal(t, lock3.Find("team").Digest, lock4.Find("team").Digest)
	assert.Equal(t, []string{"archive " + commit(2)}, remote.calls, "restoring never re-resolves the ref")

	// Editing the declaration invalidates the pin.
	src[0].Ref = "v1"
	remote.refs["v1"] = commit(1)
	lock5, res := s.Sync(context.Background(), src, lock4, SyncOptions{})
	assert.Equal(t, SyncPinned, res[0].Status)
	assert.Equal(t, commit(1), lock5.Find("team").Commit)
}

func TestSyncFailureKeepsThePreviousPin(t *testing.T) {
	remote := &fakeRemote{refs: map[string]string{"main": commit(1)}, commits: map[string][]byte{commit(1): repoArchive(t, "a")}}
	s := Syncer{Store: Store{Dir: t.TempDir()}, Remote: remote}
	src := []SourceSpec{{Name: "team", Git: "github.com/o/repo-x", Ref: "main", Path: "recipes"}}
	lock, _ := s.Sync(context.Background(), src, nil, SyncOptions{})

	remote.refs = map[string]string{} // GitHub unreachable / ref deleted
	lock2, res := s.Sync(context.Background(), src, lock, SyncOptions{UpdateAll: true})
	assert.Equal(t, SyncFailed, res[0].Status)
	assert.Equal(t, commit(1), lock2.Find("team").Commit, "a failed update never unpins")
}

func TestSyncArchiveVerifiesDigests(t *testing.T) {
	url := "https://example.invalid/r.tar.gz"
	a1 := tarGz(t, map[string]string{"problems/problems-a.yaml": fmt.Sprintf(syncRecipe, "a")})
	remote := &fakeRemote{archives: map[string][]byte{url: a1}}
	s := Syncer{Store: Store{Dir: t.TempDir()}, Remote: remote}

	sum := sha256.Sum256(a1)
	wrong := []SourceSpec{{Name: "r", Archive: url, SHA256: strings.Repeat("0", 64)}}
	_, res := s.Sync(context.Background(), wrong, nil, SyncOptions{})
	assert.Equal(t, SyncFailed, res[0].Status)
	assert.Contains(t, res[0].Error, "sha256")

	src := []SourceSpec{{Name: "r", Archive: url}}
	lock, res := s.Sync(context.Background(), src, nil, SyncOptions{})
	require.Equal(t, SyncPinned, res[0].Status, res[0].Error)
	assert.Equal(t, hex.EncodeToString(sum[:]), lock.Find("r").ArchiveSHA256)

	// The URL now serves different bytes: a restore refuses them.
	remote.archives[url] = tarGz(t, map[string]string{"problems/problems-b.yaml": fmt.Sprintf(syncRecipe, "b")})
	fresh := Syncer{Store: Store{Dir: t.TempDir()}, Remote: remote}
	_, res = fresh.Sync(context.Background(), src, lock, SyncOptions{})
	assert.Equal(t, SyncFailed, res[0].Status)
	assert.Contains(t, res[0].Error, "--update r")

	_, res = fresh.Sync(context.Background(), src, lock, SyncOptions{UpdateAll: true})
	assert.Equal(t, SyncUpdated, res[0].Status)
}

type fakeApps struct {
	refs     []BundleRef
	contents map[string]string
	fetched  []string
}

func (f *fakeApps) ListBundles(context.Context) ([]BundleRef, error) { return f.refs, nil }
func (f *fakeApps) FetchBundle(_ context.Context, id string) ([]byte, error) {
	f.fetched = append(f.fetched, id)
	return []byte(f.contents[id]), nil
}

func TestSyncAppPinsPerEnvironment(t *testing.T) {
	apps := &fakeApps{
		refs: []BundleRef{
			{ID: "d1", Name: "recipes/genai.yaml", Version: 3, AppID: "my.app"},
			{ID: "d2", Name: "recipes/other.yaml", Version: 1, AppID: "other.app"},
			{ID: "d3", Name: "recipes/fake.yaml", Version: 1}, // no originAppId
		},
		contents: map[string]string{"d1": "v3"},
	}
	store := Store{Dir: t.TempDir()}
	src := []SourceSpec{{Name: "genai", App: "my.app"}}
	s := Syncer{Store: store, Remote: &fakeRemote{}, Apps: apps, Environment: "env-a.example.invalid"}

	lock, res := s.Sync(context.Background(), src, nil, SyncOptions{})
	require.Equal(t, SyncPinned, res[0].Status, res[0].Error)
	assert.Equal(t, []string{"d1"}, apps.fetched, "only the named app's bundles are fetched")
	assert.Equal(t, "genai@3", lock.Find("genai").Pin(SourceApp, "env-a.example.invalid"))

	// The app updates its bundle; the pin holds until --update.
	apps.refs[0].Version, apps.contents["d1"] = 4, "v4"
	lock2, res := s.Sync(context.Background(), src, lock, SyncOptions{})
	assert.Equal(t, SyncUnchanged, res[0].Status)
	assert.Equal(t, 3, lock2.Find("genai").Env("env-a.example.invalid").Bundles[0].Version)

	// Another environment gets its own pins; the first one's stay.
	s.Environment = "env-b.example.invalid"
	lock3, res := s.Sync(context.Background(), src, lock2, SyncOptions{})
	require.Equal(t, SyncPinned, res[0].Status, res[0].Error)
	e := lock3.Find("genai")
	assert.Equal(t, 3, e.Env("env-a.example.invalid").Bundles[0].Version)
	assert.Equal(t, 4, e.Env("env-b.example.invalid").Bundles[0].Version)

	// A machine without the v3 content cannot restore a version the
	// environment no longer serves, and says how to move on.
	s.Environment = "env-a.example.invalid"
	s.Store = Store{Dir: t.TempDir()}
	_, res = s.Sync(context.Background(), src, lock3, SyncOptions{})
	assert.Equal(t, SyncFailed, res[0].Status)
	assert.Contains(t, res[0].Error, "--update genai")

	out := s.Outdated(context.Background(), src, lock3)
	assert.Equal(t, "outdated", out[0].Status)
	assert.Equal(t, "genai@3", out[0].Pinned)
	assert.Equal(t, "genai@4", out[0].Latest)
}

func TestSyncAppWithoutContext(t *testing.T) {
	s := Syncer{Store: Store{Dir: t.TempDir()}, Remote: &fakeRemote{}}
	_, res := s.Sync(context.Background(), []SourceSpec{{Name: "genai", App: "my.app"}}, nil, SyncOptions{})
	assert.Equal(t, SyncFailed, res[0].Status)
	assert.Contains(t, res[0].Error, "needs a current context")
}

func TestSyncReportsRemovedSources(t *testing.T) {
	lock := &LockFile{Sources: []LockEntry{{Name: "gone", Commit: commit(1)}}}
	s := Syncer{Store: Store{Dir: t.TempDir()}, Remote: &fakeRemote{}}
	next, res := s.Sync(context.Background(), nil, lock, SyncOptions{})
	assert.Empty(t, next.Sources)
	assert.Equal(t, SyncRemoved, res[0].Status)
}

func TestExtractRecipeTreeRefusesEscapes(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"dotdot":  {"top/recipes/../../evil.yaml": "x"},
		"symlink": {"top/recipes/problems/link.yaml": "->/etc/passwd"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ExtractRecipeTree(bytes.NewReader(tarGz(t, files)), "recipes", true)
			assert.Error(t, err)
		})
	}
	// Content outside the recipe root and non-recipe files are ignored.
	tree, err := ExtractRecipeTree(bytes.NewReader(tarGz(t, map[string]string{
		"top/recipes/k8s/k8s-a.yaml":         "a",
		"top/recipes/_fragments/f.tmpl":      "f",
		"top/recipes/k8s/notes.md":           "n",
		"top/recipes/.github/ci.yaml":        "c",
		"top/other/x.yaml":                   "x",
		"top/recipes/k8s/deeper/nested.tmpl": "t",
	})), "recipes", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"_fragments/f.tmpl", "k8s/k8s-a.yaml"}, sortedKeys(tree))

	_, err = ExtractRecipeTree(bytes.NewReader(tarGz(t, map[string]string{"top/README.md": "r"})), "", true)
	assert.ErrorContains(t, err, "no recipe files")
}

func TestTreeDigestDependsOnContentOnly(t *testing.T) {
	a := map[string][]byte{"x.yaml": []byte("1"), "y.yaml": []byte("2")}
	b := map[string][]byte{"y.yaml": []byte("2"), "x.yaml": []byte("1")}
	assert.Equal(t, TreeDigest(a), TreeDigest(b))
	b["y.yaml"] = []byte("3")
	assert.NotEqual(t, TreeDigest(a), TreeDigest(b))
}

func TestParseSources(t *testing.T) {
	f, err := ParseSources([]byte(`apiVersion: dtctl.dev/v1alpha1
kind: RecipeSources
builtin: false
sources:
  - {name: team, git: github.com/o/r, ref: v1, path: recipes}
  - {name: genai, app: my.app}
  - {name: drafts, dir: ./recipes}
`))
	require.NoError(t, err)
	assert.False(t, f.BuiltinEnabled())
	assert.Equal(t, SourceGit, f.Sources[0].Kind())
	assert.Equal(t, "github.com/o/r@v1//recipes", f.Sources[0].Location())

	for name, body := range map[string]string{
		"two kinds":     `{name: x, git: github.com/o/r, app: a}`,
		"no kind":       `{name: x}`,
		"bad name":      `{name: X_Y, app: a}`,
		"other host":    `{name: x, git: gitlab.com/o/r}`,
		"http archive":  `{name: x, archive: "http://example.invalid/a.tgz"}`,
		"ref on app":    `{name: x, app: a, ref: main}`,
		"escaping path": `{name: x, git: github.com/o/r, path: ../..}`,
		"unknown field": `{name: x, app: a, branch: main}`,
	} {
		_, err := ParseSources([]byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nsources:\n  - " + body + "\n"))
		assert.Error(t, err, name)
	}
	_, err = ParseSources([]byte("apiVersion: dtctl.dev/v1alpha1\nkind: RecipeSources\nsources:\n  - {name: x, app: a}\n  - {name: x, app: b}\n"))
	assert.ErrorContains(t, err, "twice")
}

func TestLockRoundTrip(t *testing.T) {
	l := &LockFile{Sources: []LockEntry{{Name: "team", Spec: "abc", Commit: commit(1), Digest: strings.Repeat("a", 64), Synced: time.Unix(0, 0).UTC()}}}
	data, err := l.Marshal()
	require.NoError(t, err)
	back, err := ParseLock(data)
	require.NoError(t, err)
	assert.Equal(t, l.Sources, back.Sources)
}

func TestHTTPRemoteAgainstFakeGitHub(t *testing.T) {
	archive := repoArchive(t, "a")
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		switch {
		case r.URL.Path == "/repos/o/repo-x/commits/main" && r.Header.Get("Accept") == "application/vnd.github.sha":
			_, _ = w.Write([]byte(commit(7)))
		case r.URL.Path == "/o/repo-x/tar.gz/"+commit(7):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	h := HTTPRemote{APIBase: srv.URL, ArchiveBase: srv.URL, Token: "tok"}
	repo := GitHubRepo{Owner: "o", Repo: "repo-x"}
	c, err := h.ResolveGitRef(context.Background(), repo, "main")
	require.NoError(t, err)
	assert.Equal(t, commit(7), c)
	c, err = h.ResolveGitRef(context.Background(), repo, commit(3))
	require.NoError(t, err)
	assert.Equal(t, commit(3), c, "a full commit hash needs no request")
	data, err := h.GitArchive(context.Background(), repo, commit(7))
	require.NoError(t, err)
	assert.Equal(t, archive, data)
	assert.Equal(t, []string{"Bearer tok", "Bearer tok"}, auth)

	_, err = h.ResolveGitRef(context.Background(), repo, "nope")
	assert.ErrorContains(t, err, "GITHUB_TOKEN")
	_, err = h.Archive(context.Background(), "http://example.invalid/x.tgz")
	assert.ErrorContains(t, err, "https")
}
