package recipes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Recipe sources are declared, synced and pinned rather than discovered on
// every run (design §13a). A sources file says *what* to load; a lock file
// records *which version* `dtctl recipes sync` resolved; a content-addressed
// store holds that content. A run reads only the lock and the store, so the
// recipes an agent sees change only when someone syncs.
const (
	KindSources = "RecipeSources"
	KindLock    = "RecipeLock"
)

// SourceKind is what a source fetches from.
type SourceKind string

const (
	// SourceGit is a GitHub repository at a ref, fetched as an HTTPS archive
	// (no git binary) and pinned to the commit the ref resolved to.
	SourceGit SourceKind = "git"
	// SourceArchive is a .tar.gz at an HTTPS URL, pinned by its SHA-256.
	SourceArchive SourceKind = "archive"
	// SourceApp is the recipe bundles one installed app ships to the current
	// environment, pinned per environment to document versions.
	SourceApp SourceKind = "app"
	// SourceDir is a local directory, read live on every run. It is the
	// authoring loop; a pinned version of the same content is a git source.
	SourceDir SourceKind = "dir"
)

var sourceNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// SourcesFile is a recipe-sources.yaml (user) or .dtctl/recipes.yaml (project).
type SourcesFile struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	// Builtin turns the compiled-in recipes off when false. Nil means on.
	Builtin *bool        `yaml:"builtin,omitempty"`
	Sources []SourceSpec `yaml:"sources"`
}

// SourceSpec is one declared source. Exactly one of Git, Archive, App, Dir is set.
type SourceSpec struct {
	Name string `yaml:"name"`
	// Git is a GitHub repository: "github.com/<owner>/<repo>" or its https URL.
	Git string `yaml:"git,omitempty"`
	// Ref is the branch, tag or commit a git source follows (default: the
	// repository's default branch). Sync pins it to a commit.
	Ref string `yaml:"ref,omitempty"`
	// Path is the recipe root inside a git or archive source (default: the
	// top level). It holds <domain>/<name>.yaml, _domains.yaml, _fragments/.
	Path string `yaml:"path,omitempty"`
	// Archive is an https URL of a .tar.gz.
	Archive string `yaml:"archive,omitempty"`
	// SHA256 optionally pins an archive's bytes up front; without it the
	// first sync records the digest in the lock.
	SHA256 string `yaml:"sha256,omitempty"`
	// App is an app ID whose recipe bundles to load from the environment.
	App string `yaml:"app,omitempty"`
	// Dir is a local directory, relative to the file that declares it.
	Dir string `yaml:"dir,omitempty"`
}

