package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// recipePathEnv lists org-layer recipe directories, colon-separated. They are
// read live, like a dir source.
const recipePathEnv = "DTCTL_RECIPE_PATH"

// Project recipe sources live in .dtctl/recipes.yaml (with its lock next to
// it), found by walking up from the working directory.
const (
	projectRecipeDir    = ".dtctl"
	projectSourcesName  = "recipes.yaml"
	projectLockName     = "recipes.lock"
	userSourcesName     = "recipe-sources.yaml"
	userSourcesLockName = "recipe-sources.lock"
)

// Where recipe state lives on the host. Variables so tests can point them at
// a temp dir: the XDG paths are resolved once per process.
var (
	recipeUserDir     = func() string { return filepath.Join(config.ConfigDir(), "recipes") }
	recipeSourcesPath = func() string { return filepath.Join(config.ConfigDir(), userSourcesName) }
	recipeStoreDir    = func() string { return filepath.Join(config.DataDir(), "recipes", "store") }
	recipeStateDir    = func() string { return filepath.Join(config.StateDir(), "recipes") }
	recipeCacheRoot   = config.CacheDir
	recipeWorkDir     = os.Getwd
	recipeRemote      = func() recipes.RemoteFetcher {
		return recipes.HTTPRemote{Token: os.Getenv("GITHUB_TOKEN"), UserAgent: "dtctl/" + version.Version}
	}
)

// recipeLoad is the merged book for one invocation and what kept a declared
// source out of it.
type recipeLoad struct {
	book *recipes.Book
	// notes name sources that declared content but contributed none: not
	// synced yet, an untrusted project, an unreadable sources file.
	notes []string
	// enabledApps are the app IDs an app source declares, so the
	// availability hint does not offer what is already on.
	enabledApps map[string]bool
}

// recipeEnvSource resolves the environment a recipe tree is loaded for. The
// recipe tree is built before cobra parses flags, so the attach stage passes
// a source built from the raw argv; loader commands pass their parsed config.
type recipeEnvSource struct {
	cfg *config.Config
	// newClient is called only when something actually needs the API.
	newClient func(*config.Config) (*client.Client, error)
}

// loadRecipeBook assembles the recipe book. It never touches the network on
// the CLI: built-in recipes, the pinned content of declared sources (from the
// local store, as the lock names it), DTCTL_RECIPE_PATH and the user
// directory. What a run sees changes only when someone runs
// `dtctl recipes sync`.
//
// A session-backed invocation reads no host files at all; it loads the
// built-in recipes plus the app bundles the request names (Session.RecipeApps).
func loadRecipeBook(ctx context.Context, src recipeEnvSource) *recipeLoad {
	l, load := recipeLoader(ctx, src)
	load.book = l.Load()
	return load
}

// recipeLoader is the loader loadRecipeBook runs, for a caller that adds a
// layer of its own first (`verify recipe -f`).
func recipeLoader(ctx context.Context, src recipeEnvSource) (recipes.Loader, *recipeLoad) {
	l := recipes.Loader{
		Builtin:      recipes.FileLayer{Layer: recipes.LayerBuiltin, FS: builtinrecipes.FS(), Root: "builtin"},
		DtctlVersion: releaseVersion(),
	}
	load := &recipeLoad{enabledApps: map[string]bool{}}
	if runSession != nil {
		l.Bundles, load.notes = sessionRecipeBundles(ctx, src)
		for _, a := range runSession.RecipeApps {
			id, _, _ := strings.Cut(a, "@")
			load.enabledApps[id] = true
		}
		return l, load
	}

	files := loadRecipeSourceFiles()
	eff, builtin, notes := effectiveRecipeSources(files)
	load.notes = notes
	l.NoBuiltinRecipes = !builtin
	store := recipes.Store{Dir: recipeStoreDir()}
	envKey := recipeEnvironmentKey(src.cfg)
	for _, es := range eff {
		spec := es.Spec
		entry := es.File.Lock.Find(spec.Name)
		if entry != nil && entry.Spec != spec.Fingerprint() {
			entry = nil
		}
		switch spec.Kind() {
		case recipes.SourceDir:
			dir := spec.Dir
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(es.File.Dir, dir)
			}
			l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerOrg, FS: os.DirFS(dir), Root: dir, Name: spec.Name})
		case recipes.SourceGit, recipes.SourceArchive:
			if entry == nil {
				load.notes = append(load.notes, fmt.Sprintf("recipe source %q is not synced (run `dtctl recipes sync`)", spec.Name))
				continue
			}
			tree, ok := store.Tree(entry.Digest)
			if !ok {
				load.notes = append(load.notes, fmt.Sprintf("recipe source %q: pinned content is not in the local store (run `dtctl recipes sync` to restore it)", spec.Name))
				continue
			}
			l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerOrg, FS: tree, Root: spec.Name, Name: spec.Name, Pin: entry.Pin(spec.Kind(), "")})
		case recipes.SourceApp:
			load.enabledApps[spec.App] = true
			if envKey == "" {
				continue
			}
			pins := entry.Env(envKey)
			if pins == nil {
				load.notes = append(load.notes, fmt.Sprintf("recipe source %q is not synced for this environment (run `dtctl recipes sync`)", spec.Name))
				continue
			}
			for _, b := range pins.Bundles {
				data, ok := store.Blob(b.Digest)
				if !ok {
					load.notes = append(load.notes, fmt.Sprintf("recipe source %q: pinned bundle %s is not in the local store (run `dtctl recipes sync` to restore it)", spec.Name, b.Name))
					continue
				}
				l.Bundles = append(l.Bundles, recipes.BundleDoc{Content: data, Source: recipes.Source{
					Layer: recipes.LayerEnvironment, Location: "document " + b.Document,
					AppID: spec.App, BundleVersion: b.Version, Name: spec.Name,
				}})
			}
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv(recipePathEnv)) {
		if dir = strings.TrimSpace(dir); dir != "" {
			l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerOrg, FS: os.DirFS(dir), Root: dir})
		}
	}
	userDir := recipeUserDir()
	l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerUser, FS: os.DirFS(userDir), Root: userDir})
	return l, load
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

