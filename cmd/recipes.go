package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/pkg/version"
	builtinrecipes "github.com/dynatrace-oss/dtctl/recipes"
	sdkdocument "github.com/dynatrace-oss/dtctl/sdk/api/document"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// recipePathEnv lists org-layer recipe directories, colon-separated.
const recipePathEnv = "DTCTL_RECIPE_PATH"

// Where recipe state lives on the host. Variables so tests can point them at
// a temp dir: the XDG paths are resolved once per process.
var (
	recipeUserDir   = func() string { return filepath.Join(config.ConfigDir(), "recipes") }
	recipeCacheRoot = config.CacheDir
)

// envLayerMode says how much network the environment layer (app-shipped
// bundles from the Document store) may use while loading.
type envLayerMode int

const (
	// envCacheOnly reads the cached bundles and never calls the API: shell
	// completion and --help must not wait on the network.
	envCacheOnly envLayerMode = iota
	// envRefreshIfStale lists bundles when the cached listing is older than
	// recipes.BundleRefreshInterval.
	envRefreshIfStale
	// envForceRefresh lists bundles now (`get recipes --refresh`, or a `run`
	// target missing from the cache: an app installed a minute ago works on
	// the first try).
	envForceRefresh
)

// envLayerStatus reports what the environment layer contributed, so a listing
// can say why app recipes are missing rather than leave the caller guessing.
type envLayerStatus struct {
	// State is "fresh" (listed now), "cached" (listing younger than the
	// refresh interval), "stale" (refresh failed; older cache used),
	// "skipped" (no cache and the listing failed), or "off" (no environment).
	State     string
	Refreshed time.Time
	Note      string
	Bundles   int
	Untrusted int
}

// recipeLoad is the merged book for one invocation and how it was assembled.
type recipeLoad struct {
	book *recipes.Book
	env  envLayerStatus
}

// recipeEnvSource resolves the environment a recipe tree is loaded for. The
// recipe tree is built before cobra parses flags, so the attach stage passes
// a source built from the raw argv; loader commands pass their parsed config.
type recipeEnvSource struct {
	cfg *config.Config
	// newClient is called only when the layer actually needs the API.
	newClient func(*config.Config) (*client.Client, error)
}

// loadRecipeBook assembles the recipe book from every layer: built-in, the
// environment's app bundles, then the org (DTCTL_RECIPE_PATH) and user
// directories. A session-backed invocation reads neither directory — they are
// host state, like aliases — and caches bundles in memory only.
func loadRecipeBook(ctx context.Context, src recipeEnvSource, mode envLayerMode) *recipeLoad {
	l, status := recipeLoader(ctx, src, mode)
	return &recipeLoad{book: l.Load(), env: status}
}

// recipeLoader is the loader loadRecipeBook runs, for a caller that adds a
// layer of its own first (`verify recipe -f`).
func recipeLoader(ctx context.Context, src recipeEnvSource, mode envLayerMode) (recipes.Loader, envLayerStatus) {
	l := recipes.Loader{
		Builtin:      recipes.FileLayer{Layer: recipes.LayerBuiltin, FS: builtinrecipes.FS(), Root: "builtin"},
		DtctlVersion: releaseVersion(),
	}
	if runSession == nil {
		for _, dir := range filepath.SplitList(os.Getenv(recipePathEnv)) {
			if dir = strings.TrimSpace(dir); dir != "" {
				l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerOrg, FS: os.DirFS(dir), Root: dir})
			}
		}
		userDir := recipeUserDir()
		l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerUser, FS: os.DirFS(userDir), Root: userDir})
	}
	cache, status := loadEnvLayer(ctx, src, mode)
	l.Bundles = cache.Docs()
	return l, status
}

// releaseVersion is the version bundles' minDtctlVersion is checked against;
// a development build checks nothing.
func releaseVersion() string {
	v := version.Version
	if v == "" || v == "dev" || strings.Contains(v, "dirty") {
		return ""
	}
	return v
}

// loadEnvLayer returns the environment's cached bundles, refreshing them as
// the mode allows. Failure never spreads: a failed listing falls back to the
// stale cache, or skips the layer with a note naming the cause.
func loadEnvLayer(ctx context.Context, src recipeEnvSource, mode envLayerMode) (*recipes.BundleCache, envLayerStatus) {
	if src.cfg == nil {
		return nil, envLayerStatus{State: "off"}
	}
	key, ok := bundleCacheKey(src.cfg)
	if !ok {
		return nil, envLayerStatus{State: "off"}
	}
	store := bundleStoreFor()
	cached := store.load(key)
	now := time.Now()
	status := func(c *recipes.BundleCache, state, note string) envLayerStatus {
		s := envLayerStatus{State: state, Note: note}
		if c != nil {
			s.Refreshed, s.Bundles, s.Untrusted = c.Refreshed, len(c.Bundles), c.Untrusted
		}
		return s
	}
	refresh := mode == envForceRefresh || (mode == envRefreshIfStale && cached.Stale(now))
	if !refresh {
		if cached == nil {
			return nil, status(nil, "skipped", "app recipe bundles not loaded yet (run `dtctl get recipes` to load them)")
		}
		return cached, status(cached, "cached", "")
	}
	c, err := src.newClient(src.cfg)
	if err == nil {
		var fresh *recipes.BundleCache
		fresh, err = recipes.RefreshBundles(ctx, documentBundleSource{c}, cached, now)
		if err == nil {
			store.save(key, fresh)
			return fresh, status(fresh, "fresh", "")
		}
	}
	note := fmt.Sprintf("could not list app recipe bundles: %v", compactErr(err))
	if cached != nil {
		return cached, status(cached, "stale", note+"; using the cached bundles")
	}
	return nil, status(nil, "skipped", note)
}

