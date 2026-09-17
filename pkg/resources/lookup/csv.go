package lookup

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
)

// utf8BOM is the UTF-8 byte order mark that some editors (notably Excel on
// Windows/macOS) prepend when saving CSV files. The DPL parser used by the
// lookup upload API rejects the BOM as invalid input, so it must be stripped
// before auto-detecting the parse pattern; otherwise the BOM ends up inside
// the first column name and produces an unparseable pattern.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// csvDelimiter is a candidate field separator for the content sent to the
// upload API, together with the DPL literal that matches it inside a parse
// pattern.
type csvDelimiter struct {
	char    rune
	literal string // DPL literal, e.g. "','"
	display string // human-readable name used in messages
}

// csvDelimiters lists the separators PrepareCSV may use, in preference order.
// A comma keeps the uploaded content byte-identical to the input whenever the
// data allows it; the other two are only used when a value contains the
// preceding candidates (for example a quoted "gamma, inc" cell). The ASCII
// unit separator is the last resort because it practically never occurs in
// CSV exports.
var csvDelimiters = []csvDelimiter{
	{char: ',', literal: `','`, display: ","},
	{char: '\t', literal: `'\t'`, display: "tab"},
	{char: '\x1f', literal: `'\u001f'`, display: "U+001F (unit separator)"},
}

// dplPlainFieldName matches column names that can be used unquoted as a DPL
// export name. Anything else (spaces, dashes, dots, …) has to be quoted, or
// the server rejects the pattern with "Named pattern element is not valid".
var dplPlainFieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PreparedCSV is the result of turning CSV input into something the lookup
// upload API can parse: a DPL pattern plus the exact content it applies to.
type PreparedCSV struct {
	// Pattern is the auto-detected DPL parse pattern for Content.
	Pattern string
	// SkippedRecords is the number of leading lines the server must skip
	// (the header row).
	SkippedRecords int
	// Content is the payload to upload. It is the input unchanged unless
	// Normalized is true.
	Content []byte
	// DataRecords is the number of data rows in the input, excluding the
	// header. It is the expected value of UploadResponse.Records.
	DataRecords int
	// Normalized reports whether Content was re-emitted instead of passed
	// through unchanged.
	Normalized bool
	// Delimiter is the human-readable separator used by Pattern/Content.
	Delimiter string
}

// PrepareCSV reads CSV data and returns the DPL parse pattern for it along
// with the content to upload.
//
// The pattern uses LD* (zero or more characters) per column: a bare LD needs
// at least one character, so any row with an empty cell fails to match and is
// dropped server-side without an error (issue #471).
//
// Quoted cells cannot be expressed with a comma-delimited DPL pattern at all —
// "gamma, inc" would be split across two columns. When a value contains the
// delimiter (or the input needs fixing up for other reasons: quoted cells,
// CRLF line endings, rows with trailing cells omitted) the CSV is re-emitted
// with the first separator that occurs in no value, and the pattern matches
// that separator instead.
//
// A leading UTF-8 BOM is stripped so it is not embedded in the first column
// name.
func PrepareCSV(data []byte) (*PreparedCSV, error) {
	data = bytes.TrimPrefix(data, utf8BOM)

	reader := csv.NewReader(bytes.NewReader(data))
	// Ragged rows are padded below rather than rejected, and quoting quirks
	// are passed through verbatim instead of failing the whole upload.
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w (use --parse-pattern for non-CSV input)", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	headers := rows[0]
	if len(headers) == 0 {
		return nil, fmt.Errorf("CSV file has no columns")
	}

	// Pad rows whose trailing cells were omitted, and reject rows with more
	// cells than the header: those would silently end up merged into the last
	// column.
	padded := false
	for i, row := range rows {
		switch {
		case len(row) > len(headers):
			return nil, fmt.Errorf("line %d has %d fields but the header declares %d columns; fix the input or use --parse-pattern", i+1, len(row), len(headers))
		case len(row) < len(headers):
			for len(row) < len(headers) {
				row = append(row, "")
			}
			rows[i] = row
			padded = true
		}
	}

	// DPL applies the pattern line by line, so a cell containing a line break
	// cannot be represented no matter which delimiter is used.
	for i, row := range rows {
		for j, field := range row {
			if strings.ContainsAny(field, "\r\n") {
				return nil, fmt.Errorf("line %d, column %q contains a line break; lookup upload parses line by line, so flatten the value or use --parse-pattern", i+1, columnLabel(headers, j))
			}
		}
	}

	delim, ok := pickCSVDelimiter(rows)
	if !ok {
		return nil, fmt.Errorf("every candidate delimiter (comma, tab, U+001F) occurs in the data; upload a pre-formatted file with --parse-pattern")
	}

	prepared := &PreparedCSV{
		Pattern:        buildCSVPattern(headers, delim),
		SkippedRecords: 1, // header row
		Content:        data,
		DataRecords:    len(rows) - 1,
		Delimiter:      delim.display,
	}

	// The input can only be uploaded unchanged if a plain comma join would
	// reproduce it: no quoting to unwrap, no CRLF to strip, no padding added.
	if delim.char != ',' || padded || bytes.ContainsRune(data, '"') || bytes.Contains(data, []byte("\r")) {
		prepared.Content = renderCSV(rows, delim.char)
		prepared.Normalized = true
	}

	return prepared, nil
}

