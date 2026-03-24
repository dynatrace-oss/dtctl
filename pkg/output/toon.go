package output

import (
	"encoding/json"
	"io"

	toon "github.com/toon-format/toon-go"
)

// ToonPrinter prints output as TOON (Token-Oriented Object Notation).
// TOON is a compact, human-readable format optimised for LLM token efficiency.
//
// Because dtctl resource structs use `json` tags (not `toon` tags), the printer
// round-trips through encoding/json to obtain a map[string]any representation
// that preserves the json field names, then passes it to toon.Marshal.
type ToonPrinter struct {
	writer io.Writer
}

// Print prints a single object as TOON.
func (p *ToonPrinter) Print(obj interface{}) error {
	return p.marshal(obj)
}

// PrintList prints a list of objects as TOON.
func (p *ToonPrinter) PrintList(obj interface{}) error {
	return p.marshal(obj)
}

// marshal converts obj to a json-tag-aware representation and encodes it as TOON.
func (p *ToonPrinter) marshal(obj interface{}) error {
	generic, err := toGeneric(obj)
	if err != nil {
		return err
	}

	data, err := toon.Marshal(generic, toon.WithLengthMarkers(true))
	if err != nil {
		return err
	}

	if _, err = p.writer.Write(data); err != nil {
		return err
	}
	// Append a trailing newline for consistency with JSON and YAML printers.
	_, err = p.writer.Write([]byte("\n"))
	return err
}

// toGeneric converts a typed Go value to an untyped representation
// (map[string]any / []any / primitives) by round-tripping through
// encoding/json. This ensures json struct tags are respected while
// producing a value that toon.Marshal can encode without needing
// `toon` struct tags on every resource struct.
func toGeneric(v interface{}) (interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic interface{}
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, err
	}
	return generic, nil
}
