package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestExecutionsCapAdvice(t *testing.T) {
	// Below the cap a bigger --limit fetches the rest.
	below := executionsCapAdvice(100, 2297, 100)
	if len(below) != 1 || !strings.Contains(below[0], "Raise --limit") {
		t.Errorf("below the cap: %q", below)
	}
	// At the cap it cannot: the advice must not offer --limit and must
	// offer the uncapped count query instead.
	for _, limit := range []int64{1000, 5000, 0} {
		at := executionsCapAdvice(1000, 2297, limit)
		if strings.Contains(strings.Join(at, " "), "Raise --limit") || len(at) != 2 || !strings.HasPrefix(at[1], "dtctl query 'fetch dt.system.events") {
			t.Errorf("limit %d: %q", limit, at)
		}
	}
}

func TestParseExecTimeDurationAgo(t *testing.T) {
	got, err := parseExecTime("7d", false)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(ts); d < 7*24*time.Hour-time.Minute || d > 7*24*time.Hour+time.Minute {
		t.Errorf("parseExecTime(7d) = %s, %v ago", got, d)
	}
}
