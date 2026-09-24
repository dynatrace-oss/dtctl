package output

import (
	"encoding/json"
	"io"
	"strings"

	toon "github.com/toon-format/toon-go"
)

// ToonPrinter prints output as TOON (Token-Oriented Object Notation).
// TOON is a compact, human-readable format optimised for LLM token efficiency.
//
// Because dtctl resource structs use `json` tags (not `toon` tags), the printer
// round-trips through encoding/json to obtain a map[string]any representation
// that preserves the json field names, then passes it to toon.Marshal.
type ToonPrinter struct {
	writer   io.Writer
	jqFilter string
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
	transformed, err := ApplyJQ(p.jqFilter, obj)
	if err != nil {
		return err
	}

	generic, err := toGeneric(transformed)
	if err != nil {
		return err
	}

	data, err := toon.Marshal(toonSafe(generic), toon.WithLengthMarkers(true))
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

// MarshalTOON encodes v as a TOON string using the same options as the TOON
// printer and the agent envelope's `-o toon` path, so every TOON payload dtctl
// emits comes out of one encoder. Exported for callers outside this package
// that embed a TOON-encoded payload inside the agent envelope (pkg/exec).
func MarshalTOON(v interface{}) (string, error) {
	generic, err := toGeneric(v)
	if err != nil {
		return "", err
	}
	return toon.MarshalString(toonSafe(generic), toon.WithLengthMarkers(true))
}

// toonSafe replaces C0 control characters other than \t, \n and \r in every
// string (keys and values) of a generic value with their Unicode Control
// Picture (U+2400 + c, e.g. ESC -> ␛). TOON only defines the escapes \\, \",
// \n, \r and \t, and the encoder rejects any other control character, so one
// ANSI color sequence in a log line would otherwise fail the whole output.
func toonSafe(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		return toonSafeString(t)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[toonSafeString(k)] = toonSafe(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = toonSafe(val)
		}
		return out
	default:
		return v
	}
}

func toonSafeString(s string) string {
	if strings.IndexFunc(s, isUnsupportedToonControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isUnsupportedToonControl(r) {
			return 0x2400 + r
		}
		return r
	}, s)
}

func isUnsupportedToonControl(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' && r != '\r'
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
