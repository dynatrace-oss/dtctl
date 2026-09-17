package output

import (
	"bytes"
	"encoding"
	"encoding/json"
	"io"
	"reflect"
)

// jsonIndent is the indentation the JSON printer emits, and therefore the
// indentation the spill byte measurement has to reproduce to stay faithful to
// what the invocation would print.
const jsonIndent = "  "

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// streamJSONArray writes v to w as an indented JSON array, encoding one element
// at a time. The bytes are identical to what
//
//	enc := json.NewEncoder(w); enc.SetIndent(prefix, indent); enc.Encode(v)
//
// produces, but peak memory is proportional to the largest single element
// instead of to the whole array: json.Encoder with an indent set marshals the
// entire value into one buffer and then re-indents it into a second one, so a
// large result costs several times its own serialised size in transient
// allocations (a 40 MB result measured ~500 MB of allocation and ~230 MB live).
//
// handled reports whether v was a shape this can stream. It is false — with w
// untouched — for anything whose JSON encoding is not "[" elements "]":
// non-slices, a nil slice (which marshals as null), byte slices (base64
// strings), and any slice type that carries its own Marshaler. The caller must
// fall back to encoding v as a whole value in that case. A non-nil error means
// an element failed to marshal *after* bytes were already written, so w holds a
// truncated array; callers that cannot tolerate that must discard the output.
func streamJSONArray(w io.Writer, v interface{}, prefix, indent string) (handled bool, err error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false, nil
		}
		rv = rv.Elem()
	}
	if !streamableAsJSONArray(rv) {
		return false, nil
	}

	if rv.Len() == 0 {
		// json.Indent leaves an empty array compact, so the whole encoding is "[]".
		_, werr := io.WriteString(w, "[]\n")
		return true, werr
	}

	// With neither a prefix nor an indent json.Encoder emits compact JSON and
	// breaks no lines at all, so the framing has to follow suit.
	nl, linePrefix, closePrefix := "", "", ""
	if prefix != "" || indent != "" {
		nl, linePrefix, closePrefix = "\n", prefix+indent, prefix
	}

	// One element is encoded at a time into buf. Indenting an element with the
	// prefix the array's members sit at (prefix+indent) reproduces exactly the
	// lines json.Indent would have produced for that member — its opening line
	// is unprefixed (we write the prefix ourselves), its inner lines and its
	// closing bracket carry it.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent(linePrefix, indent)

	if _, err := io.WriteString(w, "["+nl); err != nil {
		return true, err
	}
	for i := 0; i < rv.Len(); i++ {
		buf.Reset()
		if err := enc.Encode(rv.Index(i).Interface()); err != nil {
			return true, err
		}
		// Encode terminates the value with a newline; the separator that follows
		// the element supplies it instead.
		elem := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
		if _, err := io.WriteString(w, linePrefix); err != nil {
			return true, err
		}
		if _, err := w.Write(elem); err != nil {
			return true, err
		}
		sep := "," + nl
		if i == rv.Len()-1 {
			sep = nl
		}
		if _, err := io.WriteString(w, sep); err != nil {
			return true, err
		}
	}
	_, err = io.WriteString(w, closePrefix+"]\n")
	return true, err
}

// streamableAsJSONArray reports whether rv's JSON encoding is a plain array of
// independently encodable elements — the only shape streamJSONArray can frame
// itself. Everything it rejects encodes as something other than a bracketed
// element list and has to go through the whole-value encoder.
func streamableAsJSONArray(rv reflect.Value) bool {
	if !rv.IsValid() {
		return false
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	// A nil slice marshals as null, not as [].
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return false
	}
	// []byte and friends marshal as a base64 string.
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return false
	}
	// A slice type with its own Marshaler (json.RawMessage, a custom type) can
	// encode as anything at all.
	t := rv.Type()
	for _, iface := range []reflect.Type{jsonMarshalerType, textMarshalerType} {
		if t.Implements(iface) || reflect.PointerTo(t).Implements(iface) {
			return false
		}
	}
	return true
}
