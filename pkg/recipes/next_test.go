package recipes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nextBook(t *testing.T) *Book {
	t.Helper()
	b := loadBook(t, map[string]string{
		"services/services-red.yaml": recipeYAML("services-red", `
summary: red
timeframe: 2h
scope: [cluster]
params:
  services: {type: list}
dql: |
  fetch spans {{.scope.stage}}
  {{- if .services}} | filter in(dt.service.name, {{.services}}){{end}}
means: m
emptyMeans: e
next:
  - recipe: services-failures
    bind: {service: dt.service.name}
  - recipe: services-list
    when: empty
  - recipe: services-failures
    with: {service: "{{.services}}"}
    when: nonempty
`),
		"services/services-failures.yaml": recipeYAML("services-failures", `
summary: failures
timeframe: 2h
scope: [cluster, namespace]
params:
  service: {type: string, required: true, positional: true}
dql: |
  fetch spans {{.scope.stage}} | filter dt.service.name == {{.service}}
means: m
emptyMeans: e
`),
		"services/services-list.yaml": recipeYAML("services-list", `
summary: list
timeframe: none
dql: smartscapeNodes SERVICE
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	return b
}

func TestNextCommands(t *testing.T) {
	b := nextBook(t)
	r := b.Get("services-red")
	carry := Carry{From: "6h", Scope: map[string][]string{"cluster": {"prod"}}, Segments: []string{"seg-1"}}
	params := map[string]any{"services": []string{"checkout"}}

	got := b.NextCommands(r, params, carry, false, []map[string]any{{"dt.service.name": "cart"}})
	assert.Equal(t, []string{
		"dtctl run services-failures cart --cluster=prod --from=6h --segment=seg-1",
		"dtctl run services-failures checkout --cluster=prod --from=6h --segment=seg-1",
	}, got)

	// Empty: bind edges have no row; the empty edge applies. A state query
	// carries no window.
	got = b.NextCommands(r, params, carry, true, nil)
	assert.Equal(t, []string{"dtctl run services-list --segment=seg-1"}, got)

	// bind skips rows whose bound field is null or empty.
	got = b.NextCommands(r, map[string]any{}, carry, false, []map[string]any{
		{"dt.service.name": nil}, {"dt.service.name": " "}, {"dt.service.name": "cart"},
	})
	assert.Equal(t, []string{"dtctl run services-failures cart --cluster=prod --from=6h --segment=seg-1"}, got)

	// An unset with-param drops the edge rather than emitting a half command.
	got = b.NextCommands(r, map[string]any{}, carry, false, nil)
	assert.Empty(t, got)
}

func TestCommandLineNeverEmitsAValueAsAFlag(t *testing.T) {
	b := nextBook(t)
	target := b.Get("services-failures")
	// A value from a result row that starts with "-" is spelled as
	// --service=..., never left where it would parse as a flag.
	line := CommandLine(target, map[string]string{"service": "--context=other"}, Carry{})
	assert.Equal(t, "dtctl run services-failures --service=--context=other", line)

	line = CommandLine(target, map[string]string{"service": "it's; rm -rf /"}, Carry{})
	assert.Equal(t, `dtctl run services-failures 'it'\''s; rm -rf /'`, line)

	line = CommandLine(target, map[string]string{"namespace": "a b"}, Carry{Scope: map[string][]string{"namespace": {"x"}}})
	assert.Equal(t, `dtctl run services-failures '--namespace=a b'`, line, "an explicit value replaces a carried one")
}

func TestSearch(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/k8s-pod-restarts.yaml": recipeYAML("k8s-pod-restarts", `
summary: Pods whose containers restarted or were OOM-killed
timeframe: 1h
dql: fetch logs
means: m
emptyMeans: e
`),
		"k8s/k8s-clusters.yaml": recipeYAML("k8s-clusters", `
summary: Kubernetes clusters
timeframe: none
dql: fetch logs
means: m
emptyMeans: e
`),
		"services/services-red.yaml": recipeYAML("services-red", `
summary: Request rate, failure rate and latency per service
timeframe: 1h
dql: fetch logs
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	ms := b.Search("which pods restarted", b.Sorted())
	require.NotEmpty(t, ms)
	assert.Equal(t, "k8s-pod-restarts", ms[0].Recipe.Name())

	ms = b.Search("service latency", b.Sorted())
	require.NotEmpty(t, ms)
	assert.Equal(t, "services-red", ms[0].Recipe.Name())

	assert.Empty(t, b.Search("dashboards", b.Sorted()))
}

func TestRequiredArgsShowsOptionalPositional(t *testing.T) {
	b := nextBook(t)
	assert.Equal(t, "<service>", RequiredArgs(b.Get("services-failures")))
	_, r := mustRecipe(t, "k8s-a", `
summary: ok
timeframe: 1h
params:
  pod: {type: string, positional: true}
  top: {type: int, required: true}
dql: fetch logs
means: m
emptyMeans: e
`)
	assert.Equal(t, "[<pod>] --top <top>", RequiredArgs(r))
}

func TestNextEdgeKeepsOptionalUnsetParams(t *testing.T) {
	b := loadBook(t, map[string]string{
		"services/services-slow.yaml": recipeYAML("services-slow", `
summary: slow
timeframe: 1h
scope: [cluster]
params:
  service: {type: string, positional: true}
dql: fetch spans {{.scope.stage}}
means: m
emptyMeans: e
next:
  - recipe: services-list
    when: empty
    with: {contains: "{{.service}}"}
  - recipe: costs-trend
    when: empty
`),
		"services/services-list.yaml": recipeYAML("services-list", `
summary: list
timeframe: 2h
scope: [cluster]
params:
  contains: {type: string, positional: true}
dql: fetch spans {{.scope.stage}}
means: m
emptyMeans: e
`),
		"costs/costs-trend.yaml": recipeYAML("costs-trend", `
summary: trend
timeframe: {default: 30d, min: 7d}
dql: fetch logs
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	carry := Carry{From: "6h", Scope: map[string][]string{"cluster": {"c1"}}}
	got := b.NextCommands(b.Get("services-slow"), map[string]any{}, carry, true, nil)
	assert.Equal(t, []string{
		"dtctl run services-list --cluster=c1 --from=6h",
		"dtctl run costs-trend",
	}, got, "an unset optional with-param is left out; a trend recipe does not inherit a plain window")
}
