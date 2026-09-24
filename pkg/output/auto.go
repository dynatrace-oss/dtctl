package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// FormatAuto is the `-o auto` output format: instead of a fixed encoding, dtctl
// picks the most token-efficient lossless one for the shape of the result
// (ChooseAutoFormat) and reports what it picked.
const FormatAuto = "auto"

// Reasons ChooseAutoFormat gives for its choice. They are part of the stderr
// notice a human sees, so keep them short.
const (
	AutoReasonEmpty        = "empty result"
	AutoReasonScalar       = "scalar value"
	AutoReasonSingleObject = "single object"
	AutoReasonSingleRow    = "single row"
	AutoReasonFlatRows     = "uniform flat rows"
	AutoReasonSparseRows   = "sparse rows"
	AutoReasonNested       = "nested values"
	AutoReasonNotRows      = "list of non-object values"
	AutoReasonUnencodable  = "not JSON-encodable"
)

// Thresholds for the tabular (CSV) choice. They are deliberately conservative
// starting points, kept as named constants so an evaluation (answer accuracy
// and tokens per result shape) can tune them without touching the logic.
const (
	// autoMinTabularRows is the fewest rows worth a CSV header. A single row
	// reads better as key: value lines, which say the same thing without a
	// header line to align against.
	autoMinTabularRows = 2
	// autoMinTabularDensity is the lowest share of filled cells (rows × union of
	// columns) for which CSV still pays off. Below it most of the CSV would be
	// empty separators, and YAML, which only lists the keys a row has, is
	// denser and easier to read.
	autoMinTabularDensity = 0.5
)

// AutoChoice is the format `-o auto` settled on and why.
type AutoChoice struct {
	Format string // "csv", "yaml" or "json"
	Reason string // one of the AutoReason* constants
}

// IsAutoFormat reports whether format selects `-o auto`.
func IsAutoFormat(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), FormatAuto)
}

// ChooseAutoFormat picks an output format from the shape of data. Only lossless
// encodings are candidates (a table truncates and TOON is never the smallest
// here), and the rules are, in order:
//
//   - nil, an empty list, or a scalar → json. There is nothing to compress, and
//     in agent mode the result then stays a native JSON value.
//   - a single object, or a list with one object → yaml (key: value lines).
//   - a list of objects whose values are all scalars and whose cells are at
//     least half filled → csv. This is the typical `summarize`/`fields` result,
//     where CSV states every column name once.
//   - anything else (nested values, sparse rows, lists of non-objects) → yaml,
//     which stays readable for nested data and needs no quoting or braces.
//
// Structs are judged by their JSON form, so field names match `-o json`.
func ChooseAutoFormat(data interface{}) AutoChoice {
	generic, err := autoGeneric(data)
	if err != nil {
		return AutoChoice{Format: "json", Reason: AutoReasonUnencodable}
	}
	return chooseAutoGeneric(generic)
}

func chooseAutoGeneric(data interface{}) AutoChoice {
	switch v := data.(type) {
	case nil:
		return AutoChoice{"json", AutoReasonEmpty}
	case map[string]interface{}:
		return AutoChoice{"yaml", AutoReasonSingleObject}
	case []map[string]interface{}:
		rows := make([]interface{}, len(v))
		for i := range v {
			rows[i] = v[i]
		}
		return chooseAutoList(rows)
	case []interface{}:
		return chooseAutoList(v)
	default:
		return AutoChoice{"json", AutoReasonScalar}
	}
}

func chooseAutoList(rows []interface{}) AutoChoice {
	if len(rows) == 0 {
		return AutoChoice{"json", AutoReasonEmpty}
	}
	columns := map[string]struct{}{}
	filled := 0
	for _, r := range rows {
		row, ok := r.(map[string]interface{})
		if !ok {
			return AutoChoice{"yaml", AutoReasonNotRows}
		}
		for k, val := range row {
			if !isAutoScalar(val) {
				return AutoChoice{"yaml", AutoReasonNested}
			}
			columns[k] = struct{}{}
			if val != nil {
				filled++
			}
		}
	}
	if len(rows) < autoMinTabularRows {
		return AutoChoice{"yaml", AutoReasonSingleRow}
	}
	cells := len(rows) * len(columns)
	if cells == 0 || float64(filled)/float64(cells) < autoMinTabularDensity {
		return AutoChoice{"yaml", AutoReasonSparseRows}
	}
	return AutoChoice{"csv", AutoReasonFlatRows}
}

