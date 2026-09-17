package lookup

import (
	"strings"
	"testing"
)

func TestPrepareCSV_Pattern(t *testing.T) {
	tests := []struct {
		name               string
		csvData            string
		wantPattern        string
		wantSkippedRecords int
		wantRecords        int
		wantErr            bool
	}{
		{
			name:               "simple CSV",
			csvData:            "id,name,value\n1,Alice,100\n2,Bob,200",
			wantPattern:        "LD*:id ',' LD*:name ',' LD*:value",
			wantSkippedRecords: 1,
			wantRecords:        2,
		},
		{
			// Column names that are not plain identifiers must be quoted:
			// `LD*:user id` is rejected with "Named pattern element 'id' is
			// not valid".
			name:               "CSV with spaces in headers",
			csvData:            "user id,full name,score\n1,Alice,100",
			wantPattern:        `LD*:"user id" ',' LD*:"full name" ',' LD*:score`,
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			name:               "headers with dashes and dots",
			csvData:            "my-col,a.b\n1,2",
			wantPattern:        `LD*:"my-col" ',' LD*:"a.b"`,
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			name:               "single column CSV",
			csvData:            "id\n1\n2\n3",
			wantPattern:        "LD*:id",
			wantSkippedRecords: 1,
			wantRecords:        3,
		},
		{
			name:               "CSV with empty header",
			csvData:            "id,,value\n1,2,3",
			wantPattern:        "LD*:id ',' LD*:column_2 ',' LD*:value",
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			// Regression test for #187: Excel on macOS/Windows prepends a
			// UTF-8 BOM (0xEF 0xBB 0xBF) when saving as CSV. Without
			// stripping it, the first column name became "\ufeffcode" and the
			// server-side DPL parser rejected the pattern.
			name:               "CSV with UTF-8 BOM",
			csvData:            "\ufeffcode,description,severity,action\nERR001,timeout,critical,page",
			wantPattern:        "LD*:code ',' LD*:description ',' LD*:severity ',' LD*:action",
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			name:               "CSV with BOM and CRLF line endings",
			csvData:            "\ufeffid,name\r\n1,alice\r\n2,bob",
			wantPattern:        "LD*:id ',' LD*:name",
			wantSkippedRecords: 1,
			wantRecords:        2,
		},
		{
			name:               "BOM-only single column CSV",
			csvData:            "\ufeffid\n1",
			wantPattern:        "LD*:id",
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			name:               "quoted headers",
			csvData:            "\"First Name\",\"Last Name\",\"Email Address\"\n\"John\",\"Doe\",\"john@example.invalid\"",
			wantPattern:        `LD*:"First Name" ',' LD*:"Last Name" ',' LD*:"Email Address"`,
			wantSkippedRecords: 1,
			wantRecords:        1,
		},
		{
			name:    "empty CSV",
			csvData: "",
			wantErr: true,
		},
		{
			name:    "CSV with only newlines",
			csvData: "\n\n\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := PrepareCSV([]byte(tt.csvData))
			if (err != nil) != tt.wantErr {
				t.Fatalf("PrepareCSV() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if prepared.Pattern != tt.wantPattern {
				t.Errorf("Pattern = %q, want %q", prepared.Pattern, tt.wantPattern)
			}
			if prepared.SkippedRecords != tt.wantSkippedRecords {
				t.Errorf("SkippedRecords = %d, want %d", prepared.SkippedRecords, tt.wantSkippedRecords)
			}
			if prepared.DataRecords != tt.wantRecords {
				t.Errorf("DataRecords = %d, want %d", prepared.DataRecords, tt.wantRecords)
			}
		})
	}
}

// TestPrepareCSV_EmptyCellsUseZeroOrMore is the core of #471: a bare LD needs
// at least one character, so every row with an empty cell failed to match and
// was dropped server-side without an error. LD{0,1} — the fix suggested in the
// issue — is not the answer either: it caps the cell at a single character.
func TestPrepareCSV_EmptyCellsUseZeroOrMore(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name,owner\n1,alpha,\n2,beta,team-b\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}

	want := "LD*:id ',' LD*:name ',' LD*:owner"
	if prepared.Pattern != want {
		t.Errorf("Pattern = %q, want %q", prepared.Pattern, want)
	}
	if strings.Contains(prepared.Pattern, "LD{") {
		t.Errorf("Pattern uses a length-capped matcher: %q", prepared.Pattern)
	}
	if prepared.Normalized {
		t.Errorf("Normalized = true, want false: plain CSV must be uploaded unchanged")
	}
	if prepared.DataRecords != 2 {
		t.Errorf("DataRecords = %d, want 2", prepared.DataRecords)
	}
}

// TestPrepareCSV_QuotedDelimiter covers the second half of #471: a cell such
// as "gamma, inc" cannot be expressed in a comma-delimited DPL pattern, so the
// content is re-emitted with a separator that occurs in no cell.
func TestPrepareCSV_QuotedDelimiter(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name,owner\n1,alpha,\n3,\"gamma, inc\",team-c\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}

	if !prepared.Normalized {
		t.Fatal("Normalized = false, want true for a quoted cell containing the delimiter")
	}
	wantPattern := "LD*:id '\\t' LD*:name '\\t' LD*:owner"
	if prepared.Pattern != wantPattern {
		t.Errorf("Pattern = %q, want %q", prepared.Pattern, wantPattern)
	}
	wantContent := "id\tname\towner\n1\talpha\t\n3\tgamma, inc\tteam-c\n"
	if string(prepared.Content) != wantContent {
		t.Errorf("Content = %q, want %q", prepared.Content, wantContent)
	}
	if prepared.DataRecords != 2 {
		t.Errorf("DataRecords = %d, want 2", prepared.DataRecords)
	}
}

