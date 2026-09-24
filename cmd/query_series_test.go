package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

func TestResolveSeriesOptions(t *testing.T) {
	tests := []struct {
		name      string
		series    string
		precision int
		format    string
		want      output.SeriesMode
		wantErr   string
	}{
		{name: "default is full", format: "json", want: output.SeriesMode{Kind: output.SeriesFull}},
		{name: "summary json", series: "summary", format: "json", want: output.SeriesMode{Kind: output.SeriesSummary}},
		{name: "summary table", series: "summary", format: "table", want: output.SeriesMode{Kind: output.SeriesSummary}},
		{name: "downsample csv", series: "downsample:30", format: "csv", want: output.SeriesMode{Kind: output.SeriesDownsample, Points: 30}},
		{name: "downsample feeds a chart", series: "downsample:40", format: "chart", want: output.SeriesMode{Kind: output.SeriesDownsample, Points: 40}},
		{name: "precision alone", precision: 4, format: "json", want: output.SeriesMode{Kind: output.SeriesFull}},
		{name: "bad series", series: "tiny", format: "json", wantErr: "invalid --series"},
		{name: "summary cannot be charted", series: "summary", format: "sparkline", wantErr: "--series=summary"},
		{name: "negative precision", precision: -1, format: "json", wantErr: "--precision"},
		{name: "absurd precision", precision: 18, format: "json", wantErr: "--precision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSeriesOptions(tt.series, tt.precision, tt.format)
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
