package lookup

import (
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid path",
			path:    "/lookups/grail/pm/error_codes",
			wantErr: false,
		},
		{
			name:    "valid path with underscores",
			path:    "/lookups/my_lookup/test_data",
			wantErr: false,
		},
		{
			name:    "valid path with hyphens",
			path:    "/lookups/my-lookup/test-data",
			wantErr: false,
		},
		{
			name:    "valid path with dots",
			path:    "/lookups/grail/pm/data.csv",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "missing /lookups/ prefix",
			path:    "/data/table",
			wantErr: true,
			errMsg:  "must start with /lookups/",
		},
		{
			name:    "no /lookups/ prefix",
			path:    "lookups/test",
			wantErr: true,
			errMsg:  "must start with /lookups/",
		},
		{
			name:    "ends with slash",
			path:    "/lookups/test/",
			wantErr: true,
			errMsg:  "must end with alphanumeric",
		},
		{
			name:    "ends with hyphen",
			path:    "/lookups/test-",
			wantErr: true,
			errMsg:  "must end with alphanumeric",
		},
		{
			name:    "only one slash",
			path:    "/lookups",
			wantErr: true,
			errMsg:  "must start with /lookups/",
		},
		{
			name:    "invalid character (space)",
			path:    "/lookups/my data/table",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "invalid character (@)",
			path:    "/lookups/test@data/table",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "too long path",
			path:    "/lookups/" + string(make([]byte, 500)),
			wantErr: true,
			errMsg:  "must not exceed 500 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidatePath() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestDetectCSVPattern(t *testing.T) {
	tests := []struct {
		name               string
		csvData            string
		wantPattern        string
		wantSkippedRecords int
		wantErr            bool
	}{
		{
			name:               "simple CSV",
			csvData:            "id,name,value\n1,Alice,100\n2,Bob,200",
			wantPattern:        "LD:id ',' LD:name ',' LD:value",
			wantSkippedRecords: 1,
			wantErr:            false,
		},
		{
			name:               "CSV with spaces in headers",
			csvData:            "user id,full name,score\n1,Alice,100",
			wantPattern:        "LD:user id ',' LD:full name ',' LD:score",
			wantSkippedRecords: 1,
			wantErr:            false,
		},
		{
			name:               "single column CSV",
			csvData:            "id\n1\n2\n3",
			wantPattern:        "LD:id",
			wantSkippedRecords: 1,
			wantErr:            false,
		},
		{
			name:               "CSV with empty header",
			csvData:            "id,,value\n1,2,3",
			wantPattern:        "LD:id ',' LD:column_2 ',' LD:value",
			wantSkippedRecords: 1,
			wantErr:            false,
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
			pattern, skipped, err := DetectCSVPattern([]byte(tt.csvData))
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectCSVPattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if pattern != tt.wantPattern {
					t.Errorf("DetectCSVPattern() pattern = %v, want %v", pattern, tt.wantPattern)
				}
				if skipped != tt.wantSkippedRecords {
					t.Errorf("DetectCSVPattern() skippedRecords = %v, want %v", skipped, tt.wantSkippedRecords)
				}
			}
		})
	}
}

func TestDetectCSVPattern_MultiColumn(t *testing.T) {
	csvData := "col1,col2,col3,col4,col5\na,b,c,d,e"
	pattern, skipped, err := DetectCSVPattern([]byte(csvData))

	if err != nil {
		t.Fatalf("DetectCSVPattern() unexpected error: %v", err)
	}

	expected := "LD:col1 ',' LD:col2 ',' LD:col3 ',' LD:col4 ',' LD:col5"
	if pattern != expected {
		t.Errorf("DetectCSVPattern() pattern = %v, want %v", pattern, expected)
	}

	if skipped != 1 {
		t.Errorf("DetectCSVPattern() skippedRecords = %v, want 1", skipped)
	}
}

func TestDetectCSVPattern_QuotedHeaders(t *testing.T) {
	csvData := `"First Name","Last Name","Email Address"
"John","Doe","john@example.com"`

	pattern, skipped, err := DetectCSVPattern([]byte(csvData))

	if err != nil {
		t.Fatalf("DetectCSVPattern() unexpected error: %v", err)
	}

	expected := "LD:First Name ',' LD:Last Name ',' LD:Email Address"
	if pattern != expected {
		t.Errorf("DetectCSVPattern() pattern = %v, want %v", pattern, expected)
	}

	if skipped != 1 {
		t.Errorf("DetectCSVPattern() skippedRecords = %v, want 1", skipped)
	}
}

func TestHandleUploadError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		path       string
		wantMsg    string
	}{
		{
			name:       "bad request",
			statusCode: 400,
			body:       "invalid data",
			path:       "/lookups/test",
			wantMsg:    "invalid upload request",
		},
		{
			name:       "forbidden",
			statusCode: 403,
			body:       "access denied",
			path:       "/lookups/test",
			wantMsg:    "access denied to write file",
		},
		{
			name:       "conflict",
			statusCode: 409,
			body:       "already exists",
			path:       "/lookups/test",
			wantMsg:    "already exists",
		},
		{
			name:       "file too large",
			statusCode: 413,
			body:       "too large",
			path:       "/lookups/test",
			wantMsg:    "file size exceeds maximum limit",
		},
		{
			name:       "server error",
			statusCode: 500,
			body:       "internal error",
			path:       "/lookups/test",
			wantMsg:    "upload failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleUploadError(tt.statusCode, tt.body, tt.path)
			if err == nil {
				t.Error("handleUploadError() expected error, got nil")
				return
			}
			if !contains(err.Error(), tt.wantMsg) {
				t.Errorf("handleUploadError() error = %v, want error containing %q", err, tt.wantMsg)
			}
		})
	}
}

func TestHandleDeleteError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		path       string
		wantMsg    string
	}{
		{
			name:       "bad request",
			statusCode: 400,
			body:       "invalid request",
			path:       "/lookups/test",
			wantMsg:    "invalid delete request",
		},
		{
			name:       "forbidden",
			statusCode: 403,
			body:       "access denied",
			path:       "/lookups/test",
			wantMsg:    "access denied to delete file",
		},
		{
			name:       "not found",
			statusCode: 404,
			body:       "not found",
			path:       "/lookups/test",
			wantMsg:    "not found",
		},
		{
			name:       "server error",
			statusCode: 500,
			body:       "internal error",
			path:       "/lookups/test",
			wantMsg:    "delete failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleDeleteError(tt.statusCode, tt.body, tt.path)
			if err == nil {
				t.Error("handleDeleteError() expected error, got nil")
				return
			}
			if !contains(err.Error(), tt.wantMsg) {
				t.Errorf("handleDeleteError() error = %v, want error containing %q", err, tt.wantMsg)
			}
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{
			name:      "invalid timestamp",
			timestamp: "invalid",
			want:      "invalid",
		},
		{
			name:      "empty timestamp",
			timestamp: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimestamp(tt.timestamp)
			if got != tt.want {
				t.Errorf("formatTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
