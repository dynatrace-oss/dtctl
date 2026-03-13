package cmd

import (
	"testing"
)

// --- formatDuration ---

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0s"},
		{45, "45s"},
		{59, "59s"},
		{60, "1m"},
		{90, "1m30s"},
		{120, "2m"},
		{3600, "1h"},
		{3660, "1h1m"},
		{7200, "2h"},
		{7320, "2h2m"},
		{3599, "59m59s"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.input)
		if got != tc.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- formatBytes ---

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tc := range cases {
		got := formatBytes(tc.input)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- stringFromRecord ---

func TestStringFromRecord(t *testing.T) {
	record := map[string]interface{}{
		"name":   "server-01",
		"count":  42.0,
		"active": true,
	}
	cases := []struct {
		key  string
		want string
	}{
		{"name", "server-01"},
		{"missing", ""},
		{"count", "42"},    // non-string → fmt.Sprintf("%v", ...)
		{"active", "true"}, // non-string → fmt.Sprintf("%v", ...)
	}
	for _, tc := range cases {
		got := stringFromRecord(record, tc.key)
		if got != tc.want {
			t.Errorf("stringFromRecord(record, %q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}
