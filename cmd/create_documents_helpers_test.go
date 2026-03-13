package cmd

import (
	"testing"
)

// --- itemName ---

func TestItemName(t *testing.T) {
	cases := []struct {
		docType string
		want    string
	}{
		{"dashboard", "tiles"},
		{"notebook", "sections"},
		{"launchpad", "sections"},
		{"", "sections"},
	}
	for _, tc := range cases {
		got := itemName(tc.docType)
		if got != tc.want {
			t.Errorf("itemName(%q) = %q, want %q", tc.docType, got, tc.want)
		}
	}
}

// --- capitalize ---

func TestCapitalize(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"hello", "Hello"},
		{"Hello", "Hello"},
		{"WORLD", "WORLD"},
		{"a", "A"},
		{"tiles", "Tiles"},
	}
	for _, tc := range cases {
		got := capitalize(tc.input)
		if got != tc.want {
			t.Errorf("capitalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
