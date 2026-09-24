package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// longContentResult is a result whose content values are far longer than a
// field cap, the shape `fetch logs | fields content | limit N` produces.
func longContentResult(n, contentLen int) (*DQLQueryResponse, []map[string]interface{}) {
	records := make([]map[string]interface{}, n)
	for i := range records {
		records[i] = map[string]interface{}{
			"host":    fmt.Sprintf("web-%02d", i),
			"content": fmt.Sprintf("row-%03d ", i) + strings.Repeat("x", contentLen),
		}
	}
	return &DQLQueryResponse{Records: records}, records
}

func envelopeRows(t *testing.T, resp output.Response) []map[string]interface{} {
	t.Helper()
	ir, ok := resp.Result.(*output.InlineRecords)
	if !ok {
		t.Fatalf("result = %T, want *output.InlineRecords", resp.Result)
	}
	return ir.Records
}

func encodedSize(t *testing.T, resp output.Response) int {
	t.Helper()
	var buf bytes.Buffer
	if err := output.EncodeEnvelope(&buf, resp); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Len()
}

func hasSuggestion(ctx *output.ResponseContext, substr string) bool {
	for _, s := range ctx.Suggestions {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestBuildSpillResponse_FieldCapClipsInlineValues(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(3, 1000)
	opts := DQLExecuteOptions{
		AgentMode:     true,
		MaxFieldChars: 50,
		Spill:         SpillOptions{Mode: SpillNever},
	}

	resp, handled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	rows := envelopeRows(t, resp)
	got := rows[0]["content"].(string)
	if !strings.HasSuffix(got, "…(+958 chars)") || len([]rune(got)) != 50+len([]rune("…(+958 chars)")) {
		t.Errorf("content not clipped with marker: %q", got)
	}
	if rows[0]["host"] != "web-00" {
		t.Errorf("short value changed: %v", rows[0]["host"])
	}
	ctx := resp.Context
	if !ctx.Truncated || ctx.MaxFieldChars != 50 || !reflect.DeepEqual(ctx.TruncatedFields, []string{"content"}) {
		t.Errorf("context does not report the clip: %+v", ctx)
	}
	if ctx.Returned != nil || ctx.NextOffset != nil {
		t.Errorf("a field clip must not claim dropped rows: %+v", ctx)
	}
	if !hasSuggestion(ctx, "--max-field-chars 0") || !hasSuggestion(ctx, "'| fields content'") {
		t.Errorf("suggestions do not say how to get full values: %v", ctx.Suggestions)
	}
	if !strings.HasPrefix(records[0]["content"].(string), "row-000 xxx") || len(records[0]["content"].(string)) != 1008 {
		t.Error("caller's records were mutated")
	}
}

func TestBuildSpillResponse_FieldCapZeroKeepsFullValues(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(2, 1000)
	opts := DQLExecuteOptions{AgentMode: true, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	// The opt-out restores the pre-cap output exactly: the caller's rows as-is
	// and no truncation marker or suggestion in the envelope.
	if !reflect.DeepEqual(envelopeRows(t, resp), records) {
		t.Error("--max-field-chars 0 must return the rows unchanged")
	}
	var buf bytes.Buffer
	if err := output.EncodeEnvelope(&buf, resp); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"truncated", "max_field_chars", "chars)", "--max-field-chars"} {
		if strings.Contains(buf.String(), key) {
			t.Errorf("uncapped envelope mentions %q", key)
		}
	}
}

// The spill decision measures what inline output would carry — the clipped
// rows — so a result that is large only because of a few long values stays
// inline once they are clipped, instead of spilling to a file.
func TestBuildSpillResponse_FieldCapMeasuredBeforeSpillDecision(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(5, 5000) // ~25 KB uncapped
	opts := DQLExecuteOptions{
		AgentMode:     true,
		MaxFieldChars: 100,
		Spill:         SpillOptions{Mode: SpillAuto, Threshold: 10 * 1024, Dir: t.TempDir(), Format: "jsonl"},
	}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Context.Decided != "inline" {
		t.Fatalf("decided = %q, want inline", resp.Context.Decided)
	}
	if resp.Context.MeasuredBytes > 10*1024 {
		t.Errorf("measured_bytes = %d, want the clipped size", resp.Context.MeasuredBytes)
	}
}

// With the cap on (the agent-mode default) but nothing long enough to clip,
// the envelope carries no marker and no opt-out suggestion.
func TestBuildSpillResponse_FieldCapNothingClippedAddsNothing(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(3, 10)
	opts := DQLExecuteOptions{AgentMode: true, MaxFieldChars: 500, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Context.Truncated || resp.Context.MaxFieldChars != 0 || hasSuggestion(resp.Context, "--max-field-chars") {
		t.Errorf("nothing was clipped, context must not say so: %+v", resp.Context)
	}
}

func TestBuildSpillResponse_SpillFileKeepsFullValues(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(3, 1000)
	dest := filepath.Join(t.TempDir(), "out.jsonl")
	opts := DQLExecuteOptions{
		AgentMode:     true,
		MaxFieldChars: 50,
		Spill:         SpillOptions{Mode: SpillAlways, ToPath: dest},
	}

	if _, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "chars)") || !strings.Contains(string(data), strings.Repeat("x", 1000)) {
		t.Error("spilled file must hold the full, unclipped values")
	}
}

func TestBuildSpillResponse_BudgetDropsRowsToFit(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(50, 100)
	const budget = 2000
	opts := DQLExecuteOptions{
		AgentMode:      true,
		MaxOutputBytes: budget,
		Spill:          SpillOptions{Mode: SpillNever},
	}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if size := encodedSize(t, resp); size > budget {
		t.Fatalf("encoded envelope is %d bytes, over the %d-byte budget", size, budget)
	}
	rows := envelopeRows(t, resp)
	ctx := resp.Context
	if len(rows) == 0 || len(rows) >= 50 {
		t.Fatalf("returned %d rows, want a proper prefix", len(rows))
	}
	for i, r := range rows {
		if r["host"] != fmt.Sprintf("web-%02d", i) {
			t.Fatalf("row %d is %v: the kept rows must be the leading rows in order", i, r["host"])
		}
	}
	if !ctx.Truncated || *ctx.Total != 50 || *ctx.Returned != len(rows) || *ctx.NextOffset != len(rows) || ctx.BudgetBytes != budget {
		t.Errorf("context = %+v", ctx)
	}
	if ctx.Next != "" {
		t.Errorf("next = %q, want empty: nothing was written to disk under --spill=never", ctx.Next)
	}
	if !hasSuggestion(ctx, "--spill-to") {
		t.Errorf("suggestions must say how to get the remaining rows: %v", ctx.Suggestions)
	}

	// The fit is maximal: one more row would not have fit.
	opts.MaxOutputBytes = int64(encodedSize(t, resp)) + 100 // < one row's ~139 bytes
	bigger, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(envelopeRows(t, bigger)); n != len(rows) {
		t.Errorf("with a slightly larger budget %d rows fit, want still %d", n, len(rows))
	}
}

func TestBuildSpillResponse_UnderBudgetIsUntouched(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(3, 10)
	opts := DQLExecuteOptions{AgentMode: true, MaxOutputBytes: 1 << 20, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(envelopeRows(t, resp)) != 3 || resp.Context.Truncated || resp.Context.BudgetBytes != 0 {
		t.Errorf("an under-budget result must be emitted unchanged: %+v", resp.Context)
	}
}

// With spilling available, a budget cut writes the full result to disk so the
// agent continues from next_offset with inspect instead of re-querying Grail.
func TestBuildSpillResponse_BudgetWritesContinuationFile(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(50, 100)
	dir := t.TempDir()
	opts := DQLExecuteOptions{
		AgentMode:      true,
		MaxOutputBytes: 3000,
		ContextName:    "prod",
		Spill:          SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: dir, Format: "jsonl"},
	}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := resp.Context
	if ctx.Decided != "inline" || !ctx.Truncated {
		t.Fatalf("context = %+v", ctx)
	}
	k := *ctx.NextOffset
	matches, _ := filepath.Glob(filepath.Join(dir, "results", "q-*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("continuation files = %v, want one", matches)
	}
	want := fmt.Sprintf("dtctl inspect %s --page --offset %d --limit %d", matches[0], k, k)
	if ctx.Next != want {
		t.Errorf("next = %q, want %q", ctx.Next, want)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 50 {
		t.Errorf("continuation file has %d rows, want all 50", lines)
	}
	if size := encodedSize(t, resp); size > 3000 {
		t.Errorf("envelope %d bytes over budget", size)
	}
}

// context.next must stay one runnable argument even when the spill location
// contains spaces or shell metacharacters.
func TestBuildSpillResponse_BudgetNextQuotesPath(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(50, 100)
	dir := filepath.Join(t.TempDir(), "my dir $(touch x); 'q'")
	opts := DQLExecuteOptions{
		AgentMode:      true,
		MaxOutputBytes: 3000,
		Spill:          SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: dir, Format: "jsonl"},
	}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "results", "q-*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("continuation files = %v", matches)
	}
	k := *resp.Context.NextOffset
	quoted := "'" + strings.ReplaceAll(matches[0], "'", `'\''`) + "'"
	want := fmt.Sprintf("dtctl inspect %s --page --offset %d --limit %d", quoted, k, k)
	if resp.Context.Next != want {
		t.Errorf("next = %q, want %q", resp.Context.Next, want)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/home/u/.cache/dtctl/results/prod/q-1a2b.jsonl": "/home/u/.cache/dtctl/results/prod/q-1a2b.jsonl",
		"/tmp/my dir/q.jsonl":                            "'/tmp/my dir/q.jsonl'",
		"/tmp/$(rm -rf ~)/q.jsonl":                       "'/tmp/$(rm -rf ~)/q.jsonl'",
		"/tmp/it's/q.jsonl":                              `'/tmp/it'\''s/q.jsonl'`,
		"":                                               "''",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestBuildSpillResponse_BudgetTOON(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(50, 100)
	opts := DQLExecuteOptions{AgentMode: true, MaxOutputBytes: 2500, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "toon", opts)
	if err != nil {
		t.Fatal(err)
	}
	enc, ok := resp.Result.(*output.InlineRecordsEncoded)
	if !ok {
		t.Fatalf("result = %T", resp.Result)
	}
	if size := encodedSize(t, resp); size > 2500 {
		t.Errorf("envelope %d bytes over budget", size)
	}
	k := *resp.Context.Returned
	if k == 0 || k >= 50 || !strings.Contains(enc.Records, fmt.Sprintf("row-%03d", k-1)) || strings.Contains(enc.Records, fmt.Sprintf("row-%03d", k)) {
		t.Errorf("TOON payload does not hold exactly the first %d rows", k)
	}
}

func TestBuildSpillResponse_BudgetBelowEnvelopeOverhead(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(5, 100)
	opts := DQLExecuteOptions{AgentMode: true, MaxOutputBytes: 10, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(envelopeRows(t, resp)); n != 0 || *resp.Context.Returned != 0 {
		t.Errorf("returned %d rows, want 0", n)
	}
	found := false
	for _, w := range resp.Context.Warnings {
		if strings.Contains(w, "smaller than the envelope") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings must say the budget cannot hold even one row: %v", resp.Context.Warnings)
	}
}

func TestE2E_QueryFieldCapWithJQ(t *testing.T) {
	_, records := longContentResult(3, 1000)
	e := mockGrail(t, records)

	out := runAndCapture(t, func() error {
		return e.ExecuteWithOptions("fetch logs", DQLExecuteOptions{
			OutputFormat:   "json",
			AgentMode:      true,
			JQFilter:       `[.records[] | select(.content | endswith("xxx"))]`,
			MaxFieldChars:  20,
			MaxOutputBytes: 1 << 20,
			Spill:          SpillOptions{Mode: SpillNever},
		})
	})

	var env struct {
		Result  []map[string]interface{} `json:"result"`
		Context output.ResponseContext   `json:"context"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(env.Result) != 3 {
		t.Fatalf("the filter must see full values; got %d rows", len(env.Result))
	}
	if c := env.Result[0]["content"].(string); !strings.HasSuffix(c, "…(+988 chars)") {
		t.Errorf("content = %q", c)
	}
	if !env.Context.Truncated || env.Context.MaxFieldChars != 20 {
		t.Errorf("context = %+v", env.Context)
	}
	found := false
	for _, w := range env.Context.Warnings {
		if strings.Contains(w, "--max-output-bytes") {
			found = true
		}
	}
	if !found {
		t.Errorf("an output budget that --jq cannot honor must warn: %v", env.Context.Warnings)
	}
}

// Under -o auto the field cap applies first: auto measures, chooses and encodes
// the clipped rows, so the long values never reach the envelope.
func TestBuildSpillResponse_FieldCapBeforeAutoFormat(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(5, 5000)
	opts := DQLExecuteOptions{
		AgentMode:     true,
		MaxFieldChars: 20,
		Spill:         SpillOptions{Mode: SpillAuto, Threshold: 10 * 1024, Dir: t.TempDir(), Format: "jsonl"},
	}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "auto", opts)
	if err != nil {
		t.Fatal(err)
	}
	enc, ok := resp.Result.(*output.InlineRecordsEncoded)
	if !ok {
		t.Fatalf("result = %T, want the auto-encoded inline rows", resp.Result)
	}
	if enc.Encoding != "csv" || resp.Context.Format != "csv" {
		t.Errorf("encoding = %q, format = %q, want csv", enc.Encoding, resp.Context.Format)
	}
	if strings.Contains(enc.Records, strings.Repeat("x", 100)) || !strings.Contains(enc.Records, "…(+4988 chars)") {
		t.Errorf("auto encoded unclipped values: %.200s", enc.Records)
	}
	if resp.Context.Decided != "inline" || resp.Context.MeasuredBytes > 10*1024 {
		t.Errorf("auto must measure the clipped rows: %+v", resp.Context)
	}
	if !resp.Context.Truncated || !reflect.DeepEqual(resp.Context.TruncatedFields, []string{"content"}) {
		t.Errorf("clip not reported: %+v", resp.Context)
	}
}

func TestBuildSpillResponse_BudgetWithAutoFormat(t *testing.T) {
	e := &DQLExecutor{}
	result, records := longContentResult(50, 100)
	opts := DQLExecuteOptions{AgentMode: true, MaxOutputBytes: 2000, Spill: SpillOptions{Mode: SpillNever}}

	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "auto", opts)
	if err != nil {
		t.Fatal(err)
	}
	if size := encodedSize(t, resp); size > 2000 {
		t.Errorf("envelope %d bytes over budget", size)
	}
	enc, ok := resp.Result.(*output.InlineRecordsEncoded)
	if !ok {
		t.Fatalf("result = %T", resp.Result)
	}
	k := *resp.Context.Returned
	if k < 2 || k >= 50 || enc.Encoding != resp.Context.Format {
		t.Errorf("returned %d, encoding %q, context.format %q", k, enc.Encoding, resp.Context.Format)
	}
	if !strings.Contains(enc.Records, fmt.Sprintf("row-%03d", k-1)) || strings.Contains(enc.Records, fmt.Sprintf("row-%03d", k)) {
		t.Errorf("auto payload does not hold exactly the first %d rows", k)
	}
}