// pickCSVDelimiter returns the first candidate delimiter that appears in no
// cell (column names included, since those go into the pattern).
func pickCSVDelimiter(rows [][]string) (csvDelimiter, bool) {
	for _, candidate := range csvDelimiters {
		used := false
		for _, row := range rows {
			for _, field := range row {
				if strings.ContainsRune(field, candidate.char) {
					used = true
					break
				}
			}
			if used {
				break
			}
		}
		if !used {
			return candidate, true
		}
	}
	return csvDelimiter{}, false
}

// buildCSVPattern renders the DPL pattern for the given header row, e.g.
// `LD*:id ',' LD*:"full name"`.
func buildCSVPattern(headers []string, delim csvDelimiter) string {
	parts := make([]string, 0, len(headers))
	for i := range headers {
		parts = append(parts, "LD*:"+dplFieldName(columnLabel(headers, i)))
	}
	return strings.Join(parts, " "+delim.literal+" ")
}

// columnLabel returns the name of column i, falling back to a positional name
// for blank headers.
func columnLabel(headers []string, i int) string {
	if i < len(headers) {
		if name := strings.TrimSpace(headers[i]); name != "" {
			return name
		}
	}
	return fmt.Sprintf("column_%d", i+1)
}

// dplFieldName quotes a column name for use as a DPL export name unless it is
// a plain identifier. Double quotes and backslashes cannot be expressed in a
// quoted name, so they are replaced.
func dplFieldName(name string) string {
	if dplPlainFieldName.MatchString(name) {
		return name
	}
	replacer := strings.NewReplacer(`"`, "_", `\`, "_")
	return `"` + replacer.Replace(name) + `"`
}

// renderCSV joins already-parsed rows with delim and LF line endings. Callers
// must have verified that no cell contains delim or a line break, so no
// quoting is needed — which is the point: the uploaded content is then
// unambiguous for a DPL pattern.
func renderCSV(rows [][]string, delim rune) []byte {
	var buf bytes.Buffer
	sep := string(delim)
	for _, row := range rows {
		buf.WriteString(strings.Join(row, sep))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// countDataRecords counts the non-empty lines in data. It is the record count
// baseline for uploads with a caller-supplied parse pattern, where dtctl does
// not know the input format.
func countDataRecords(data []byte) int {
	data = bytes.TrimPrefix(data, utf8BOM)
	count := 0
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			count++
		}
	}
	return count
}
