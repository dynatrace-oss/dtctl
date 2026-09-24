package dqlhint

import (
	"strings"
	"testing"
)

func TestSnippet(t *testing.T) {
	long := "fetch logs | filter " + strings.Repeat("a == 1 and ", 20) + "b = 2"
	tests := []struct {
		name  string
		query string
		span  Span
		want  string
	}{
		{
			name:  "single token",
			query: "fetch logs | summarize count() by service.name",
			span:  Span{StartLine: 1, StartColumn: 32, EndLine: 1, EndColumn: 33},
			want:  "fetch logs | summarize count() by service.name\n" + strings.Repeat(" ", 31) + "^^",
		},
		{
			name:  "zero-width end falls back to one caret",
			query: `fetch logs | filter x = 1`,
			span:  Span{StartLine: 1, StartColumn: 23},
			want:  "fetch logs | filter x = 1\n" + strings.Repeat(" ", 22) + "^",
		},
		{
			name:  "only the offending line of a multi-line query",
			query: "fetch logs\n| limit 3\n| summarize count() by service.name",
			span:  Span{StartLine: 3, StartColumn: 21, EndLine: 3, EndColumn: 22},
			want:  "| summarize count() by service.name\n" + strings.Repeat(" ", 20) + "^^",
		},
		{
			name:  "span across lines is marked to the end of the first line",
			query: "fetch logs\n| summarize c: count(\n)",
			span:  Span{StartLine: 2, StartColumn: 13, EndLine: 3, EndColumn: 1},
			want:  "| summarize c: count(\n" + strings.Repeat(" ", 12) + "^^^^^^^^^",
		},
		{
			name:  "tabs keep the caret aligned",
			query: "fetch logs\n\t| summarize count() by x",
			span:  Span{StartLine: 2, StartColumn: 22, EndLine: 2, EndColumn: 23},
			want:  "\t| summarize count() by x\n\t" + strings.Repeat(" ", 20) + "^^",
		},
		{
			name:  "columns count characters, not bytes",
			query: `fetch logs | filter content == "größe" and x = 1`,
			span:  Span{StartLine: 1, StartColumn: 46, EndLine: 1, EndColumn: 46},
			want:  "fetch logs | filter content == \"größe\" and x = 1\n" + strings.Repeat(" ", 45) + "^",
		},
		{
			name:  "long line is windowed around the position",
			query: long,
			span:  Span{StartLine: 1, StartColumn: len(long) - 2, EndLine: 1, EndColumn: len(long) - 2},
			want: "…" + long[len(long)-3-snippetContext:] + "\n" +
				strings.Repeat(" ", 1+snippetContext) + "^",
		},
		{"line out of range", "fetch logs", Span{StartLine: 2, StartColumn: 1}, ""},
		{"column out of range", "fetch logs", Span{StartLine: 1, StartColumn: 40}, ""},
		{"no position", "fetch logs", Span{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Snippet(tt.query, tt.span); got != tt.want {
				t.Errorf("Snippet =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
