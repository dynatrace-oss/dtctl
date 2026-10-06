package recipes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validSpec = `
summary: ok
timeframe: 1h
dql: fetch logs | limit 1
means: m
emptyMeans: e
`

func validateErr(t *testing.T, name, spec string) string {
	t.Helper()
	b := loadBook(t, map[string]string{"x/" + name + ".yaml": recipeYAML(name, spec)})
	require.Nil(t, b.Get(name), "an invalid recipe must not load")
	require.Len(t, b.Problems, 1)
	return b.Problems[0].Message
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]struct{ name, spec, want string }{
		"unknown domain": {"nope-x", validSpec, `domain "nope"`},
		"no domain":      {"single", validSpec, "<domain>-<name>"},
		"no emptyMeans": {"k8s-a", `
summary: ok
timeframe: 1h
dql: fetch logs
means: m
`, "emptyMeans is required"},
		"multiline summary": {"k8s-a", `
summary: "two\nlines"
timeframe: 1h
dql: fetch logs
means: m
emptyMeans: e
`, "one line"},
		"no timeframe": {"k8s-a", `
summary: ok
dql: fetch logs
means: m
emptyMeans: e
`, "timeframe is required"},
		"reserved param": {"k8s-a", `
summary: ok
timeframe: 1h
params:
  from: {type: string}
dql: fetch logs
means: m
emptyMeans: e
`, "reserved"},
		"param named like a scope": {"k8s-a", `
summary: ok
timeframe: 1h
params:
  namespace: {type: string}
dql: fetch logs
means: m
emptyMeans: e
`, "reserved"},
		"two positionals": {"k8s-a", `
summary: ok
timeframe: 1h
params:
  a: {type: string, positional: true}
  b: {type: string, positional: true}
dql: fetch logs
means: m
emptyMeans: e
`, "at most one"},
		"undeclared template field": {"k8s-a", `
summary: ok
timeframe: 1h
dql: fetch logs | filter x == {{.service}}
means: m
emptyMeans: e
`, ".service, which is not a declared param"},
		"scope declared but not placed": {"k8s-a", `
summary: ok
timeframe: 1h
scope: [cluster]
dql: fetch logs
means: m
emptyMeans: e
`, "never places .scope"},
		"unknown scope": {"k8s-a", `
summary: ok
timeframe: 1h
scope: [galaxy]
dql: fetch logs {{.scope.stage}}
means: m
emptyMeans: e
`, "unknown dimension"},
		"own from": {"k8s-a", `
summary: ok
timeframe: 1h
dql: |
  fetch logs, from: now()-2h
  | limit 1
means: m
emptyMeans: e
`, "sets its own from:/to:"},
		"inline without window": {"k8s-a", `
summary: ok
timeframe: {default: 1d, inline: true}
dql: fetch logs
means: m
emptyMeans: e
`, "inline timeframe must write"},
		"window without inline": {"k8s-a", `
summary: ok
timeframe: 1d
dql: |
  fetch logs | filter timestamp >= {{.window.from}}
means: m
emptyMeans: e
`, "only an inline timeframe"},
		"unknown fragment": {"k8s-a", `
summary: ok
timeframe: 1h
dql: '{{template "nope"}}'
means: m
emptyMeans: e
`, "not defined"},
		"typo field": {"k8s-a", `
summary: ok
timeframe: 1h
dql: fetch logs
means: m
emptymeans: e
`, "emptymeans"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			assert.Contains(t, validateErr(t, c.name, c.spec), c.want)
		})
	}
}

func TestValidateAcceptsNestedFromInLaterCommands(t *testing.T) {
	// A from: in a later command (a lookup subquery) is not the top-level window.
	mustRecipe(t, "k8s-a", `
summary: ok
timeframe: 1h
dql: |
  fetch logs
  | lookup [fetch spans, from: now()-1h], sourceField: trace_id, lookupField: trace_id
means: m
emptyMeans: e
`)
}

func TestLint(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/k8s-a.yaml": recipeYAML("k8s-a", `
summary: a
timeframe: 1h
requires: [k8s, warp-drive]
dql: fetch logs
means: m
emptyMeans: e
next:
  - recipe: k8s-b
    with: {pod: "x"}
  - recipe: k8s-missing
deprecated:
  replacedBy: k8s-gone
`),
		"k8s/k8s-b.yaml": recipeYAML("k8s-b", validSpec),
	})
	require.Empty(t, b.Problems)
	issues := b.Lint(map[string]bool{"k8s": true})
	var msgs []string
	for _, i := range issues {
		assert.Equal(t, "k8s-a", i.Recipe)
		msgs = append(msgs, i.Message)
	}
	assert.Len(t, msgs, 4, "%v", msgs)
	joined := ""
	for _, m := range msgs {
		joined += m + "\n"
	}
	assert.Contains(t, joined, `"warp-drive"`)
	assert.Contains(t, joined, `"k8s-missing" does not exist`)
	assert.Contains(t, joined, `pod`)
	assert.Contains(t, joined, `k8s-gone`)
}

func TestLintWithTemplateReferencesSourceParams(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/k8s-a.yaml": recipeYAML("k8s-a", `
summary: a
timeframe: 1h
params:
  pod: {type: string}
dql: fetch logs
means: m
emptyMeans: e
next:
  - recipe: k8s-b
    with: {pod: "{{.pod}}"}
  - recipe: k8s-b
    with: {pod: "{{.podd}}"}
`),
		"k8s/k8s-b.yaml": recipeYAML("k8s-b", `
summary: b
timeframe: 1h
params:
  pod: {type: string}
dql: fetch logs
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	issues := b.Lint(map[string]bool{})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, ".podd")
}
