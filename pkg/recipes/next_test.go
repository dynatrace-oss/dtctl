package recipes

import (
	"testing"
	"time"

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

	// Empty: bind edges have no row; the empty edge applies without a window.
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
	// A row value starting with "-" is spelled --service=..., never a bare flag.
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

// TestNextBindsListsAndRowWindows: an array binds to a list param; the follow-up runs over the row's window.
func TestNextBindsListsAndRowWindows(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/k8s-problem.yaml": recipeYAML("k8s-problem", `
summary: p
timeframe: 24h
dql: fetch dt.davis.problems
means: m
emptyMeans: e
next:
  - recipe: k8s-evidence
    bind: {ids: dt.davis.event_ids}
    window: {from: event.start, to: event.end, pad: 5m}
`),
		"k8s/k8s-evidence.yaml": recipeYAML("k8s-evidence", `
summary: e
timeframe: {default: 1h, max: 6h}
params:
  ids: {type: list, required: true}
dql: fetch dt.davis.events | filter in(event.id, {{.ids}})
means: m
emptyMeans: e
`),
	})
	require.Empty(t, b.Problems)
	r := b.Get("k8s-problem")
	rows := []map[string]any{
		{"dt.davis.event_ids": []any{"a,b"}, "event.start": "2026-03-10T10:00:00Z"}, // an id with a comma cannot bind
		{"dt.davis.event_ids": []any{"e-1", "e-2"}, "event.start": "2026-03-10T10:00:00.000000000Z", "event.end": "2026-03-10T11:00:00Z"},
	}
	got := b.NextCommands(r, nil, Carry{From: "24h"}, false, rows)
	require.Len(t, got, 1)
	assert.Equal(t, "dtctl run k8s-evidence --ids=e-1,e-2 --from=2026-03-10T09:55:00Z --to=2026-03-10T11:05:00Z", got[0])

	// Open problem runs to now; a long window is cut at the target's max, keeping the onset.
	open := []map[string]any{{"dt.davis.event_ids": []any{"e-1"}, "event.start": "2026-03-10T10:00:00Z", "event.end": nil}}
	from, to := rowWindow(r.Spec.Next[0].Window, open[0], 0, testNow)
	assert.Equal(t, "2026-03-10T09:55:00Z", from)
	assert.Empty(t, to)
	from, to = rowWindow(r.Spec.Next[0].Window, open[0], 2*time.Hour, testNow)
	assert.Equal(t, "2026-03-10T09:55:00Z", from)
	assert.Equal(t, "2026-03-10T11:55:00Z", to)

	assert.Empty(t, b.Lint(map[string]bool{}))
}

// TestNextLiteralWindowWidensAnEmptyResult: an empty result suggests one wider look-back, once.
func TestNextLiteralWindowWidensAnEmptyResult(t *testing.T) {
	b := loadBook(t, map[string]string{
		"k8s/k8s-problem.yaml": recipeYAML("k8s-problem", `
summary: p
timeframe: {default: 24h, max: 14d}
params:
  id: {type: string, required: true, positional: true}
dql: fetch dt.davis.problems | filter display_id == {{.id}}
means: m
emptyMeans: e
next:
  - recipe: k8s-problem
    with: {id: "{{.id}}"}
    when: empty
    window: {from: 30d}
`),
	})
	require.Empty(t, b.Problems)
	r := b.Get("k8s-problem")
	params := map[string]any{"id": "P-1"}
	got := b.NextCommands(r, params, Carry{}, true, nil)
	assert.Equal(t, []string{"dtctl run k8s-problem P-1 --from=14d"}, got, "cut at the target's max")
	assert.Empty(t, b.NextCommands(r, params, Carry{From: "14d"}, true, nil), "already that wide")
	assert.Empty(t, b.NextCommands(r, params, Carry{}, false, nil), "only on an empty result")
	assert.Empty(t, b.Lint(map[string]bool{}))
}

func TestNextWindowValidation(t *testing.T) {
	for spec, want := range map[string]string{
		"{from: 30d, pad: 5m}":        "a duration from has none",
		"{from: 30d, to: event.end}":  "to must be a duration too",
		"{from: event.start, to: 1d}": "use durations for both",
	} {
		b := loadBook(t, map[string]string{"k8s/k8s-x.yaml": recipeYAML("k8s-x", `
summary: x
timeframe: 1h
dql: fetch logs
means: m
emptyMeans: e
next:
  - recipe: k8s-x
    window: `+spec)})
		require.Len(t, b.Problems, 1, spec)
		assert.Contains(t, b.Problems[0].Message, want, spec)
	}
}