// TestPrepareCSV_TabInQuotedCell falls through to the unit separator: the cell
// contains both candidate delimiters.
func TestPrepareCSV_TabInQuotedCell(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name\n1,\"a,\tb\"\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}

	wantPattern := `LD*:id '\u001f' LD*:name`
	if prepared.Pattern != wantPattern {
		t.Errorf("Pattern = %q, want %q", prepared.Pattern, wantPattern)
	}
	wantContent := "id\x1fname\n1\x1fa,\tb\n"
	if string(prepared.Content) != wantContent {
		t.Errorf("Content = %q, want %q", prepared.Content, wantContent)
	}
}

func TestPrepareCSV_QuotesStrippedFromValues(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name\n1,\"alpha\"\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}

	// Without re-emitting, the server would store the value with its quotes.
	if !prepared.Normalized {
		t.Fatal("Normalized = false, want true for quoted values")
	}
	if got, want := string(prepared.Content), "id,name\n1,alpha\n"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
	if prepared.Pattern != "LD*:id ',' LD*:name" {
		t.Errorf("Pattern = %q, want comma-delimited pattern", prepared.Pattern)
	}
}

// TestPrepareCSV_CRLFNormalized keeps the trailing CR out of the last column.
func TestPrepareCSV_CRLFNormalized(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name\r\n1,alice\r\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}
	if !prepared.Normalized {
		t.Fatal("Normalized = false, want true for CRLF input")
	}
	if got, want := string(prepared.Content), "id,name\n1,alice\n"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

// TestPrepareCSV_ShortRowsPadded covers rows whose trailing cells were omitted
// entirely, which the auto-detected pattern cannot match.
func TestPrepareCSV_ShortRowsPadded(t *testing.T) {
	prepared, err := PrepareCSV([]byte("id,name,owner\n1,alpha\n2,beta,team-b\n"))
	if err != nil {
		t.Fatalf("PrepareCSV() error = %v", err)
	}
	if !prepared.Normalized {
		t.Fatal("Normalized = false, want true for a short row")
	}
	if got, want := string(prepared.Content), "id,name,owner\n1,alpha,\n2,beta,team-b\n"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
	if prepared.DataRecords != 2 {
		t.Errorf("DataRecords = %d, want 2", prepared.DataRecords)
	}
}

func TestPrepareCSV_Errors(t *testing.T) {
	tests := []struct {
		name    string
		csvData string
		wantMsg string
	}{
		{
			name:    "row with extra fields",
			csvData: "id,name\n1,alpha,extra\n",
			wantMsg: "line 2 has 3 fields but the header declares 2 columns",
		},
		{
			name:    "cell with a line break",
			csvData: "id,name\n1,\"alpha\nbeta\"\n",
			wantMsg: "contains a line break",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PrepareCSV([]byte(tt.csvData))
			if err == nil {
				t.Fatal("PrepareCSV() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestCountDataRecords(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int
	}{
		{name: "trailing newline", data: "a\nb\nc\n", want: 3},
		{name: "no trailing newline", data: "a\nb", want: 2},
		{name: "blank lines ignored", data: "a\n\n\nb\n", want: 2},
		{name: "BOM stripped", data: "\ufeffa\n", want: 1},
		{name: "empty", data: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countDataRecords([]byte(tt.data)); got != tt.want {
				t.Errorf("countDataRecords() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUploadResponse_CheckRecordCount(t *testing.T) {
	tests := []struct {
		name        string
		resp        UploadResponse
		wantWarning string
		wantErr     bool
	}{
		{
			name: "all records stored",
			resp: UploadResponse{Records: 3, PatternMatches: 3, InputRecords: 3},
		},
		{
			name: "input count unknown",
			resp: UploadResponse{Records: 0},
		},
		{
			name: "duplicates account for the difference",
			resp: UploadResponse{Records: 2, PatternMatches: 3, DiscardedDuplicates: 1, InputRecords: 3},
		},
		{
			// The reported shape of #471: 2514 input lines, 2 stored.
			name:        "most records dropped",
			resp:        UploadResponse{Records: 2, PatternMatches: 2, InputRecords: 2514},
			wantWarning: "only 2 of 2514 records were stored; 2512 line(s) did not match",
		},
		{
			name:    "nothing stored",
			resp:    UploadResponse{Records: 0, InputRecords: 10},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := tt.resp.CheckRecordCount()
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckRecordCount() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantWarning == "" && warning != "" {
				t.Errorf("CheckRecordCount() warning = %q, want none", warning)
			}
			if tt.wantWarning != "" && !strings.Contains(warning, tt.wantWarning) {
				t.Errorf("CheckRecordCount() warning = %q, want it to contain %q", warning, tt.wantWarning)
			}
		})
	}
}
