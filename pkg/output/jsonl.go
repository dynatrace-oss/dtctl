package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// JSONLPrinter prints output as JSON Lines (JSONL / newline-delimited JSON):
// one compact JSON value per line. It is well suited to large query results —
// each record is encoded and written as it is visited rather than marshalling
// the whole slice into one buffer, and the result is read natively by common
// local data tooling.
//
// Note: the input record slice is still fully materialised in memory by the
// caller; only the serialised form is produced incrementally. End-to-end bounded
// streaming (records never fully buffered) is a separate, later change.
type JSONLPrinter struct {
	writer io.Writer
}

// Print writes a single object as one JSON line.
func (p *JSONLPrinter) Print(obj interface{}) error {
	enc := json.NewEncoder(p.writer)
	// json.Encoder.Encode emits compact JSON followed by a newline — exactly the
	// JSONL contract, one object per line.
	return enc.Encode(obj)
}

// PrintList writes each element of a slice as its own JSON line. Non-slice input
// is written as a single line (mirroring the single-object case).
func (p *JSONLPrinter) PrintList(obj interface{}) error {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %s", v.Kind())
	}

	// Empty slice → no output, consistent with the CSV printer.
	if v.Len() == 0 {
		return nil
	}

	enc := json.NewEncoder(p.writer)
	for i := 0; i < v.Len(); i++ {
		if err := enc.Encode(v.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}
