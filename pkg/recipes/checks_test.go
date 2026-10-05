package recipes

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const checkedRecipe = `
summary: token usage
timeframe: 2h
dql: |
  fetch spans
  | dedup {trace.id, n}
  | summarize total = sum(n)
means: m
emptyMeans: e
checks:
  - match: ['\bsum\(n\)']
    unless: ['\bdedup\b']
    warn: sums n twice per trace; dedup first.
    example: fetch spans | summarize sum(n)
`

func TestCheckFiresUnlessAndIgnoresComments(t *testing.T) {
	b, r := mustRecipe(t, "costs-tokens", checkedRecipe)
	c := &r.Spec.Checks[0]
	assert.True(t, c.Fires("fetch spans | summarize sum(n)"))
	assert.False(t, c.Fires("fetch spans | dedup {trace.id} | summarize sum(n)"), "unless suppresses it")
	assert.False(t, c.Fires("fetch spans // sum(n)\n| limit 1"), "a comment is not the query")
	assert.Empty(t, b.lintChecks(r), "fires on its example, not on the recipe's own DQL")

	hits := b.QueryChecks("fetch spans | summarize sum(n)", b.Sorted())
	require.Len(t, hits, 1)
	assert.Equal(t, "costs-tokens", hits[0].Recipe.Name())
	assert.Equal(t, "sums n twice per trace; dedup first.", hits[0].Warn)
}

func TestCheckValidationAndLint(t *testing.T) {
	b := loadBook(t, map[string]string{"costs/costs-bad.yaml": recipeYAML("costs-bad", `
summary: bad
timeframe: 2h
dql: fetch spans | summarize sum(n)
means: m
emptyMeans: e
checks:
  - match: ['(']
`)})
	msgs := ""
	for _, p := range b.Problems {
		msgs += p.String() + "\n"
	}
	assert.Contains(t, msgs, "spec.checks[0]")
	assert.Contains(t, msgs, "warn is required")
	assert.Contains(t, msgs, "example is required")

	// A check that fires on the recipe's own statement warns about the
	// right way to do it: the lint says so.
	b, r := mustRecipe(t, "costs-tokens", strings.Replace(checkedRecipe, `unless: ['\bdedup\b']`, `unless: ['nothing']`, 1))
	assert.Contains(t, strings.Join(b.lintChecks(r), "\n"), "checks[0] fires on the recipe's own DQL")
}

func TestLintQueryAppliesNoStyleLints(t *testing.T) {
	got := LintQuery("fetch spans | summarize count()", 0)
	assert.Empty(t, got, "an unaliased aggregate is style, not a trap, in an ad-hoc query")
	got = LintQuery("fetch logs, samplingRatio: 10 | limit 1", 0)
	assert.Empty(t, got, "logs honour samplingRatio")
	got = LintQuery("fetch events, samplingRatio: 10 | limit 1", 0)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "samplingRatio on fetch events")
}

const boundRecipe = `
summary: one problem's events
timeframe: {default: 1d, max: 7d}
scope: [cluster]
params:
  id: {type: string, required: true, positional: true, pattern: '^P-\d+$'}
dql: |
  fetch dt.davis.events {{.scope.stage}}
  | filter display_id == {{.id}}
means: m
emptyMeans: e
`

func TestBindQueryReadsParamsScopeAndWindow(t *testing.T) {
	b, r := mustRecipe(t, "costs-events", boundRecipe)
	q := `fetch dt.davis.events, from: now()-3d | filter display_id == "P-42" and k8s.cluster.name == "prod"`
	qb := b.BindQuery(r, q, "")
	assert.True(t, qb.Subject)
	assert.Equal(t, map[string]string{"id": "P-42"}, qb.Args)
	assert.Equal(t, []string{"prod"}, qb.Carry.Scope["cluster"])
	assert.Equal(t, "3d", qb.Carry.From)
	assert.Equal(t, "dtctl run costs-events P-42 --cluster=prod --from=3d", HintCommand(r, qb))

	qb = b.BindQuery(r, `fetch dt.davis.events | filter display_id == "oops"`, "30d")
	assert.False(t, qb.Subject, "a value the param rejects is not bound")
	assert.Empty(t, qb.Carry.From, "a window beyond the recipe's max is not carried")
	assert.Equal(t, "dtctl run costs-events <id>", HintCommand(r, qb))

	assert.Equal(t, 6*time.Hour, QueryWindow("fetch logs, from: -6h", ""))
	assert.Equal(t, 2*time.Hour, QueryWindow("fetch logs, from: -6h", "2h"), "--from wins")
	assert.Zero(t, QueryWindow("fetch logs", ""))
}