// recipeEnvironmentKey names the current context's environment for app pins.
func recipeEnvironmentKey(cfg *config.Config) string {
	if runSession != nil {
		return recipes.EnvironmentKey(runSession.EnvironmentURL)
	}
	if cfg == nil {
		return ""
	}
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return ""
	}
	return recipes.EnvironmentKey(ctx.Environment)
}

// recipeSourceFile is one sources file and its lock.
type recipeSourceFile struct {
	Origin   string // "user" or "project"
	Path     string
	LockPath string
	Dir      string // what a dir source's relative path resolves against
	File     *recipes.SourcesFile
	Lock     *recipes.LockFile
	Err      error
	// Trusted is always true for the user file. A project file is honoured
	// only after `dtctl recipes sync` ran against exactly its current
	// content: recipe text is prompt input for agents, and a cloned
	// repository must not be able to plant it just by being the working
	// directory (the same reasoning that keeps .dtctl.yaml aliases off).
	Trusted bool
}

// loadRecipeSourceFiles reads the user sources file and the project one, if any.
func loadRecipeSourceFiles() []*recipeSourceFile {
	var out []*recipeSourceFile
	user := &recipeSourceFile{
		Origin: "user", Path: recipeSourcesPath(),
		LockPath: filepath.Join(filepath.Dir(recipeSourcesPath()), userSourcesLockName),
		Dir:      filepath.Dir(recipeSourcesPath()), Trusted: true,
	}
	if readRecipeSourceFile(user) {
		out = append(out, user)
	}
	if p := findProjectRecipeSources(); p != "" {
		proj := &recipeSourceFile{
			Origin: "project", Path: p, LockPath: filepath.Join(filepath.Dir(p), projectLockName),
			Dir: filepath.Dir(filepath.Dir(p)),
		}
		if readRecipeSourceFile(proj) {
			proj.Trusted = projectTrusted(proj)
			out = append(out, proj)
		}
	}
	return out
}

// readRecipeSourceFile fills f from disk; false when the file does not exist.
func readRecipeSourceFile(f *recipeSourceFile) bool {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		f.Err = err
		return true
	}
	f.File, f.Err = recipes.ParseSources(data)
	if f.Err != nil {
		return true
	}
	lockData, err := os.ReadFile(f.LockPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.Err = err
		return true
	}
	f.Lock, f.Err = recipes.ParseLock(lockData)
	if f.Err != nil {
		f.Err = fmt.Errorf("%s: %w", f.LockPath, f.Err)
	}
	return true
}

// findProjectRecipeSources walks up from the working directory to the first
// .dtctl/recipes.yaml.
func findProjectRecipeSources() string {
	dir, err := recipeWorkDir()
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, projectRecipeDir, projectSourcesName)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// effectiveSource is a declared source and the file that declared it.
type effectiveSource struct {
	Spec recipes.SourceSpec
	File *recipeSourceFile
}

