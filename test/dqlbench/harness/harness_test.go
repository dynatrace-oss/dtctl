package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/dqlcost"
)

// fixturesDir returns the absolute path to the sibling fixtures/ directory.
// Using a relative path lets the test run from any working dir.
func fixturesDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../fixtures")
	if err != nil {
		t.Fatalf("resolve fixtures dir: %v", err)
	}
	return p
}

func TestFixtures_Snapshot(t *testing.T) {
	fixtures, err := LoadFixtures(fixturesDir(t))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures loaded")
	}

	for _, f := range fixtures {
		t.Run(f.ID, func(t *testing.T) {
			// 1. Expected-rule assertions on raw query.
			findings := dqlcost.Lint(f.Query)
			for _, want := range f.ExpectRules {
				if !hasRule(findings, want) {
					t.Errorf("expected rule %s to fire on raw query, got %+v", want, findings)
				}
			}

			// 2. Rewrite then re-lint; specified rules must no longer fire.
			rewritten, _ := dqlcost.Rewrite(f.Query, dqlcost.DefaultRewriteOptions())
			post := dqlcost.Lint(rewritten)
			for _, bad := range f.AfterRewriteNoRules {
				if hasRule(post, bad) {
					t.Errorf("rule %s should NOT fire after rewrite; query=%q findings=%+v", bad, rewritten, post)
				}
			}

			// 3. must_contain / must_not_contain on raw query.
			for _, s := range f.MustContain {
				if !strings.Contains(f.Query, s) {
					t.Errorf("query missing must_contain %q", s)
				}
			}
			for _, s := range f.MustNotContain {
				if strings.Contains(f.Query, s) {
					t.Errorf("query contains forbidden %q", s)
				}
			}
		})
	}
}

func hasRule(findings []dqlcost.Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