func compactErr(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// documentBundleSource lists and downloads bundle documents.
type documentBundleSource struct{ c *client.Client }

func (d documentBundleSource) handler() *sdkdocument.Handler {
	return sdkdocument.NewHandler(httpclient.Wrap(d.c.HTTP()))
}

func (d documentBundleSource) ListBundles(ctx context.Context) ([]recipes.BundleRef, error) {
	// Agent resources are skills as well as bundles; the name tells them
	// apart (recipes.IsBundleName), the filter only narrows the listing.
	list, err := d.handler().List(ctx, sdkdocument.DocumentFilters{
		Filter:    fmt.Sprintf("type=='%s' and name contains 'recipes/'", recipes.BundleDocumentType),
		ChunkSize: 100,
	})
	if err != nil {
		return nil, err
	}
	refs := make([]recipes.BundleRef, 0, len(list.Documents))
	for _, m := range list.Documents {
		refs = append(refs, recipes.BundleRef{ID: m.ID, Name: m.Name, Version: m.Version, AppID: m.OriginAppID})
	}
	return refs, nil
}

func (d documentBundleSource) FetchBundle(ctx context.Context, id string) ([]byte, error) {
	return d.handler().GetContent(ctx, id)
}

// bundleStore persists the environment layer's cache.
type bundleStore interface {
	load(key string) *recipes.BundleCache
	save(key string, c *recipes.BundleCache)
}

// bundleCacheKey identifies the environment (and, in a session, the
// principal) a cache belongs to.
func bundleCacheKey(cfg *config.Config) (string, bool) {
	if runSession != nil {
		sum := sha256.Sum256([]byte(runSession.EnvironmentURL + "\x00" + runSession.Token))
		return hex.EncodeToString(sum[:16]), true
	}
	if cfg.CurrentContext == "" {
		return "", false
	}
	if _, err := cfg.CurrentContextObj(); err != nil {
		return "", false
	}
	return cfg.CurrentContext, true
}

// bundleStoreFor picks the cache: a file per context for the CLI, process
// memory for a session — a cached bundle must never land on a service's disk
// (Embedding Invariants §2), and a tenant's bundles never serve another.
func bundleStoreFor() bundleStore {
	if runSession != nil {
		return sessionBundleCache
	}
	return fileBundleStore{dir: filepath.Join(recipeCacheRoot(), "recipes")}
}

type fileBundleStore struct{ dir string }

func (f fileBundleStore) path(key string) string {
	return filepath.Join(f.dir, safeFileName(key)+".json")
}

func (f fileBundleStore) load(key string) *recipes.BundleCache {
	data, err := os.ReadFile(f.path(key))
	if err != nil {
		return nil
	}
	var c recipes.BundleCache
	if json.Unmarshal(data, &c) != nil {
		return nil
	}
	return &c
}

func (f fileBundleStore) save(key string, c *recipes.BundleCache) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	if os.MkdirAll(f.dir, 0o700) != nil {
		return
	}
	tmp := f.path(key) + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, f.path(key))
	}
}

// safeFileName maps a context name to a file name: context names are
// free-form, and one with a slash must not escape the cache directory.
func safeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
}

type memoryBundleStore struct {
	mu sync.Mutex
	m  map[string]*recipes.BundleCache
}

func (s *memoryBundleStore) load(key string) *recipes.BundleCache {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key]
}

func (s *memoryBundleStore) save(key string, c *recipes.BundleCache) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]*recipes.BundleCache{}
	}
	s.m[key] = c
}

var sessionBundleCache = &memoryBundleStore{}

// parsedRecipeEnv is the env source for a command whose flags are parsed.
func parsedRecipeEnv() recipeEnvSource {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = nil
	}
	return recipeEnvSource{cfg: cfg, newClient: NewClientFromConfig}
}

// rawRecipeEnv is the env source for the attach stage, which runs before
// cobra parses --context/--config.
func rawRecipeEnv(args []string) recipeEnvSource {
	return recipeEnvSource{cfg: configForArgs(args), newClient: NewClientFromConfig}
}
