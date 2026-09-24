package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

func TestResolveSeriesOptions(t *testing.T) {
	summary := output.SeriesMode{Kind: output.SeriesSummary}
	full := output.SeriesMode{Kind: output.SeriesFull}
	tests := []struct {
		name      string
		series    string // "" = flag not given
		precision int
		precSet   bool
		format    string
		agent     bool
		want      seriesOptions
		wantErr   string
	}{
		{name: "human default is full, unrounded", format: "table", want: seriesOptions{Mode: full}},
		{name: "human summary json", series: "summary", format: "json", want: seriesOptions{Mode: summary}},
		{name: "downsample csv", series: "downsample:30", format: "csv", want: seriesOptions{Mode: output.SeriesMode{Kind: output.SeriesDownsample, Points: 30}}},
		{name: "downsample feeds a chart", series: "downsample:40", format: "chart", want: seriesOptions{Mode: output.SeriesMode{Kind: output.SeriesDownsample, Points: 40}}},
		{name: "human precision alone", precision: 4, precSet: true, format: "json", want: seriesOptions{Mode: full, Precision: 4}},

		{name: "agent default: summary and 4 digits", format: "table", agent: true,
			want: seriesOptions{Mode: summary, SeriesDefaulted: true, Precision: exec.AgentDefaultPrecision, PrecisionDefaulted: true}},
		{name: "agent opt-out restores full, unrounded", series: "full", precision: 0, precSet: true, format: "table", agent: true,
			want: seriesOptions{Mode: full}},
		{name: "agent explicit summary is not a default", series: "summary", format: "json", agent: true,
			want: seriesOptions{Mode: summary, Precision: exec.AgentDefaultPrecision, PrecisionDefaulted: true}},
		{name: "agent explicit precision wins", precision: 6, precSet: true, format: "table", agent: true,
			want: seriesOptions{Mode: summary, SeriesDefaulted: true, Precision: 6}},
		{name: "agent chart keeps the full series", format: "chart", agent: true,
			want: seriesOptions{Mode: full, Precision: exec.AgentDefaultPrecision, PrecisionDefaulted: true}},
		{name: "agent parquet export is left alone", format: "parquet", agent: true, want: seriesOptions{Mode: full}},

		{name: "bad series", series: "tiny", format: "json", wantErr: "invalid --series"},
		{name: "explicit summary cannot be charted", series: "summary", format: "sparkline", wantErr: "--series=summary"},
		{name: "negative precision", precision: -1, precSet: true, format: "json", wantErr: "--precision"},
		{name: "absurd precision", precision: 18, precSet: true, format: "json", wantErr: "--precision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSeriesOptions(tt.series, tt.series != "", tt.precision, tt.precSet, tt.format, tt.agent)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestQuerySeriesFlagsAreExperimental(t *testing.T) {
	// Both flags change the numbers a caller receives, so they ship
	// experimental on the stable query command until the shape settles.
	for _, name := range []string{"series", "precision"} {
		f := queryCmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("query has no --%s flag", name)
		}
		if got := f.Annotations[stability.AnnotationLevel]; len(got) == 0 || got[0] != string(stability.Experimental) {
			t.Errorf("--%s stability annotation = %v, want experimental", name, got)
		}
	}
}