// Kind reports which source kind the spec declares ("" when none or several).
func (s SourceSpec) Kind() SourceKind {
	var kinds []SourceKind
	if s.Git != "" {
		kinds = append(kinds, SourceGit)
	}
	if s.Archive != "" {
		kinds = append(kinds, SourceArchive)
	}
	if s.App != "" {
		kinds = append(kinds, SourceApp)
	}
	if s.Dir != "" {
		kinds = append(kinds, SourceDir)
	}
	if len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

// Location is the human form of where the source points.
func (s SourceSpec) Location() string {
	switch s.Kind() {
	case SourceGit:
		loc := s.Git
		if s.Ref != "" {
			loc += "@" + s.Ref
		}
		if s.Path != "" {
			loc += "//" + s.Path
		}
		return loc
	case SourceArchive:
		if s.Path != "" {
			return s.Archive + "//" + s.Path
		}
		return s.Archive
	case SourceApp:
		return s.App
	case SourceDir:
		return s.Dir
	}
	return ""
}

// Validate checks one spec on its own.
func (s SourceSpec) Validate() error {
	if !sourceNameRe.MatchString(s.Name) {
		return fmt.Errorf("source name %q: lowercase letters, digits and '-'", s.Name)
	}
	kind := s.Kind()
	if kind == "" {
		return fmt.Errorf("source %q: set exactly one of git, archive, app, dir", s.Name)
	}
	if s.Ref != "" && kind != SourceGit {
		return fmt.Errorf("source %q: ref applies to git sources only", s.Name)
	}
	if s.Path != "" && kind != SourceGit && kind != SourceArchive {
		return fmt.Errorf("source %q: path applies to git and archive sources only", s.Name)
	}
	if s.SHA256 != "" && kind != SourceArchive {
		return fmt.Errorf("source %q: sha256 applies to archive sources only", s.Name)
	}
	if s.Path != "" {
		if _, err := cleanRelPath(s.Path); err != nil {
			return fmt.Errorf("source %q: path: %v", s.Name, err)
		}
	}
	switch kind {
	case SourceGit:
		if _, err := ParseGitHubRepo(s.Git); err != nil {
			return fmt.Errorf("source %q: %v", s.Name, err)
		}
	case SourceArchive:
		u, err := url.Parse(s.Archive)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("source %q: archive must be an https URL", s.Name)
		}
		if s.SHA256 != "" && !sha256Re.MatchString(s.SHA256) {
			return fmt.Errorf("source %q: sha256 must be 64 hex characters", s.Name)
		}
	case SourceApp:
		if strings.ContainsAny(s.App, " '\"/\\") {
			return fmt.Errorf("source %q: %q is not an app ID", s.Name, s.App)
		}
	}
	return nil
}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Fingerprint identifies what a spec asks for. A lock entry whose fingerprint
// no longer matches its spec (the ref, path or URL was edited) is resolved
// afresh by the next sync instead of silently serving the old pin.
func (s SourceSpec) Fingerprint() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{string(s.Kind()), s.Git, s.Ref, s.Path, s.Archive, s.SHA256, s.App}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// GitHubRepo is an owner/repo on github.com.
type GitHubRepo struct{ Owner, Repo string }

func (r GitHubRepo) String() string { return "github.com/" + r.Owner + "/" + r.Repo }

var ghPartRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ParseGitHubRepo accepts "github.com/o/r", "https://github.com/o/r" and a
// trailing ".git". Other hosts are refused: fetching a ref needs the host's
// API, and an archive source covers any host that serves a tarball.
func ParseGitHubRepo(s string) (GitHubRepo, error) {
	t := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), "/"), ".git")
	t = strings.TrimPrefix(strings.TrimPrefix(t, "https://"), "http://")
	parts := strings.Split(t, "/")
	if len(parts) != 3 || !strings.EqualFold(parts[0], "github.com") {
		return GitHubRepo{}, fmt.Errorf("git %q: only github.com/<owner>/<repo> is supported (use an archive source for other hosts)", s)
	}
	if !ghPartRe.MatchString(parts[1]) || !ghPartRe.MatchString(parts[2]) || parts[1] == ".." || parts[2] == ".." {
		return GitHubRepo{}, fmt.Errorf("git %q: not a valid owner/repo", s)
	}
	return GitHubRepo{Owner: parts[1], Repo: parts[2]}, nil
}

// ParseSources decodes and validates a sources file.
func ParseSources(data []byte) (*SourcesFile, error) {
	var f SourcesFile
	if len(bytes.TrimSpace(data)) == 0 {
		return &SourcesFile{APIVersion: APIVersion, Kind: KindSources}, nil
	}
	if err := decodeStrict(data, &f); err != nil {
		return nil, err
	}
	if f.APIVersion != APIVersion {
		return nil, fmt.Errorf("apiVersion %q is not supported (want %s)", f.APIVersion, APIVersion)
	}
	if f.Kind != KindSources {
		return nil, fmt.Errorf("kind is %q, want %s", f.Kind, KindSources)
	}
	seen := map[string]bool{}
	for _, s := range f.Sources {
		if err := s.Validate(); err != nil {
			return nil, err
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("source %q is declared twice", s.Name)
		}
		seen[s.Name] = true
	}
	return &f, nil
}

