package cmd

import (
	"testing"
)

func TestParseProfileTimestamp(t *testing.T) {
	cases := []struct {
		input   string
		wantMs  int64
		wantErr bool
	}{
		{"1700000000000", 1700000000000, false},
		{"2023-11-14T22:13:20Z", 1700000000000, false},
		{"not-a-time", 0, true},
		{"", 0, true},
	}

	for _, tc := range cases {
		got, err := parseProfileTimestamp(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseProfileTimestamp(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.wantMs {
			t.Errorf("parseProfileTimestamp(%q) = %d, want %d", tc.input, got, tc.wantMs)
		}
	}
}
