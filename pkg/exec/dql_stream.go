package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
)

// Streaming output for `dtctl query`.
//
// The SDK can deliver rows one at a time (ExecuteAndPollStream), but that only
// bounds memory if what consumes them does not put them back into a slice.
// Most of the output pipeline does: a table needs every row to size its
// columns, CSV needs every row for its header, compaction hoists the columns
// that are constant *across* rows, and --jq addresses the whole document.
//
// So streaming applies where rows reach a destination that takes them one at a
// time — a JSONL spill file or `-o jsonl` on stdout — and everything else keeps
// today's accumulate-then-print path. Even on an eligible invocation the rows
// are buffered at first, so a result that turns out to be small takes exactly
// the same code path it does today; the stream only takes over once the result
// is provably too big to be emitted inline (see streamSwitchRows).

// maxBufferedBytes caps what the buffering phase holds before handing over to
// the stream. The row count below settles the inline decision exactly, but says
// nothing about how fat a row is, so this bounds the phase in bytes too.
//
// Crossing it can in principle spill a result that compaction would have
// squeezed back under the threshold — that needs every column of a multi-megabyte
// result to be constant, and the row count would have settled it first in all
// but the most contrived shapes. The cost if it ever happens is a result on disk
// instead of inline, which the envelope describes either way.
const maxBufferedBytes = 32 << 20

// streamSinkKind is where a streamed row goes.
type streamSinkKind int

const (
	sinkSpill streamSinkKind = iota // a JSONL spill file, summarised in an agent envelope
	sinkJSONL                       // -o jsonl on stdout
)

// streamPlan is the streaming decision, made before the query runs because the
// sink has to be wired into the SDK call.
type streamPlan struct {
	kind streamSinkKind
	// spillDir is the directory the streamed file is built in. The final name
	// depends on the response metadata, which arrives after the rows, so the
	// file is committed to it only at the end (output.SpillWriter).
	spillDir string
	// switchRows is how many rows may be buffered before the result is known to
	// exceed the spill threshold; 0 streams from the first row.
	switchRows int
	// tabular mirrors what printResults passes to ApplySeriesMode for this
	// destination.
	tabular bool
	format  string // effective display format
}

// planStream decides whether this invocation can stream its rows, and where to.
// It mirrors the branches printResults would take, so an ineligible invocation
// is one whose output genuinely needs every row at once.
func (e *DQLExecutor) planStream(opts DQLExecuteOptions) (*streamPlan, bool) {
	format := opts.OutputFormat

	// --jq addresses the whole document, and snapshot decoding reshapes rows
	// per display format; both want the result in hand. --typed casts rows
	// using the response's type block, which the decoder may only reach after
	// the rows it describes. All three are opt-in.
	if opts.JQFilter != "" || opts.Decode != DecodeNone || opts.Typed {
		return nil, false
	}

	// -o jsonl with no spill destination writes rows straight to stdout.
	if !opts.Spill.Enabled() && !opts.AgentMode {
		if format != "jsonl" {
			return nil, false
		}
		return &streamPlan{kind: sinkJSONL, format: format}, true
	}

	// Agent mode under --spill=never always emits the rows inline, so they are
	// all needed at once.
	if !opts.Spill.Enabled() {
		return nil, false
	}

	// Only JSONL can be written row by row: JSON needs array framing the
	// printer owns, CSV a header built from every row, Parquet a full schema.
	spillFormat, err := e.streamedSpillFormat(opts)
	if err != nil || spillFormat != "jsonl" {
		return nil, false
	}

	dir, ok := e.streamedSpillDir(opts)
	if !ok {
		return nil, false
	}

	return &streamPlan{
		kind:       sinkSpill,
		spillDir:   dir,
		switchRows: streamSwitchRows(opts, format),
		tabular:    seriesTabular(format, opts),
		format:     format,
	}, true
}

// streamedSpillFormat is the format a streamed spill would be written in.
func (e *DQLExecutor) streamedSpillFormat(opts DQLExecuteOptions) (string, error) {
	if opts.Spill.ToPath != "" {
		return spillFormatForPath(opts.Spill.ToPath, opts.Spill.Format)
	}
	if opts.Spill.Format == "" {
		return defaultSpillFormat, nil
	}
	return opts.Spill.Format, validateSpillFormat(opts.Spill.Format)
}

