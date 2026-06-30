package output

import (
	"fmt"

	"github.com/itchyny/gojq"
)

// IsStructuredOutputFormat reports whether a format can represent arbitrary
// JSON values emitted by jq.
func IsStructuredOutputFormat(format string) bool {
	switch format {
	case "json", "yaml", "yml", "toon":
		return true
	default:
		return false
	}
}

// NormalizeJQOutputFormat promotes non-structured formats to json when --jq is used.
func NormalizeJQOutputFormat(format string) string {
	if IsStructuredOutputFormat(format) {
		return format
	}
	return "json"
}

// ApplyJQ transforms input using the provided jq filter.
// If filter is empty, input is returned unchanged.
func ApplyJQ(filter string, input interface{}) (interface{}, error) {
	if filter == "" {
		return input, nil
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}

	generic, err := toGeneric(input)
	if err != nil {
		return nil, fmt.Errorf("failed to apply --jq filter: %w", err)
	}

	iter := code.Run(generic)
	results := make([]interface{}, 0, 1)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if runErr, ok := v.(error); ok {
			return nil, fmt.Errorf("failed to apply --jq filter: %w", runErr)
		}
		results = append(results, v)
	}

	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0], nil
	default:
		return results, nil
	}
}

// CompiledJQ is a jq program parsed and compiled once for repeated, per-record
// execution in a streaming filter (e.g. `dtctl inspect --jq` over a whole
// spilled file). ApplyJQ re-parses and re-compiles on every call, which is fine
// for a one-shot post-filter but quadratic when run over millions of records;
// CompiledJQ pays that cost once.
type CompiledJQ struct {
	code *gojq.Code
}

// CompileJQ parses and compiles a jq program for streaming, per-record use. The
// returned *CompiledJQ is safe to reuse across records (gojq does not mutate the
// compiled code). A parse/compile failure is reported as an invalid-filter error.
func CompileJQ(filter string) (*CompiledJQ, error) {
	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}
	return &CompiledJQ{code: code}, nil
}

// RunRecord runs the compiled program over a single record and returns every
// value it emits, in order. A record that yields no output — a `select(...)`
// that does not match, or `empty` — returns an empty slice: that is exactly how
// filtering drops a row. A runtime error from the program (e.g. a type error)
// is returned so the caller can surface it rather than silently skipping rows.
//
// The record is normalised through encoding/json first so values produced by any
// reader (Parquet/CSV typed columns, time values) run cleanly through gojq, which
// only accepts nil/bool/int/float64/string/[]any/map[string]any inputs.
func (c *CompiledJQ) RunRecord(rec map[string]interface{}) ([]interface{}, error) {
	generic, err := toGeneric(rec)
	if err != nil {
		return nil, fmt.Errorf("failed to apply --jq filter: %w", err)
	}
	iter := c.code.Run(generic)
	var out []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if runErr, ok := v.(error); ok {
			return nil, fmt.Errorf("failed to apply --jq filter: %w", runErr)
		}
		out = append(out, v)
	}
	return out, nil
}