// effectiveRecipeSources merges the files: project sources after user ones,
// and a project source replaces a user source of the same name. A project
// file's `builtin` setting wins over the user's.
func effectiveRecipeSources(files []*recipeSourceFile) ([]effectiveSource, bool, []string) {
	var out []effectiveSource
	var notes []string
	builtin := true
	index := map[string]int{}
	for _, f := range files {
		switch {
		case f.Err != nil:
			notes = append(notes, fmt.Sprintf("%s recipe sources ignored: %v", f.Origin, f.Err))
			continue
		case !f.Trusted:
			notes = append(notes, fmt.Sprintf("project recipe sources in %s are not trusted yet: run `dtctl recipes sync` there to review and enable them", f.Path))
			continue
		}
		if f.File.Builtin != nil {
			builtin = *f.File.Builtin
		}
		for _, s := range f.File.Sources {
			es := effectiveSource{Spec: s, File: f}
			if i, ok := index[s.Name]; ok {
				out[i] = es
				continue
			}
			index[s.Name] = len(out)
			out = append(out, es)
		}
	}
	return out, builtin, notes
}

// The project trust record maps a project sources file to the digest of its
// content and lock as they were when the user last synced it.
func recipeTrustPath() string { return filepath.Join(recipeStateDir(), "trusted-projects.json") }

func projectTrustDigest(f *recipeSourceFile) string {
	src, _ := os.ReadFile(f.Path)
	lock, _ := os.ReadFile(f.LockPath)
	h := sha256.New()
	h.Write(src)
	h.Write([]byte{0})
	h.Write(lock)
	return hex.EncodeToString(h.Sum(nil))
}

func readRecipeTrust() map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile(recipeTrustPath())
	if err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func projectTrusted(f *recipeSourceFile) bool {
	abs, err := filepath.Abs(f.Path)
	if err != nil {
		return false
	}
	return readRecipeTrust()[abs] == projectTrustDigest(f)
}

func trustProject(f *recipeSourceFile) error {
	abs, err := filepath.Abs(f.Path)
	if err != nil {
		return err
	}
	m := readRecipeTrust()
	m[abs] = projectTrustDigest(f)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(recipeTrustPath(), data)
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

// recipeAvailability is a cached listing of the apps on an environment that
// ship recipe bundles. It only feeds a hint ("this app ships recipes: add
// it"); nothing loads from it.
type recipeAvailability struct {
	Refreshed time.Time      `json:"refreshed"`
	Apps      map[string]int `json:"apps"` // app ID -> bundle documents
	Untrusted int            `json:"untrusted,omitempty"`
}

// availableRecipeApps returns the cached listing, refreshing it when it is
// older than recipes.BundleRefreshInterval and refresh is allowed. Any
// failure yields nil: a hint must never break a command.
func availableRecipeApps(ctx context.Context, src recipeEnvSource, refresh bool) *recipeAvailability {
	if src.cfg == nil || runSession != nil {
		return nil
	}
	key, ok := bundleCacheKey(src.cfg)
	if !ok {
		return nil
	}
	path := filepath.Join(recipeCacheRoot(), "recipes", "available-"+safeFileName(key)+".json")
	var cached *recipeAvailability
	if data, err := os.ReadFile(path); err == nil {
		var a recipeAvailability
		if json.Unmarshal(data, &a) == nil {
			cached = &a
		}
	}
	if !refresh || (cached != nil && time.Since(cached.Refreshed) < recipes.BundleRefreshInterval) {
		return cached
	}
	c, err := src.newClient(src.cfg)
	if err != nil {
		return cached
	}
	refs, err := documentBundleSource{c}.ListBundles(ctx)
	if err != nil {
		return cached
	}
	a := &recipeAvailability{Refreshed: time.Now(), Apps: map[string]int{}}
	for _, r := range refs {
		switch {
		case !recipes.IsBundleName(r.Name):
		case r.AppID == "":
			a.Untrusted++
		default:
			a.Apps[r.AppID]++
		}
	}
	if data, err := json.Marshal(a); err == nil {
		_ = writeFileAtomic(path, data)
	}
	return a
}

// availableAppsHint names apps that ship recipes this machine has not
// enabled, with the command that enables each.
func availableAppsHint(a *recipeAvailability, enabled map[string]bool) string {
	if a == nil {
		return ""
	}
	var apps []string
	for id := range a.Apps {
		if !enabled[id] {
			apps = append(apps, id)
		}
	}
	if len(apps) == 0 {
		return ""
	}
	sort.Strings(apps)
	lines := []string{fmt.Sprintf("%d app(s) on this environment ship recipes that are not enabled:", len(apps))}
	for _, id := range apps {
		lines = append(lines, fmt.Sprintf("  dtctl recipes add %s --app %s", suggestSourceName(id), id))
	}
	return strings.Join(lines, "\n")
}

// suggestSourceName derives a source name from an app ID
// ("my.company.genai-observability" -> "genai-observability").
func suggestSourceName(appID string) string {
	n := appID
	if i := strings.LastIndexByte(n, '.'); i >= 0 && i < len(n)-1 {
		n = n[i+1:]
	}
	n = strings.ToLower(n)
	n = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, n)
	n = strings.Trim(n, "-")
	if n == "" {
		n = "app"
	}
	if len(n) > 63 {
		n = n[:63]
	}
	return n
}

