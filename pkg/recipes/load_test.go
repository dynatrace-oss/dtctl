package recipes

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fileLayer(l Layer, files map[string]string) FileLayer {
	fsys := fstest.MapFS{}
	for p, c := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(c)}
	}
	return FileLayer{Layer: l, FS: fsys, Root: string(l)}
}

func specWithSummary(summary string) string {
	return "summary: " + summary + "\ntimeframe: 1h\ndql: fetch logs\nmeans: m\nemptyMeans: e\n"
}

func bundleYAML(name string, version int, recipes, extra string) string {
	return "apiVersion: dtctl.dev/v1alpha1\nkind: RecipeBundle\nmetadata:\n  name: " + name +
		"\n  version: " + itoa(version) + "\nspec:\n" + extra + "  recipes:\n" + recipes
}

func bundleRecipeYAML(name, summary string) string {
	return "    - metadata: {name: " + name + ", version: 1}\n      spec:\n" +
		indent(specWithSummary(summary), "        ") + "\n"
}

func itoa(i int) string { return string(rune('0' + i)) }

func appBundle(appID, content string) BundleDoc {
	return BundleDoc{Content: []byte(content), Source: Source{Layer: LayerEnvironment, AppID: appID, Location: "document " + appID}}
}

func TestLayerPrecedenceAndShadows(t *testing.T) {
	l := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(map[string]string{
			"k8s/k8s-a.yaml": recipeYAML("k8s-a", specWithSummary("builtin")),
			"k8s/k8s-b.yaml": recipeYAML("k8s-b", specWithSummary("builtin b")),
		}), Root: "builtin"},
		Bundles: []BundleDoc{appBundle("my.app", bundleYAML("b", 3, bundleRecipeYAML("k8s-a", "app"), ""))},
		Files: []FileLayer{
			fileLayer(LayerOrg, map[string]string{"k8s-a.yaml": recipeYAML("k8s-a", specWithSummary("org"))}),
			fileLayer(LayerUser, map[string]string{"sub/k8s-a.yaml": recipeYAML("k8s-a", specWithSummary("user"))}),
		},
	}
	b := l.Load()
	require.Empty(t, b.Problems)
	a := b.Get("k8s-a")
	assert.Equal(t, "user", a.Spec.Summary)
	var shadows []string
	for _, s := range a.Shadows {
		shadows = append(shadows, s.String())
	}
	assert.Equal(t, []string{"builtin", "app:my.app@3", "org"}, shadows)
	assert.Equal(t, "builtin b", b.Get("k8s-b").Spec.Summary)
}

func TestLaterLayersCannotRedefineRegistries(t *testing.T) {
	b := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
		Files: []FileLayer{fileLayer(LayerUser, map[string]string{
			"_domains.yaml":    "apiVersion: dtctl.dev/v1alpha1\nkind: RecipeDomains\ndomains:\n  k8s: mine\n  team: our team\n",
			"team/team-a.yaml": recipeYAML("team-a", specWithSummary("ok")),
		})},
	}.Load()
	require.Len(t, b.Problems, 1)
	assert.Contains(t, b.Problems[0].Message, `domain "k8s" is already defined`)
	assert.Equal(t, "Kubernetes", b.Domains["k8s"].Description)
	assert.NotNil(t, b.Get("team-a"), "a layer may add a domain and use it")
}

func TestBrokenFileDoesNotBreakTheRest(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/bad.yaml":   "apiVersion: dtctl.dev/v1alpha1\nkind: Recipe\nmetadata: [",
		"k8s/k8s-a.yaml": recipeYAML("k8s-a", validSpec),
		"k8s/old.yaml":   "kind: RecipeBook\nrecipes: []\n", // a foreign kind is ignored silently
	})
	assert.Len(t, b.Problems, 1)
	assert.NotNil(t, b.Get("k8s-a"))
}

func TestBundleConflictsLoadNeither(t *testing.T) {
	b := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
		Bundles: []BundleDoc{
			appBundle("app.one", bundleYAML("one", 1, bundleRecipeYAML("k8s-x", "one")+bundleRecipeYAML("k8s-only-one", "one"), "")),
			appBundle("app.two", bundleYAML("two", 1, bundleRecipeYAML("k8s-x", "two"), "")),
		},
	}.Load()
	assert.Nil(t, b.Get("k8s-x"))
	assert.NotNil(t, b.Get("k8s-only-one"))
	require.Len(t, b.Problems, 1)
	assert.Contains(t, b.Problems[0].Message, `recipe "k8s-x" is defined by app:app.one@1 and app:app.two@1`)
}

func TestBundleCapabilities(t *testing.T) {
	caps := `  capabilities:
    hosts: {entityTypes: [HOST]}
    llm: {dataObject: spans}
    broken: {}
`
	b := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
		Bundles: []BundleDoc{appBundle("app.ai", bundleYAML("ai", 1, bundleRecipeYAML("k8s-y", "y"), caps))},
	}.Load()
	assert.Contains(t, b.Capabilities, "llm")
	assert.NotContains(t, b.Capabilities, "hosts", "a bundle may not redefine a built-in capability")
	assert.NotContains(t, b.Capabilities, "broken")
	assert.Equal(t, "app:app.ai@1", b.CapabilitySources["llm"].String())
	assert.Len(t, b.Problems, 2)
}

