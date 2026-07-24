package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateCommandRegistered(t *testing.T) {
	var found bool
	for _, child := range rootCmd.Commands() {
		if child.Name() == "evaluate" {
			found = true
			require.Equal(t, "init", child.Commands()[0].Name())
			require.Equal(t, "tenant", child.Commands()[1].Name())
		}
	}
	require.True(t, found)
}

func TestCountEvaluateItems(t *testing.T) {
	t.Run("array root", func(t *testing.T) {
		require.Equal(t, 3, countEvaluateItems(`[{"id":1},{"id":2},{"id":3}]`))
	})

	t.Run("agent result list", func(t *testing.T) {
		require.Equal(t, 2, countEvaluateItems(`{"ok":true,"result":[{"id":"a"},{"id":"b"}]}`))
	})

	t.Run("single result", func(t *testing.T) {
		require.Equal(t, 1, countEvaluateItems(`{"ok":true,"result":{"id":"one"}}`))
	})

	t.Run("invalid json", func(t *testing.T) {
		require.Equal(t, 0, countEvaluateItems(`not-json`))
	})
}

func TestBuildEvaluateSummary(t *testing.T) {
	summary := buildEvaluateSummary(1, []evaluateProbeResult{
		{Status: "ok", ItemCount: 4},
		{Status: "error", ItemCount: 0},
		{Status: "timeout", ItemCount: 0, DeepCheck: true},
	}, evaluateAssessment{Score: 82, Grade: "B"})

	require.Equal(t, "tenant evaluation completed with probe issues", summary["message"])
	require.Equal(t, 1, summary["contexts"])
	require.Equal(t, 3, summary["total_probes"])
	require.Equal(t, 1, summary["successful"])
	require.Equal(t, 1, summary["failed"])
	require.Equal(t, 1, summary["timeouts"])
	require.Equal(t, 1, summary["deep_checks"])
	require.Equal(t, 4, summary["discovered_items"])
	require.Equal(t, 82, summary["score"])
	require.Equal(t, "B", summary["grade"])
}

func TestDefaultEvaluateProbes(t *testing.T) {
	probes := defaultEvaluateProbes()
	require.GreaterOrEqual(t, len(probes), 5)
	require.Equal(t, "get", probes[0].Command[0])
	require.NotEmpty(t, probes[0].Domain)
}

func TestRenderEvaluateExecutiveMarkdown(t *testing.T) {
	report := evaluateReport{
		GeneratedAt: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
		Target: evaluateContext{Name: "esa-eval", Environment: "https://example.apps.dynatrace.com"},
		Assessment: evaluateAssessment{Score: 88, Grade: "B"},
		Domains: []evaluateDomainSummary{{Domain: "automation", ProbeCount: 2, Successful: 2, DiscoveredItems: 5}},
		Risks: []evaluateRisk{{Severity: "high", Title: "Workflow executions include failures"}},
		Summary: map[string]any{"total_probes": 10, "successful": 9},
	}

	out := renderEvaluateMarkdownWithAudience(report, "executive")
	require.Contains(t, out, "# Executive Tenant Assessment")
	require.Contains(t, out, "Workflow executions include failures")
	require.Contains(t, out, "88/100")
}
