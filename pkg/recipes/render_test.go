package recipes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuoteStringEscapes(t *testing.T) {
	assert.Equal(t, `"plain"`, QuoteString("plain"))
	assert.Equal(t, `"a\"b"`, QuoteString(`a"b`))
	assert.Equal(t, `"back\\slash"`, QuoteString(`back\slash`))
	assert.Equal(t, `"line\nbreak"`, QuoteString("line\nbreak"))
}

func TestRenderEscapesStringParams(t *testing.T) {
	b, r := mustRecipe(t, "services-failures", `
summary: failures
timeframe: 2h
params:
  service:
    type: string
    required: true
    positional: true
dql: |
  fetch spans
  | filter dt.service.name == {{.service}}
means: m
emptyMeans: e
`)
	// An injection attempt stays inside one string literal.
	out, err := b.Render(r, Input{Params: map[string]any{"service": `x" or true or "`}, Window: mustWindow(t, r, "", "")})
	require.NoError(t, err)
	assert.Contains(t, out.DQL, `dt.service.name == "x\" or true or \""`)
	assert.NotContains(t, out.DQL, `== "x" or`)
}

func TestRenderParamTypes(t *testing.T) {
	b, r := mustRecipe(t, "k8s-things", `
summary: things
timeframe: 1h
params:
  names:
    type: list
  min_restarts:
    type: int
    default: 5
    min: 1
    max: 100
  agg:
    type: enum
    values: [avg, max]
    default: avg
    render: identifier
  detailed:
    type: bool
dql: |
  timeseries v = {{.agg}}(dt.kubernetes.container.restarts)
  | filter v >= {{.min_restarts}}
  {{- if .names}}
  | filter in(k8s.pod.name, {{.names}}){{end}}
  {{- if .detailed}}
  | fieldsAdd detailed = true{{end}}
means: m
emptyMeans: e
`)
	w := mustWindow(t, r, "", "")

	out, err := b.Render(r, Input{Window: w})
	require.NoError(t, err)
	assert.Equal(t, "timeseries v = avg(dt.kubernetes.container.restarts)\n| filter v >= 5", out.DQL)

	names, err := r.Spec.Params.Get("names").ParseValue("a, b")
	require.NoError(t, err)
	out, err = b.Render(r, Input{Window: w, Params: map[string]any{"names": names, "agg": "max", "detailed": true}})
	require.NoError(t, err)
	assert.Contains(t, out.DQL, "max(dt.kubernetes")
	assert.Contains(t, out.DQL, `in(k8s.pod.name, {"a", "b"})`)
	assert.Contains(t, out.DQL, "fieldsAdd detailed = true")

	_, err = r.Spec.Params.Get("min_restarts").ParseValue("0")
	assert.ErrorContains(t, err, "min")
	_, err = r.Spec.Params.Get("min_restarts").ParseValue("abc")
	assert.Error(t, err)
	_, err = r.Spec.Params.Get("agg").ParseValue("sum")
	assert.ErrorContains(t, err, "avg")
}

func TestRenderRequiredAndPattern(t *testing.T) {
	b, r := mustRecipe(t, "services-get", `
summary: get
timeframe: 30d
params:
  id:
    type: string
    required: true
    positional: true
    pattern: '^P-[0-9]+$'
dql: |
  fetch dt.davis.problems
  | filter display_id == {{.id}}
means: m
emptyMeans: e
`)
	w := mustWindow(t, r, "", "")
	_, err := b.Render(r, Input{Window: w})
	assert.ErrorContains(t, err, "missing required argument <id>")

	_, err = r.Spec.Params.Get("id").ParseValue("P-12x")
	assert.Error(t, err)

	// Placeholders need not match the pattern.
	out, err := b.Render(r, Input{Window: w, Placeholders: true})
	require.NoError(t, err)
	assert.Contains(t, out.DQL, `display_id == "<id>"`)
}

func TestRenderUnsetOptionalParamFailsLoudly(t *testing.T) {
	b, r := mustRecipe(t, "services-x", `
summary: x
timeframe: 1h
params:
  service:
    type: string
dql: |
  fetch spans | filter dt.service.name == {{.service}}
means: m
emptyMeans: e
`)
	_, err := b.Render(r, Input{Window: mustWindow(t, r, "", "")})
	assert.ErrorContains(t, err, "unset value")
}

func TestRenderScope(t *testing.T) {
	b, r := mustRecipe(t, "k8s-pods", `
summary: pods
timeframe: 1h
scope: [cluster, namespace, tag]
dql: |
  fetch logs
  {{- with .scope.stage}}
  {{.}}{{end}}
  | limit 10
means: m
emptyMeans: e
`)
	w := mustWindow(t, r, "", "")

	out, err := b.Render(r, Input{Window: w})
	require.NoError(t, err)
	assert.Equal(t, "fetch logs\n| limit 10", out.DQL)

	out, err = b.Render(r, Input{Window: w, Scope: map[string][]string{
		"cluster":   {"prod"},
		"namespace": {"a", "b"},
		"tag":       {"team=payments", "cost-center=42"},
	}})
	require.NoError(t, err)
	// Always in(), even for one value: some fields are arrays.
	assert.Contains(t, out.DQL, `in(k8s.cluster.name, {"prod"})`)
	assert.Contains(t, out.DQL, `in(k8s.namespace.name, {"a", "b"})`)
	assert.Contains(t, out.DQL, `in(primary_tags.team, {"payments"})`)
	// A tag key that is not a plain identifier quotes the whole flat field name.
	assert.Contains(t, out.DQL, "in(`primary_tags.cost-center`, {\"42\"})")
	assert.Contains(t, out.DQL, " and ")

	_, err = b.Render(r, Input{Window: w, Scope: map[string][]string{"tag": {"novalue"}}})
	assert.ErrorContains(t, err, "key=value")
}

func TestRenderInlineWindow(t *testing.T) {
	b, r := mustRecipe(t, "costs-usage", `
summary: usage
timeframe: {default: 7d, inline: true, align: utc-day}
segments: off
dql: |
  fetch dt.system.events, from: {{.window.from}}, to: {{.window.to}}
  | limit 1
means: m
emptyMeans: e
`)
	w := mustWindow(t, r, "", "")
	out, err := b.Render(r, Input{Window: w})
	require.NoError(t, err)
	assert.Nil(t, out.Window, "an inline window is not sent as the default timeframe")
	require.NotNil(t, out.InlineWindow)
	assert.Contains(t, out.DQL, `from: toTimestamp("2026-03-03T00:00:00Z"), to: toTimestamp("2026-03-10T00:00:00Z")`)
}

func TestParseValueRejectsEmpty(t *testing.T) {
	for _, p := range []*Param{
		{Name: "service", Type: TypeString},
		{Name: "services", Type: TypeList},
	} {
		_, err := p.ParseValue("  ")
		assert.ErrorContains(t, err, "omit the flag", p.Name)
	}
}