// Marshal encodes a sources file.
func (f *SourcesFile) Marshal() ([]byte, error) {
	f.APIVersion, f.Kind = APIVersion, KindSources
	return yamlMarshal(f)
}

// Find returns the spec named name.
func (f *SourcesFile) Find(name string) *SourceSpec {
	for i := range f.Sources {
		if f.Sources[i].Name == name {
			return &f.Sources[i]
		}
	}
	return nil
}

// BuiltinEnabled reports whether the file leaves the built-in recipes on.
func (f *SourcesFile) BuiltinEnabled() bool { return f == nil || f.Builtin == nil || *f.Builtin }

// LockFile records what sync resolved for each source of one sources file.
type LockFile struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Sources    []LockEntry `yaml:"sources"`
}

// LockEntry pins one source.
type LockEntry struct {
	Name string `yaml:"name"`
	// Spec is the fingerprint of the spec this pin was resolved for.
	Spec string `yaml:"spec"`
	// Commit is the commit a git source's ref resolved to.
	Commit string `yaml:"commit,omitempty"`
	// ArchiveSHA256 is the digest of an archive source's bytes.
	ArchiveSHA256 string `yaml:"archiveSha256,omitempty"`
	// Digest names the stored recipe tree of a git or archive source.
	Digest string `yaml:"digest,omitempty"`
	// Environments pins an app source per environment: each environment
	// holds its own documents, with its own IDs and versions.
	Environments []AppLock `yaml:"environments,omitempty"`
	Synced       time.Time `yaml:"synced"`
}

// AppLock is an app source's pins on one environment.
type AppLock struct {
	Environment string      `yaml:"environment"`
	Bundles     []BundlePin `yaml:"bundles"`
	Synced      time.Time   `yaml:"synced"`
}

// BundlePin is one bundle document at one version.
type BundlePin struct {
	Document string `yaml:"document"`
	Name     string `yaml:"name"`
	Version  int    `yaml:"version"`
	Digest   string `yaml:"digest"`
}

// ParseLock decodes a lock file; empty input is an empty lock.
func ParseLock(data []byte) (*LockFile, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &LockFile{APIVersion: APIVersion, Kind: KindLock}, nil
	}
	var l LockFile
	if err := decodeStrict(data, &l); err != nil {
		return nil, err
	}
	if l.Kind != KindLock {
		return nil, fmt.Errorf("kind is %q, want %s", l.Kind, KindLock)
	}
	return &l, nil
}

// Marshal encodes a lock file.
func (l *LockFile) Marshal() ([]byte, error) {
	l.APIVersion, l.Kind = APIVersion, KindLock
	return yamlMarshal(l)
}

// Find returns the entry for name.
func (l *LockFile) Find(name string) *LockEntry {
	if l == nil {
		return nil
	}
	for i := range l.Sources {
		if l.Sources[i].Name == name {
			return &l.Sources[i]
		}
	}
	return nil
}

// Env returns an app entry's pins for one environment.
func (e *LockEntry) Env(env string) *AppLock {
	if e == nil {
		return nil
	}
	for i := range e.Environments {
		if e.Environments[i].Environment == env {
			return &e.Environments[i]
		}
	}
	return nil
}

// Pin is the short form of an entry's resolved version for one environment.
func (e *LockEntry) Pin(kind SourceKind, env string) string {
	if e == nil {
		return ""
	}
	switch kind {
	case SourceGit:
		return shortHash(e.Commit, 12)
	case SourceArchive:
		return "sha256:" + shortHash(e.ArchiveSHA256, 12)
	case SourceApp:
		a := e.Env(env)
		if a == nil {
			return ""
		}
		parts := make([]string, 0, len(a.Bundles))
		for _, b := range a.Bundles {
			parts = append(parts, fmt.Sprintf("%s@%d", strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(b.Name, "/"), "recipes/"), ".yaml"), b.Version))
		}
		if len(parts) == 0 {
			return "(no bundles)"
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func shortHash(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func yamlMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
