package exec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// newTimeseriesExecutor serves one timeseries record with 60 full-precision
// points and a spike at index 30.
func newTimeseriesExecutor(t *testing.T) *DQLExecutor {
	t.Helper()
	values := make([]interface{}, 60)
	for i := range values {
		values[i] = 3.10276124773992 + float64(i%5)
	}
	values[30] = 409.123456789
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := DQLQueryResponse{
			State: "SUCCEEDED",
			Result: &DQLResult{Records: []map[string]interface{}{{
				"host.name": "web-01",
				"cpu":       values,
				"interval":  "60000000000",
				"timeframe": map[string]interface{}{
					"start": "2026-01-01T12:00:00.000000000Z",
					"end":   "2026-01-01T13:00:00.000000000Z",
				},
			}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	c, err := client.NewForTesting(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return NewDQLExecutor(c)
}

func TestDQLExecutor_SeriesModes(t *testing.T) {
	clearAIAgentEnvVars(t)
	executor := newTimeseriesExecutor(t)
	run := func(t *testing.T, opts DQLExecuteOptions) string {
		t.Helper()
		return string(captureStdout(t, func() {
			if err := executor.ExecuteWithContext(context.Background(), "timeseries cpu=avg(x)", opts); err != nil {
				t.Fatalf("execute: %v", err)
			}
		}))
	}

	t.Run("default output keeps every full-precision point", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{OutputFormat: "json"})
		if !strings.Contains(out, "3.10276124773992") || strings.Contains(out, `"spark"`) {
			t.Errorf("default output must be unchanged:\n%s", out)
		}
	})

	t.Run("summary in json", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{OutputFormat: "json", Series: output.SeriesMode{Kind: output.SeriesSummary}})
		for _, want := range []string{`"spark"`, `"max": 409`, `"max_at": "2026-01-01T12:30:00Z"`, `"n": 60`} {
			if !strings.Contains(out, want) {
				t.Errorf("summary output missing %s:\n%s", want, out)
			}
		}
		if strings.Contains(out, "3.10276124773992") {
			t.Errorf("summary should not carry raw points:\n%s", out)
		}
	})

	t.Run("summary inside the agent envelope", func(t *testing.T) {
		// Agent mode leaves -o at its "table" default; the summary must still
		// be an object in the JSON envelope, not a table cell string.
		out := run(t, DQLExecuteOptions{
			OutputFormat: "table",
			AgentMode:    true,
			Spill:        SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: t.TempDir(), Format: "json"},
			Series:       output.SeriesMode{Kind: output.SeriesSummary},
		})
		var env struct {
			OK     bool `json:"ok"`
			Result struct {
				Records []map[string]interface{} `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("not an envelope: %v\n%s", err, out)
		}
		cpu, ok := env.Result.Records[0]["cpu"].(map[string]interface{})
		if !env.OK || !ok || cpu["max"] != 409.0 {
			t.Errorf("agent envelope should carry the summary, got:\n%s", out)
		}
	})

	t.Run("summary is visible to --jq in agent mode", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{
			OutputFormat: "json",
			AgentMode:    true,
			JQFilter:     ".records[0].cpu.max",
			Series:       output.SeriesMode{Kind: output.SeriesSummary},
		})
		if !strings.Contains(out, `"result":409`) {
			t.Errorf("jq should filter the summarized records, got:\n%s", out)
		}
	})

	t.Run("summary in a table is one readable cell", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{OutputFormat: "table", Series: output.SeriesMode{Kind: output.SeriesSummary}})
		if strings.Contains(out, "<60 items>") || !strings.Contains(out, "max=409") {
			t.Errorf("table should render the summary instead of <60 items>:\n%s", out)
		}
	})

	t.Run("downsample keeps the spike and rescales the interval", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{OutputFormat: "json", Series: output.SeriesMode{Kind: output.SeriesDownsample, Points: 10}})
		var doc struct {
			Records []map[string]interface{} `json:"records"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("invalid json: %v\n%s", err, out)
		}
		cpu := doc.Records[0]["cpu"].([]interface{})
		if len(cpu) > 10 || !strings.Contains(out, "409.123456789") {
			t.Errorf("downsample:10 should keep the spike in at most 10 points, got %d:\n%s", len(cpu), out)
		}
		if doc.Records[0]["interval"] != "360000000000" {
			t.Errorf("interval = %v, want 6 minutes", doc.Records[0]["interval"])
		}
	})

	t.Run("precision rounds full output", func(t *testing.T) {
		out := run(t, DQLExecuteOptions{OutputFormat: "json", Precision: 3})
		if strings.Contains(out, "3.10276124773992") || !strings.Contains(out, "3.1") || !strings.Contains(out, "409") {
			t.Errorf("--precision 3 should round every point:\n%s", out)
		}
	})
}