// streamedSpillDir is the directory a streamed spill file is built in. It is
// the destination's own directory for --spill-to, and the managed per-context
// cache otherwise — the same choices resolveSpillTarget makes, minus the file
// name, which depends on metadata that has not arrived yet.
func (e *DQLExecutor) streamedSpillDir(opts DQLExecuteOptions) (string, bool) {
	if opts.Spill.ToPath != "" {
		dir := filepath.Dir(opts.Spill.ToPath)
		if !output.ProbeWritable(dir) {
			return "", false
		}
		return dir, true
	}
	base, managed, err := output.SpillBaseDir(opts.Spill.Dir)
	if err != nil {
		// No writable location: the buffered path degrades to summary-only,
		// which still needs the decision logic there.
		return "", false
	}
	if managed {
		return filepath.Join(base, output.SanitizeContextName(opts.ContextName)), true
	}
	return base, true
}

// streamSwitchRows is the largest number of rows that can still be emitted
// inline, and so the most the buffering phase may hold. Compaction can only
// ever shrink a row to an empty one, and an empty row still costs its share of
// the encoding, so any count above this exceeds the threshold whatever the rows
// turn out to contain. At or below it the buffered path is kept, which is what
// keeps a small result's output exactly what it is today.
func streamSwitchRows(opts DQLExecuteOptions, format string) int {
	if opts.Spill.Mode == SpillAlways {
		return 0
	}
	per, frame := emptyRowCost(format)
	if opts.Spill.Threshold <= 0 || per <= 0 {
		return 0
	}
	n := (opts.Spill.Threshold - frame) / per
	if n < 0 {
		return 0
	}
	return int(n)
}

// emptyRowCost measures what an all-empty row set costs in the inline envelope:
// per is the marginal cost of one more row, frame the fixed cost around them.
// Measured rather than assumed, so the switch point stays exact for whichever
// encoding the envelope uses.
func emptyRowCost(format string) (per, frame int64) {
	empty := map[string]interface{}{}
	one, _ := output.MeasureSerializedBytes([]map[string]interface{}{empty}, format)
	two, _ := output.MeasureSerializedBytes([]map[string]interface{}{empty, empty}, format)
	per = two - one
	if per <= 0 {
		return 1, 0
	}
	return per, one - per
}

// seriesTabular mirrors the tabular flag printResults computes for
// ApplySeriesMode, so a streamed row is transformed exactly as a buffered one.
func seriesTabular(format string, opts DQLExecuteOptions) bool {
	switch format {
	case "", "table", "wide":
		return !opts.AgentMode
	case "csv":
		return true
	}
	return false
}

// executeStreaming runs the query with rows delivered one at a time and hands
// the outcome to the collector, which either replays a buffered result through
// the ordinary output path or finishes the stream it started.
func (e *DQLExecutor) executeStreaming(ctx context.Context, query string, opts DQLExecuteOptions, plan *streamPlan) error {
	col := newStreamCollector(e, plan, opts)
	defer col.abort()

	result, err := e.runQuery(ctx, query, opts, col.observe)
	if err != nil {
		return err
	}
	if result == nil {
		return nil // context was cancelled; message already printed to stderr
	}
	if col.err != nil {
		return col.err
	}
	return col.finish(query, result, opts)
}

// streamCollector receives rows as they are decoded. It buffers at first and
// switches to the streamed sink once the result is provably too big to emit
// inline; from then on each row is written out and released, and everything the
// summary needs is folded into accumulators instead.
type streamCollector struct {
	e    *DQLExecutor
	plan *streamPlan
	opts DQLExecuteOptions

	buf       []map[string]interface{}
	bufBytes  int64
	streaming bool

	rows    int
	stats   *output.StatsAccumulator
	compact *output.CompactionAccumulator
	sample  []map[string]interface{}

	writer *output.SpillWriter
	enc    *json.Encoder

	effect output.SeriesEffect
	err    error
}

func newStreamCollector(e *DQLExecutor, plan *streamPlan, opts DQLExecuteOptions) *streamCollector {
	return &streamCollector{e: e, plan: plan, opts: opts}
}

// observe takes one decoded row.
func (c *streamCollector) observe(row map[string]interface{}) error {
	if c.err != nil {
		return c.err
	}
	row = c.transform(row)

	if !c.streaming {
		c.buf = append(c.buf, row)
		c.bufBytes += int64(estimateRowBytes(row))
		if len(c.buf) > c.plan.switchRows || c.bufBytes > maxBufferedBytes {
			if err := c.startStreaming(); err != nil {
				c.err = err
				return err
			}
		}
		return nil
	}
	return c.write(row)
}

