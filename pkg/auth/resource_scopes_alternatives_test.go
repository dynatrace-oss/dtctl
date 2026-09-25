package auth

import (
	"reflect"
	"testing"
)

func TestAlternativeScopesForResource(t *testing.T) {
	want := [][]string{{"app-engine:functions:run"}, {"platform-management:environments:read"}}
	for _, r := range []string{"environment", "environments", "license", "license-settings"} {
		if got := AlternativeScopesForResource(r, AccessRead); !reflect.DeepEqual(got, want) {
			t.Errorf("AlternativeScopesForResource(%q, read) = %v, want %v", r, got, want)
		}
		// The listed scope is unchanged: it is what login requests.
		if got := ScopesForResource(r, AccessRead); !reflect.DeepEqual(got, []string{"app-engine:apps:run"}) {
			t.Errorf("ScopesForResource(%q, read) = %v", r, got)
		}
	}
	if got := AlternativeScopesForResource("workflow", AccessRead); got != nil {
		t.Errorf("AlternativeScopesForResource(workflow, read) = %v, want nil", got)
	}
	if got := AlternativeScopesForResource("does-not-exist", AccessRead); got != nil {
		t.Errorf("AlternativeScopesForResource(unknown) = %v, want nil", got)
	}
}

func TestAlternativesForDeleteFallsBackToWrite(t *testing.T) {
	as := AccessScopes{
		Write:        []string{"w"},
		Alternatives: map[Access][][]string{AccessWrite: {{"w2"}}},
	}
	if got := as.AlternativesFor(AccessDelete); !reflect.DeepEqual(got, [][]string{{"w2"}}) {
		t.Errorf("AlternativesFor(delete) = %v, want the write alternatives", got)
	}
	as.Delete = []string{"d"}
	if got := as.AlternativesFor(AccessDelete); got != nil {
		t.Errorf("AlternativesFor(delete) with a distinct delete list = %v, want nil", got)
	}
}

// An alternative must be a non-empty set of non-empty scopes on an access level
// the entry actually lists: an empty set would be satisfied by every token.
func TestAlternativesAreWellFormed(t *testing.T) {
	for resource, as := range ResourceScopes {
		for access, alts := range as.Alternatives {
			if len(as.For(access)) == 0 {
				t.Errorf("ResourceScopes[%q] has alternatives for %s but lists no %s scopes", resource, access, access)
			}
			for i, alt := range alts {
				if len(alt) == 0 {
					t.Errorf("ResourceScopes[%q] %s alternative %d is empty", resource, access, i)
				}
				for _, s := range alt {
					if s == "" {
						t.Errorf("ResourceScopes[%q] %s alternative %d has an empty scope", resource, access, i)
					}
				}
			}
		}
	}
}
