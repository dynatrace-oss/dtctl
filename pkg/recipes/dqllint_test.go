package recipes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDQLLints: each trap is caught, and its fixed form is not.
func TestDQLLints(t *testing.T) {
	cases := []struct {
		name, timeframe, dql, code string
	}{
		{"sampling on events", "1h", `fetch events, samplingRatio: 10 | summarize n = sum(coalesce(dt.system.sampling_ratio, 1))`, "sampling-source"},
		{"sampling on logs", "1h", `fetch logs, samplingRatio: 10 | summarize n = sum(coalesce(dt.system.sampling_ratio, 1))`, ""},
		{"sampled count unscaled", "1h", `fetch logs, samplingRatio: 100 | summarize n = count()`, "sampling-unscaled"},
		{"limit before summarize", "1h", "fetch logs\n| limit 1000\n| summarize n = count(), by: {loglevel}", "limit-before-aggregate"},
		{"limit after summarize", "1h", "fetch logs\n| summarize n = count(), by: {loglevel}\n| limit 10", ""},
		{"coalesce compared in filter", "1h", `fetch logs | filter coalesce(service.name, k8s.workload.name) == "x" | summarize n = count()`, "coalesce-filter"},
		{"coalesce in fieldsAdd", "1h", `fetch logs | fieldsAdd s = coalesce(service.name, k8s.workload.name) | summarize n = count(), by: {s}`, ""},
		{"case-folded field in filter", "1h", `fetch logs | filter contains(lower(content), "oomkill") | summarize n = count()`, "case-folded-filter"},
		{"case-folded coalesce in filter", "1h", `fetch spans | filter contains(lower(coalesce(a.b, c.d)), "gpt") | summarize n = count()`, "case-folded-filter"},
		{"case-insensitive contains", "1h", `fetch logs | filter contains(content, "oomkill", caseSensitive: false) | summarize n = count()`, ""},
		{"case-folded literal", "1h", `fetch logs | filter trace_id == lower("ABC") | summarize n = count()`, ""},
		{"case-folded fieldsAdd", "1h", `fetch logs | fieldsAdd s = lower(loglevel) | summarize n = count(), by: {s}`, ""},
		{"unaliased aggregate", "1h", `fetch logs | summarize count(), by: {loglevel}`, "unaliased-aggregate"},
		{"comparison is not an alias", "1h", `fetch logs | summarize n = countIf(loglevel == "ERROR")`, ""},
		{"multi-key timeseries", "1h", `timeseries { a = avg(dt.host.cpu.usage), b = avg(dt.host.cpu.steal) }, by: {dt.smartscape.host}`, "multi-key-timeseries"},
		{"multi-key timeseries with union", "1h", `timeseries { a = avg(dt.host.cpu.usage), b = avg(dt.host.cpu.steal) }, by: {dt.smartscape.host}, union: true`, ""},
		{"interval equals window", "1h", `timeseries n = sum(dt.service.request.count), interval: 1h`, "interval-equals-window"},
		{"interval below window", "1h", `timeseries n = sum(dt.service.request.count), interval: 5m`, ""},
		{"commented-out trap", "1h", "fetch logs\n// | limit 5\n| summarize n = count()", ""},
		{"pipe inside a subquery", "1h", `fetch logs | filter in(trace.id, [fetch spans | limit 5 | fields trace.id]) | summarize n = count()`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, r := mustRecipe(t, "k8s-lint", "summary: s\ntimeframe: "+c.timeframe+"\ndql: |\n"+indent(c.dql, "  ")+"\nmeans: m\nemptyMeans: e")
			issues := b.DQLLint(r)
			if c.code == "" {
				assert.Empty(t, issues)
				return
			}
			require.Len(t, issues, 1, "%v", issues)
			assert.True(t, strings.HasPrefix(issues[0], c.code+": "), issues[0])
		})
	}
}

func TestIsEmptyZeroRow(t *testing.T) {
	r := &Recipe{}
	one := []map[string]any{{"n": 0.0}}
	assert.True(t, r.IsEmpty([]map[string]any{}))
	assert.False(t, r.IsEmpty(nil), "a streamed result is never empty")
	assert.False(t, r.IsEmpty(one), "no-rows is the default")

	r.Spec.Empty = EmptyZeroRow
	assert.True(t, r.IsEmpty(one))
	assert.True(t, r.IsEmpty([]map[string]any{{"n": "0", "unit": "DPS", "series": []any{0.0, nil}, "x": nil}}))
	assert.False(t, r.IsEmpty([]map[string]any{{"n": 0.0, "m": 3.0}}))
	assert.False(t, r.IsEmpty([]map[string]any{{"series": []any{0.0, 2.0}}}))
	assert.False(t, r.IsEmpty([]map[string]any{{"label": "x"}}), "a row without numbers is a finding")
	assert.False(t, r.IsEmpty([]map[string]any{{"n": 0.0}, {"n": 0.0}}), "two rows are two groups")
}
