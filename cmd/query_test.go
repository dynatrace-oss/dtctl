package cmd

import "testing"

func TestIsSupportedQueryOutputFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{name: "default", format: "", want: true},
		{name: "json", format: "json", want: true},
		{name: "yaml alias", format: "yml", want: true},
		{name: "chart", format: "chart", want: true},
		{name: "spark alias", format: "spark", want: true},
		{name: "bar alias", format: "bar", want: true},
		{name: "braille alias", format: "br", want: true},
		{name: "trimmed and mixed case", format: " Json ", want: true},
		{name: "unsupported", format: "xml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedQueryOutputFormat(tt.format); got != tt.want {
				t.Fatalf("isSupportedQueryOutputFormat(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}