func TestBundleMinDtctlVersion(t *testing.T) {
	content := "apiVersion: dtctl.dev/v1alpha1\nkind: RecipeBundle\nmetadata:\n  name: n\n  version: 1\n  minDtctlVersion: 0.50.0\nspec:\n  recipes:\n" + bundleRecipeYAML("k8s-z", "z")
	load := func(v string) *Book {
		return Loader{
			Builtin:      FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
			Bundles:      []BundleDoc{appBundle("a", content)},
			DtctlVersion: v,
		}.Load()
	}
	old := load("0.42.0")
	assert.Nil(t, old.Get("k8s-z"))
	require.Len(t, old.Problems, 1)
	assert.Contains(t, old.Problems[0].Message, "needs dtctl 0.50.0")
	assert.NotNil(t, load("0.50.0").Get("k8s-z"))
	assert.NotNil(t, load("").Get("k8s-z"), "a development build checks nothing")
}

func TestBundleFragmentsArePrivate(t *testing.T) {
	frag := "  fragments: |\n    {{define \"app-base\"}}fetch spans{{end}}\n"
	user := recipeYAML("k8s-u", "summary: u\ntimeframe: 1h\ndql: '{{template \"app-base\"}}'\nmeans: m\nemptyMeans: e\n")
	app := "    - metadata: {name: k8s-app, version: 1}\n      spec:\n" +
		indent("summary: a\ntimeframe: 1h\ndql: '{{template \"app-base\"}} | limit 1'\nmeans: m\nemptyMeans: e", "        ") + "\n"
	b := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
		Bundles: []BundleDoc{appBundle("app", bundleYAML("app", 1, app, frag))},
		Files:   []FileLayer{fileLayer(LayerUser, map[string]string{"u.yaml": user})},
	}.Load()
	r := b.Get("k8s-app")
	require.NotNil(t, r)
	out, err := b.Render(r, Input{Window: mustWindow(t, r, "", "")})
	require.NoError(t, err)
	assert.Equal(t, "fetch spans | limit 1", out.DQL)
	assert.Nil(t, b.Get("k8s-u"), "a bundle's fragments are visible to its own recipes only")
}

type fakeBundles struct {
	refs    []BundleRef
	content map[string]string
	fetched []string
	err     error
}

func (f *fakeBundles) ListBundles(context.Context) ([]BundleRef, error) { return f.refs, f.err }

func (f *fakeBundles) FetchBundle(_ context.Context, id string) ([]byte, error) {
	f.fetched = append(f.fetched, id)
	return []byte(f.content[id]), nil
}

func TestRefreshBundles(t *testing.T) {
	src := &fakeBundles{
		refs: []BundleRef{
			{ID: "d1", Name: "/recipes/ai.yaml", Version: 2, AppID: "app.ai"},
			{ID: "d2", Name: "/recipes/hand-made.yaml", Version: 1}, // no originAppId: untrusted
			{ID: "d3", Name: "/skills/x/SKILL.md", Version: 1, AppID: "app.ai"},
			{ID: "d4", Name: "recipes/costs.yml", Version: 1, AppID: "app.costs"},
		},
		content: map[string]string{"d1": "one", "d4": "four"},
	}
	c, err := RefreshBundles(context.Background(), src, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Untrusted)
	require.Len(t, c.Bundles, 2)
	assert.Equal(t, []string{"d1", "d4"}, src.fetched, "skills and untrusted documents are never downloaded")

	// Unchanged versions are reused; a new version is fetched again.
	src.fetched = nil
	src.refs[0].Version = 3
	src.content["d1"] = "one-v3"
	c2, err := RefreshBundles(context.Background(), src, c, testNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, []string{"d1"}, src.fetched)
	assert.Equal(t, "one-v3", string(c2.Bundles[0].Content))

	docs := c2.Docs()
	assert.Equal(t, "app.ai", docs[0].Source.AppID)
	assert.Equal(t, 3, docs[0].Source.BundleVersion)

	assert.False(t, c2.Stale(testNow.Add(90*time.Minute)))
	assert.True(t, c2.Stale(testNow.Add(2*time.Hour)))
	assert.True(t, (*BundleCache)(nil).Stale(testNow))

	_, err = RefreshBundles(context.Background(), &fakeBundles{err: errors.New("403")}, c2, testNow)
	assert.Error(t, err)
}

func TestDomainNamesCannotContainTheSeparator(t *testing.T) {
	b := Loader{
		Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(nil), Root: "builtin"},
		Files: []FileLayer{fileLayer(LayerUser, map[string]string{
			"_domains.yaml": "apiVersion: dtctl.dev/v1alpha1\nkind: RecipeDomains\ndomains:\n  my-team: nope\n",
		})},
	}.Load()
	require.Len(t, b.Problems, 1)
	assert.Contains(t, b.Problems[0].Message, `domain "my-team"`)
	assert.NotContains(t, b.Domains, "my-team")
}

// TestFragmentUsedInsideWithCounts: a fragment invoked only inside {{with}} counts as used.
func TestFragmentUsedInsideWithCounts(t *testing.T) {
	b := loadBook(t, map[string]string{
		"_fragments/f.tmpl": `{{define "by-name"}}| filter name == {{.}}{{end}}`,
		"k8s/k8s-w.yaml": recipeYAML("k8s-w", `
summary: w
timeframe: 1h
params:
  pod: {type: string}
dql: fetch logs {{with .pod}}{{template "by-name" .}}{{end}}
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	assert.Empty(t, b.Lint(map[string]bool{}))
}
