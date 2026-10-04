package recipes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RemoteFetcher reads git and archive sources.
type RemoteFetcher interface {
	// ResolveGitRef returns the commit a ref ("" = default branch) names now.
	ResolveGitRef(ctx context.Context, repo GitHubRepo, ref string) (string, error)
	// GitArchive downloads a repository's .tar.gz at a commit.
	GitArchive(ctx context.Context, repo GitHubRepo, commit string) ([]byte, error)
	// Archive downloads a .tar.gz from an https URL.
	Archive(ctx context.Context, url string) ([]byte, error)
}

// Syncer resolves sources, fetches their content into the store and pins
// what it fetched in a lock.
type Syncer struct {
	Store  Store
	Remote RemoteFetcher
	// Apps reads the current environment's bundle documents; nil when no
	// context is configured (app sources then fail with a clear error).
	Apps BundleSource
	// Environment keys app pins: the environment URL's host.
	Environment string
	Now         func() time.Time
}

// SyncStatus is what sync did with one source.
type SyncStatus string

const (
	SyncUnchanged SyncStatus = "unchanged" // pin kept, content already stored
	SyncRestored  SyncStatus = "restored"  // pin kept, content fetched (new machine, cleaned store)
	SyncPinned    SyncStatus = "pinned"    // first sync of the source
	SyncUpdated   SyncStatus = "updated"   // --update moved the pin
	SyncLive      SyncStatus = "live"      // a dir source: nothing to pin
	SyncRemoved   SyncStatus = "removed"   // a lock entry whose source is gone
	SyncFailed    SyncStatus = "failed"
)

