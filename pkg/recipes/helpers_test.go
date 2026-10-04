package recipes

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

const testDomains = `apiVersion: dtctl.dev/v1alpha1
kind: RecipeDomains
domains:
  k8s: Kubernetes
  services: services
  costs: billing
`

const testScopes = `apiVersion: dtctl.dev/v1alpha1
kind: RecipeScopes
scopes:
  cluster:
    field: k8s.cluster.name
    description: cluster
  namespace:
    field: k8s.namespace.name
    description: namespace
  tag:
    fieldPattern: "primary_tags.{key}"
    form: key=value
    description: primary tag
`

// recipeYAML builds a recipe document; spec is indented YAML under `spec:`.
func recipeYAML(name, spec string) string {
	return "apiVersion: dtctl.dev/v1alpha1\nkind: Recipe\nmetadata:\n  name: " + name +
		"\n  version: 1\n  tags: [test]\nspec:\n" + indent(strings.TrimSpace(spec), "  ") + "\n"
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// builtinFS is a built-in layer with the test registries plus files.
func builtinFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"_domains.yaml": {Data: []byte(testDomains)},
		"_scopes.yaml":  {Data: []byte(testScopes)},
	}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

func loadBook(t *testing.T, files map[string]string) *Book {
	t.Helper()
	return Loader{Builtin: FileLayer{Layer: LayerBuiltin, FS: builtinFS(files), Root: "builtin"}}.Load()
}

// mustRecipe loads one recipe and fails on any load problem.
func mustRecipe(t *testing.T, name, spec string) (*Book, *Recipe) {
	t.Helper()
	b := loadBook(t, map[string]string{strings.SplitN(name, "-", 2)[0] + "/" + name + ".yaml": recipeYAML(name, spec)})
	require.Empty(t, b.Problems)
	r := b.Get(name)
	require.NotNil(t, r)
	return b, r
}

var testNow = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

func mustWindow(t *testing.T, r *Recipe, from, to string) *Window {
	t.Helper()
	w, err := ResolveWindow(r.Spec.Timeframe, from, to, testNow)
	require.NoError(t, err)
	return w
}