func isAutoScalar(v interface{}) bool {
	switch v.(type) {
	case nil, string, bool, float64, float32, int, int64, int32, uint, uint64, uint32, json.Number:
		return true
	default:
		return false
	}
}

// autoGeneric returns data as untyped JSON values (maps, slices, scalars) so the
// shape rules and the encoders see exactly what `-o json` would print. Values
// that already are untyped — DQL records, --jq output — pass through as-is, so
// `-o auto` renders them byte-identically to the explicit format it picks.
// Anything else round-trips through encoding/json, keeping integers as int64
// rather than float64 so large ones don't print in exponent notation.
func autoGeneric(data interface{}) (interface{}, error) {
	switch data.(type) {
	case nil, map[string]interface{}, []map[string]interface{}, []interface{}, string, bool, float64:
		return data, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var generic interface{}
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	return intsFromNumbers(generic), nil
}

// intsFromNumbers replaces every json.Number with an int64 when it is one and a
// float64 otherwise.
func intsFromNumbers(v interface{}) interface{} {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64()
		return f
	case map[string]interface{}:
		for k, val := range t {
			t[k] = intsFromNumbers(val)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = intsFromNumbers(val)
		}
		return t
	default:
		return v
	}
}

// MarshalAuto chooses a format for data and encodes it. For a json choice the
// value is returned unchanged (encoded stays empty) so callers can embed it
// natively; for csv/yaml encoded holds the text.
func MarshalAuto(data interface{}) (choice AutoChoice, encoded string, err error) {
	generic, err := autoGeneric(data)
	if err != nil {
		return AutoChoice{}, "", err
	}
	choice = chooseAutoGeneric(generic)
	if choice.Format == "json" {
		return choice, "", nil
	}
	var buf bytes.Buffer
	p := NewPrinterWithOpts(PrinterOptions{Format: choice.Format, Writer: &buf})
	if err := p.PrintList(generic); err != nil {
		return AutoChoice{}, "", err
	}
	return choice, buf.String(), nil
}

// AutoDefaultSuggestion is the context.suggestions entry added when the
// agent-mode default (-o auto, no -o given) returned a non-JSON encoding. It
// names the opt-out that restores the native JSON result.
func AutoDefaultSuggestion(format string) string {
	return "# result is " + format + " (agent default -o auto); -o json returns native JSON"
}

// FprintAutoChoice writes the one-line notice that tells a human which format
// `-o auto` picked. It goes to stderr so stdout stays exactly the chosen format.
func FprintAutoChoice(w io.Writer, choice AutoChoice) {
	FprintInfo(w, "-o auto: %s (%s)", choice.Format, choice.Reason)
}

// AutoPrinter implements `-o auto` outside agent mode: it chooses the format
// from the (jq-filtered) data, prints it in that format, and names the choice on
// stderr. In agent mode the choice is reported in context.format instead (see
// AgentPrinter).
type AutoPrinter struct {
	writer   io.Writer
	notice   io.Writer // nil means os.Stderr, resolved per call
	jqFilter string
}

// Print prints obj in the format ChooseAutoFormat picks for it.
func (p *AutoPrinter) Print(obj interface{}) error {
	transformed, err := ApplyJQ(p.jqFilter, obj)
	if err != nil {
		return err
	}
	generic, err := autoGeneric(transformed)
	if err != nil {
		return fmt.Errorf("-o auto: %w", err)
	}
	choice := chooseAutoGeneric(generic)
	notice := p.notice
	if notice == nil {
		notice = os.Stderr
	}
	FprintAutoChoice(notice, choice)
	return NewPrinterWithOpts(PrinterOptions{Format: choice.Format, Writer: p.writer}).PrintList(generic)
}

// PrintList prints a list in the format ChooseAutoFormat picks for it.
func (p *AutoPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}
