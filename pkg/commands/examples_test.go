package commands

import "testing"

func TestExampleAvailable(t *testing.T) {
	verbs := map[string]*MinimalVerb{
		"query": {},
		"get":   {Resources: []string{"slos", "recipes"}},
		"run":   {},
	}
	cases := map[string]bool{
		`dtctl query 'fetch logs'`:             true,
		`dtctl get slos  # definitions`:        true,
		`dtctl get workflow-executions`:        false, // resource hidden
		`dtctl run problems-active`:            true,  // run lists no resources
		`dtctl describe <resource> <id>`:       false, // verb hidden
		`dtctl get recipes --search "<words>"`: true,
	}
	for ex, want := range cases {
		if got := exampleAvailable(ex, verbs); got != want {
			t.Errorf("exampleAvailable(%q) = %v, want %v", ex, got, want)
		}
	}
}
