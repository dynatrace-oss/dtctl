package cmd

import (
	"strings"
	"testing"
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