// transform applies the per-row reshaping printResults applies to the whole
// slice. Both are row-independent, so folding the effect flags row by row
// yields the same advice the buffered path produces.
func (c *streamCollector) transform(row map[string]interface{}) map[string]interface{} {
	if c.opts.Series.Kind == output.SeriesFull && c.opts.Precision <= 0 {
		return row
	}
	out, eff := output.ApplySeriesModeWithEffect(
		[]map[string]interface{}{row}, c.opts.Series, c.opts.Precision, c.plan.tabular)
	c.effect.Summarized = c.effect.Summarized || eff.Summarized
	c.effect.Rounded = c.effect.Rounded || eff.Rounded
	if len(out) == 1 {
		return out[0]
	}
	return row
}

// startStreaming opens the sink and hands it every buffered row.
func (c *streamCollector) startStreaming() error {
	if c.plan.kind == sinkSpill {
		w, err := output.NewSpillWriter(c.plan.spillDir)
		if err != nil {
			return fmt.Errorf("failed to open spill file: %w", err)
		}
		c.writer = w
		c.enc = json.NewEncoder(w.Writer())
	} else {
		c.enc = json.NewEncoder(os.Stdout)
	}
	c.stats = output.NewStatsAccumulator(output.DefaultStatsTopK, output.DefaultStatsMaxDistinct)
	c.compact = output.NewCompactionAccumulator()
	c.streaming = true

	buffered := c.buf
	c.buf, c.bufBytes = nil, 0
	for _, row := range buffered {
		if err := c.write(row); err != nil {
			return err
		}
	}
	return nil
}

// write folds a row into the summary accumulators and emits it.
func (c *streamCollector) write(row map[string]interface{}) error {
	c.rows++
	if c.plan.kind == sinkSpill {
		c.stats.Observe(row)
		c.compact.Observe(row)
		if len(c.sample) < output.DefaultSampleRows {
			c.sample = append(c.sample, row)
		}
	}
	// json.Encoder writes one compact object per line — the JSONL contract, and
	// what the -o jsonl and jsonl-spill writers produce for a buffered result.
	if err := c.enc.Encode(row); err != nil {
		c.err = err
		return err
	}
	return nil
}

// abort discards an uncommitted spill file.
func (c *streamCollector) abort() {
	if c.writer != nil {
		c.writer.Abort()
	}
}

// finish emits the result. A collector that never left the buffering phase
// holds every row, so it replays the ordinary output path unchanged; one that
// started streaming has already emitted the rows and only has to close out.
func (c *streamCollector) finish(query string, result *DQLQueryResponse, opts DQLExecuteOptions) error {
	if !c.streaming {
		opts.seriesAdvice = defaultSeriesAdvice(c.effect, opts)
		return c.e.printRecords(query, result, orEmptyRows(c.buf), opts)
	}
	opts.seriesAdvice = defaultSeriesAdvice(c.effect, opts)
	if c.plan.kind == sinkJSONL {
		// The rows are already on stdout; notifications and the metadata footer
		// follow on stderr exactly as the buffered path emits them.
		return c.e.finishStreamedJSONL(query, result, opts)
	}
	return c.e.finishStreamedSpill(query, result, c, opts)
}

// orEmptyRows keeps a nil buffer distinguishable from "no rows at all" for the
// output path, which treats a nil slice as an absent result.
func orEmptyRows(rows []map[string]interface{}) []map[string]interface{} {
	if rows == nil {
		return []map[string]interface{}{}
	}
	return rows
}

// estimateRowBytes approximates a row's in-memory cost for the buffering cap.
// It counts keys and string values, which dominate a telemetry row, rather than
// serialising it — the cap only has to bound the buffer, not measure it.
func estimateRowBytes(row map[string]interface{}) int {
	n := 0
	for k, v := range row {
		n += len(k) + 16
		if s, ok := v.(string); ok {
			n += len(s)
		} else {
			n += 16
		}
	}
	return n
}

// streamCall picks the accumulating or the streaming SDK driver. Everything
// around it — progress reporting, token refresh, cancellation — is shared.
func streamCall(ctx context.Context, handler *sdkquery.Handler, req sdkquery.ExecuteRequest, sdkOpts sdkquery.ExecuteAndPollOptions, onRecord func(map[string]interface{}) error) (*sdkquery.Response, error) {
	if onRecord == nil {
		return handler.ExecuteAndPollWithOptions(ctx, req, sdkOpts)
	}
	return handler.ExecuteAndPollStream(ctx, req, sdkOpts, onRecord)
}