// bundleCacheKey identifies the environment a cache belongs to.
func bundleCacheKey(cfg *config.Config) (string, bool) {
	if cfg.CurrentContext == "" {
		return "", false
	}
	if _, err := cfg.CurrentContextObj(); err != nil {
		return "", false
	}
	return cfg.CurrentContext, true
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

// sessionRecipeBundles loads the app bundles a session names. Each entry is
// "<app-id>" (whatever the environment serves) or "<app-id>@<version>"
// (only a bundle document at exactly that version: the pin a host keeps in
// its own configuration). Listings are cached in memory per environment and
// principal for a minute, contents per document version for the process.
func sessionRecipeBundles(ctx context.Context, src recipeEnvSource) ([]recipes.BundleDoc, []string) {
	if len(runSession.RecipeApps) == 0 || src.cfg == nil {
		return nil, nil
	}
	c, err := src.newClient(src.cfg)
	if err != nil {
		return nil, []string{fmt.Sprintf("app recipe bundles not loaded: %v", err)}
	}
	ds := documentBundleSource{c}
	sum := sha256.Sum256([]byte(runSession.EnvironmentURL + "\x00" + runSession.Token))
	key := hex.EncodeToString(sum[:16])
	refs, err := sessionBundles.list(ctx, key, ds)
	if err != nil {
		return nil, []string{fmt.Sprintf("app recipe bundles not loaded: %v", compactErr(err))}
	}
	var docs []recipes.BundleDoc
	var notes []string
	for _, entry := range runSession.RecipeApps {
		app, ver, pinned := strings.Cut(entry, "@")
		want, _ := strconv.Atoi(ver)
		found := 0
		for _, r := range refs {
			if r.AppID != app || !recipes.IsBundleName(r.Name) {
				continue
			}
			found++
			if pinned && r.Version != want {
				notes = append(notes, fmt.Sprintf("app %s bundle %s is at version %d, the request pins %d; skipped", app, r.Name, r.Version, want))
				continue
			}
			data, err := sessionBundles.content(ctx, key, ds, r)
			if err != nil {
				notes = append(notes, fmt.Sprintf("app %s bundle %s: %v", app, r.Name, compactErr(err)))
				continue
			}
			docs = append(docs, recipes.BundleDoc{Content: data, Source: recipes.Source{
				Layer: recipes.LayerEnvironment, Location: "document " + r.ID, AppID: app, BundleVersion: r.Version,
			}})
		}
		if found == 0 {
			notes = append(notes, fmt.Sprintf("app %s ships no recipe bundles to this environment", app))
		}
	}
	return docs, notes
}

type sessionBundleCache struct {
	mu       sync.Mutex
	listings map[string]sessionListing
	contents map[string][]byte // "<doc id>@<version>"
}

type sessionListing struct {
	at   time.Time
	refs []recipes.BundleRef
}

var sessionBundles = &sessionBundleCache{}

const sessionListingTTL = time.Minute

func (s *sessionBundleCache) list(ctx context.Context, key string, ds documentBundleSource) ([]recipes.BundleRef, error) {
	s.mu.Lock()
	l, ok := s.listings[key]
	s.mu.Unlock()
	if ok && time.Since(l.at) < sessionListingTTL {
		return l.refs, nil
	}
	refs, err := ds.ListBundles(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listings == nil || len(s.listings) > 1024 {
		s.listings = map[string]sessionListing{}
	}
	s.listings[key] = sessionListing{at: time.Now(), refs: refs}
	return refs, nil
}

func (s *sessionBundleCache) content(ctx context.Context, principal string, ds documentBundleSource, r recipes.BundleRef) ([]byte, error) {
	// Keyed by environment and principal as well as document version: a
	// document ID is no secret and need not be unique across tenants, so a
	// key without the principal would let one tenant's request be served
	// content another tenant's request fetched.
	key := fmt.Sprintf("%s\x00%s@%d", principal, r.ID, r.Version)
	s.mu.Lock()
	data, ok := s.contents[key]
	s.mu.Unlock()
	if ok {
		return data, nil
	}
	data, err := ds.FetchBundle(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.contents == nil || len(s.contents) > 1024 {
		s.contents = map[string][]byte{}
	}
	s.contents[key] = data
	return data, nil
}

func compactErr(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

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