// SyncResult reports one source.
type SyncResult struct {
	Source string     `json:"source"`
	Kind   SourceKind `json:"kind,omitempty"`
	Status SyncStatus `json:"status"`
	From   string     `json:"from,omitempty"`
	To     string     `json:"to,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// SyncOptions selects which pins may move.
type SyncOptions struct {
	// Update re-resolves the named sources (all when UpdateAll).
	Update    map[string]bool
	UpdateAll bool
}

func (o SyncOptions) updates(name string) bool { return o.UpdateAll || o.Update[name] }

// Sync brings the store in line with the lock, resolving sources the lock
// does not cover yet (or that --update names). It returns the new lock and a
// result per source; a failed source keeps its previous entry, so one
// unreachable source never unpins the others.
func (s Syncer) Sync(ctx context.Context, sources []SourceSpec, lock *LockFile, opts SyncOptions) (*LockFile, []SyncResult) {
	next := &LockFile{APIVersion: APIVersion, Kind: KindLock}
	var results []SyncResult
	declared := map[string]bool{}
	for _, spec := range sources {
		declared[spec.Name] = true
		prev := lock.Find(spec.Name)
		if prev != nil && prev.Spec != spec.Fingerprint() {
			// The ref, path, URL or app was edited: the old pin answers a
			// different question, so the source is resolved afresh.
			prev = nil
		}
		entry, res := s.syncOne(ctx, spec, prev, opts.updates(spec.Name))
		results = append(results, res)
		if entry != nil {
			next.Sources = append(next.Sources, *entry)
		}
	}
	for _, e := range lockEntries(lock) {
		if !declared[e.Name] {
			results = append(results, SyncResult{Source: e.Name, Status: SyncRemoved})
		}
	}
	return next, results
}

func lockEntries(l *LockFile) []LockEntry {
	if l == nil {
		return nil
	}
	return l.Sources
}

func (s Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC().Truncate(time.Second)
	}
	return time.Now().UTC().Truncate(time.Second)
}

func (s Syncer) syncOne(ctx context.Context, spec SourceSpec, prev *LockEntry, update bool) (*LockEntry, SyncResult) {
	res := SyncResult{Source: spec.Name, Kind: spec.Kind()}
	fail := func(err error) (*LockEntry, SyncResult) {
		res.Status, res.Error = SyncFailed, err.Error()
		return prev, res
	}
	switch spec.Kind() {
	case SourceDir:
		res.Status = SyncLive
		return nil, res
	case SourceGit:
		entry, status, err := s.syncGit(ctx, spec, prev, update)
		if err != nil {
			return fail(err)
		}
		res.Status, res.To = status, entry.Pin(SourceGit, "")
		if prev != nil && status == SyncUpdated {
			res.From = prev.Pin(SourceGit, "")
		}
		return entry, res
	case SourceArchive:
		entry, status, err := s.syncArchive(ctx, spec, prev, update)
		if err != nil {
			return fail(err)
		}
		res.Status, res.To = status, entry.Pin(SourceArchive, "")
		if prev != nil && status == SyncUpdated {
			res.From = prev.Pin(SourceArchive, "")
		}
		return entry, res
	case SourceApp:
		entry, status, err := s.syncApp(ctx, spec, prev, update)
		if err != nil {
			return fail(err)
		}
		res.Status, res.To = status, entry.Pin(SourceApp, s.Environment)
		if prev != nil && status == SyncUpdated {
			res.From = prev.Pin(SourceApp, s.Environment)
		}
		return entry, res
	}
	return fail(fmt.Errorf("source %q declares no kind", spec.Name))
}

func (s Syncer) syncGit(ctx context.Context, spec SourceSpec, prev *LockEntry, update bool) (*LockEntry, SyncStatus, error) {
	repo, err := ParseGitHubRepo(spec.Git)
	if err != nil {
		return nil, "", err
	}
	if prev != nil && prev.Commit != "" && !update {
		if s.Store.HasTree(prev.Digest) {
			return prev, SyncUnchanged, nil
		}
		digest, err := s.fetchGitTree(ctx, repo, prev.Commit, spec.Path)
		if err != nil {
			return nil, "", err
		}
		if digest != prev.Digest {
			return nil, "", fmt.Errorf("commit %s no longer has the locked content (digest %s, got %s)", shortHash(prev.Commit, 12), shortHash(prev.Digest, 12), shortHash(digest, 12))
		}
		return prev, SyncRestored, nil
	}
	commit, err := s.Remote.ResolveGitRef(ctx, repo, spec.Ref)
	if err != nil {
		return nil, "", err
	}
	if prev != nil && prev.Commit == commit && s.Store.HasTree(prev.Digest) {
		return prev, SyncUnchanged, nil
	}
	digest, err := s.fetchGitTree(ctx, repo, commit, spec.Path)
	if err != nil {
		return nil, "", err
	}
	status := SyncPinned
	if prev != nil {
		status = SyncUpdated
		if prev.Digest == digest {
			status = SyncUnchanged
		}
	}
	return &LockEntry{Name: spec.Name, Spec: spec.Fingerprint(), Commit: commit, Digest: digest, Synced: s.now()}, status, nil
}

func (s Syncer) fetchGitTree(ctx context.Context, repo GitHubRepo, commit, root string) (string, error) {
	data, err := s.Remote.GitArchive(ctx, repo, commit)
	if err != nil {
		return "", err
	}
	files, err := ExtractRecipeTree(bytes.NewReader(data), root, true)
	if err != nil {
		return "", fmt.Errorf("%s@%s: %w", repo, shortHash(commit, 12), err)
	}
	return s.Store.PutTree(files)
}

func (s Syncer) syncArchive(ctx context.Context, spec SourceSpec, prev *LockEntry, update bool) (*LockEntry, SyncStatus, error) {
	if prev != nil && prev.ArchiveSHA256 != "" && !update && s.Store.HasTree(prev.Digest) {
		return prev, SyncUnchanged, nil
	}
	data, err := s.Remote.Archive(ctx, spec.Archive)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	switch {
	case spec.SHA256 != "" && got != spec.SHA256:
		return nil, "", fmt.Errorf("archive sha256 is %s, the source pins %s", got, spec.SHA256)
	case prev != nil && prev.ArchiveSHA256 != "" && !update && got != prev.ArchiveSHA256:
		return nil, "", fmt.Errorf("archive changed since it was locked (sha256 %s, locked %s); run with --update %s to accept it", shortHash(got, 12), shortHash(prev.ArchiveSHA256, 12), spec.Name)
	}
	files, err := ExtractRecipeTree(bytes.NewReader(data), spec.Path, false)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", spec.Archive, err)
	}
	digest, err := s.Store.PutTree(files)
	if err != nil {
		return nil, "", err
	}
	status := SyncPinned
	switch {
	case prev != nil && prev.ArchiveSHA256 == got && !update:
		status = SyncRestored
	case prev != nil && prev.ArchiveSHA256 == got:
		status = SyncUnchanged
	case prev != nil:
		status = SyncUpdated
	}
	return &LockEntry{Name: spec.Name, Spec: spec.Fingerprint(), ArchiveSHA256: got, Digest: digest, Synced: s.now()}, status, nil
}

// syncApp pins the bundles one app ships to the current environment. Pins of
// other environments are kept as they are: syncing against one context never
// moves what another context runs.
func (s Syncer) syncApp(ctx context.Context, spec SourceSpec, prev *LockEntry, update bool) (*LockEntry, SyncStatus, error) {
	if s.Apps == nil || s.Environment == "" {
		return nil, "", fmt.Errorf("app source %q needs a current context (its bundles live in the environment)", spec.Name)
	}
	entry := &LockEntry{Name: spec.Name, Spec: spec.Fingerprint(), Synced: s.now()}
	var pinned *AppLock
	if prev != nil {
		for _, e := range prev.Environments {
			if e.Environment == s.Environment {
				cp := e
				pinned = &cp
				continue
			}
			entry.Environments = append(entry.Environments, e)
		}
	}
	if pinned != nil && !update && s.allBlobsStored(pinned) {
		entry.Environments = append(entry.Environments, *pinned)
		sortEnvs(entry.Environments)
		return entry, SyncUnchanged, nil
	}
	refs, err := s.Apps.ListBundles(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("list recipe bundles: %w", err)
	}
	current := map[string]BundleRef{}
	var ids []string
	for _, r := range refs {
		if r.AppID == spec.App && IsBundleName(r.Name) {
			current[r.ID] = r
			ids = append(ids, r.ID)
		}
	}
	sort.Strings(ids)

	if pinned != nil && !update {
		// Restore the pinned versions. The Document store serves only a
		// document's current content, so a pin the environment has moved
		// past cannot be restored on a machine that never stored it.
		for _, b := range pinned.Bundles {
			if _, ok := s.Store.Blob(b.Digest); ok {
				continue
			}
			cur, ok := current[b.Document]
			if !ok || cur.Version != b.Version {
				return nil, "", fmt.Errorf("pinned bundle %s version %d is no longer served by the environment; run with --update %s to move the pin", b.Name, b.Version, spec.Name)
			}
			data, err := s.Apps.FetchBundle(ctx, b.Document)
			if err != nil {
				return nil, "", err
			}
			if BlobDigest(data) != b.Digest {
				return nil, "", fmt.Errorf("bundle %s version %d no longer has the locked content; run with --update %s", b.Name, b.Version, spec.Name)
			}
			if _, err := s.Store.PutBlob(data); err != nil {
				return nil, "", err
			}
		}
		entry.Environments = append(entry.Environments, *pinned)
		sortEnvs(entry.Environments)
		return entry, SyncRestored, nil
	}

	if len(ids) == 0 {
		return nil, "", fmt.Errorf("app %s ships no recipe bundles to this environment (documents of type %s named recipes/<name>.yaml)", spec.App, BundleDocumentType)
	}
	lock := AppLock{Environment: s.Environment, Synced: s.now()}
	for _, id := range ids {
		ref := current[id]
		data, err := s.Apps.FetchBundle(ctx, id)
		if err != nil {
			return nil, "", fmt.Errorf("download bundle %s: %w", ref.Name, err)
		}
		digest, err := s.Store.PutBlob(data)
		if err != nil {
			return nil, "", err
		}
		lock.Bundles = append(lock.Bundles, BundlePin{Document: id, Name: ref.Name, Version: ref.Version, Digest: digest})
	}
	status := SyncPinned
	if pinned != nil {
		status = SyncUpdated
		if sameBundles(pinned.Bundles, lock.Bundles) {
			status = SyncUnchanged
			lock.Synced = pinned.Synced
		}
	}
	entry.Environments = append(entry.Environments, lock)
	sortEnvs(entry.Environments)
	return entry, status, nil
}

func (s Syncer) allBlobsStored(a *AppLock) bool {
	for _, b := range a.Bundles {
		if _, ok := s.Store.Blob(b.Digest); !ok {
			return false
		}
	}
	return true
}

func sameBundles(a, b []BundlePin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Document != b[i].Document || a[i].Digest != b[i].Digest {
			return false
		}
	}
	return true
}

func sortEnvs(e []AppLock) {
	sort.Slice(e, func(i, j int) bool { return e[i].Environment < e[j].Environment })
}

// OutdatedResult compares a pin with what its source resolves to now.
type OutdatedResult struct {
	Source  string     `json:"source"`
	Kind    SourceKind `json:"kind"`
	Pinned  string     `json:"pinned"`
	Latest  string     `json:"latest,omitempty"`
	Status  string     `json:"status"` // current | outdated | unpinned | live | unknown
	Message string     `json:"message,omitempty"`
}

// Outdated resolves every source without fetching content and reports which
// pins have moved upstream. It changes nothing.
func (s Syncer) Outdated(ctx context.Context, sources []SourceSpec, lock *LockFile) []OutdatedResult {
	var out []OutdatedResult
	for _, spec := range sources {
		r := OutdatedResult{Source: spec.Name, Kind: spec.Kind()}
		entry := lock.Find(spec.Name)
		if entry != nil && entry.Spec != spec.Fingerprint() {
			entry = nil
		}
		r.Pinned = entry.Pin(spec.Kind(), s.Environment)
		switch spec.Kind() {
		case SourceDir:
			r.Status = "live"
		case SourceGit:
			repo, _ := ParseGitHubRepo(spec.Git)
			commit, err := s.Remote.ResolveGitRef(ctx, repo, spec.Ref)
			if err != nil {
				r.Status, r.Message = "unknown", err.Error()
				break
			}
			r.Latest = shortHash(commit, 12)
			r.Status = outdatedStatus(entry != nil && r.Pinned != "", entry != nil && entry.Commit == commit)
		case SourceArchive:
			if spec.SHA256 != "" {
				r.Latest, r.Status = r.Pinned, outdatedStatus(r.Pinned != "", true)
				r.Message = "pinned by sha256 in the source"
				break
			}
			data, err := s.Remote.Archive(ctx, spec.Archive)
			if err != nil {
				r.Status, r.Message = "unknown", err.Error()
				break
			}
			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			r.Latest = "sha256:" + shortHash(got, 12)
			r.Status = outdatedStatus(entry != nil, entry != nil && entry.ArchiveSHA256 == got)
		case SourceApp:
			if s.Apps == nil || s.Environment == "" {
				r.Status, r.Message = "unknown", "needs a current context"
				break
			}
			refs, err := s.Apps.ListBundles(ctx)
			if err != nil {
				r.Status, r.Message = "unknown", err.Error()
				break
			}
			var latest []BundlePin
			for _, ref := range refs {
				if ref.AppID == spec.App && IsBundleName(ref.Name) {
					latest = append(latest, BundlePin{Document: ref.ID, Name: ref.Name, Version: ref.Version})
				}
			}
			sort.Slice(latest, func(i, j int) bool { return latest[i].Document < latest[j].Document })
			r.Latest = (&LockEntry{Environments: []AppLock{{Environment: s.Environment, Bundles: latest}}}).Pin(SourceApp, s.Environment)
			pinned := entry.Env(s.Environment)
			same := pinned != nil && len(pinned.Bundles) == len(latest)
			if same {
				for i := range latest {
					if pinned.Bundles[i].Document != latest[i].Document || pinned.Bundles[i].Version != latest[i].Version {
						same = false
					}
				}
			}
			r.Status = outdatedStatus(pinned != nil, same)
		}
		out = append(out, r)
	}
	return out
}

func outdatedStatus(pinned, same bool) string {
	switch {
	case !pinned:
		return "unpinned"
	case same:
		return "current"
	}
	return "outdated"
}

// EnvironmentKey is how app pins name an environment: its URL's host.
func EnvironmentKey(envURL string) string {
	u := strings.TrimSpace(envURL)
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}
