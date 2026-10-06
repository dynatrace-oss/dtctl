package recipes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// BundleDocumentType is the Document-store type apps ship agent skills as;
// a recipe bundle is one more agent resource, named under /recipes/.
const BundleDocumentType = "ai-agent-resource"

// IsBundleName reports whether an agent-resource name is "/recipes/<name>.yaml" (leading slash optional).
func IsBundleName(name string) bool {
	n := strings.TrimPrefix(name, "/")
	return strings.HasPrefix(n, "recipes/") && (strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml"))
}

// BundleRefreshInterval is how long a bundle listing is reused.
const BundleRefreshInterval = time.Hour

// BundleRef is one bundle document as the listing reports it.
type BundleRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	// AppID is the originAppId; bundles without one are never loaded, since only
	// app-deployed content is trusted as agent input.
	AppID string `json:"appId,omitempty"`
}

// BundleSource reads bundle documents from an environment.
type BundleSource interface {
	ListBundles(ctx context.Context) ([]BundleRef, error)
	FetchBundle(ctx context.Context, id string) ([]byte, error)
}

// BundleCache is the environment layer's cached state for one environment.
type BundleCache struct {
	Refreshed time.Time      `json:"refreshed"`
	Bundles   []CachedBundle `json:"bundles"`
	// Untrusted counts bundle documents ignored for lacking an originAppId.
	Untrusted int `json:"untrusted,omitempty"`
}

// CachedBundle is a downloaded bundle, keyed by document ID and version.
type CachedBundle struct {
	BundleRef
	Content []byte `json:"content"`
}

// Docs turns the cache into loader input.
func (c *BundleCache) Docs() []BundleDoc {
	if c == nil {
		return nil
	}
	out := make([]BundleDoc, 0, len(c.Bundles))
	for _, b := range c.Bundles {
		out = append(out, BundleDoc{
			Content: b.Content,
			Source: Source{
				Layer:         LayerEnvironment,
				Location:      "document " + b.ID,
				AppID:         b.AppID,
				BundleVersion: b.Version,
			},
		})
	}
	return out
}

// Stale reports whether the listing is older than the refresh interval.
func (c *BundleCache) Stale(now time.Time) bool {
	return c == nil || now.Sub(c.Refreshed) >= BundleRefreshInterval
}

// RefreshBundles lists bundle documents and downloads trusted new or changed ones;
// unchanged bundles are reused from prev.
func RefreshBundles(ctx context.Context, src BundleSource, prev *BundleCache, now time.Time) (*BundleCache, error) {
	refs, err := src.ListBundles(ctx)
	if err != nil {
		return nil, err
	}
	have := map[string]CachedBundle{}
	if prev != nil {
		for _, b := range prev.Bundles {
			have[b.ID] = b
		}
	}
	next := &BundleCache{Refreshed: now}
	for _, ref := range refs {
		if !IsBundleName(ref.Name) {
			continue
		}
		if ref.AppID == "" {
			next.Untrusted++
			continue
		}
		if cached, ok := have[ref.ID]; ok && cached.Version == ref.Version && len(cached.Content) > 0 {
			cached.BundleRef = ref
			next.Bundles = append(next.Bundles, cached)
			continue
		}
		content, err := src.FetchBundle(ctx, ref.ID)
		if err != nil {
			return nil, fmt.Errorf("download bundle %s: %w", ref.ID, err)
		}
		next.Bundles = append(next.Bundles, CachedBundle{BundleRef: ref, Content: content})
	}
	sort.Slice(next.Bundles, func(i, j int) bool { return next.Bundles[i].ID < next.Bundles[j].ID })
	return next, nil
}
