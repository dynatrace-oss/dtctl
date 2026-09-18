package stability_test

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/cmd"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// TestSinceVersionsNameThisReleaseOrTheNextOne is the guard on the one part of
// the manifest a release can falsify on its own.
//
// A since-version is a claim about a release: `experimental since 0.39.0` tells
// a caller which version withdrew the guarantee, and 62 lines of
// docs/STABILITY.md make that claim about a version that does not exist yet.
// Nothing connected those constants to the version the release actually gets.
// If the next tag were cut as 0.40.0 -- an extra release in between, a
// hand-edited manifest, a different bump than release-please's
// bump-minor-pre-major produced -- every one of those lines would name a
// release that had already shipped without the demotion, and the manifest would
// be wrong in the direction that matters: it would understate how long the
// surface was still guaranteed.
//
// So a declared since-version may only be this release or the next one. That
// makes the check fail on release-please's own version-bump PR, which is the
// last moment it can be fixed for free, rather than after the tag exists.
func TestSinceVersionsNameThisReleaseOrTheNextOne(t *testing.T) {
	current, err := parseVersion(version.Version)
	if err != nil {
		t.Fatalf("pkg/version.Version is not a release version: %v", err)
	}

	// release-please runs with bump-minor-pre-major, so below 1.0 the next
	// release is the next patch (fixes only) or the next minor (any feature).
	// Both are legitimate for a declaration written today; anything else names
	// the wrong release.
	allowed := map[[3]int]string{
		current:                                  "this release",
		{current[0], current[1], current[2] + 1}: "the next patch release",
		{current[0], current[1] + 1, 0}:          "the next minor release",
	}

	for since, users := range cmd.StabilitySinceVersions() {
		v, err := parseVersion(since)
		if err != nil {
			t.Errorf("stability-since %q is not a version (declared by %s)", since, first(users))
			continue
		}
		if _, ok := allowed[v]; ok {
			continue
		}
		sort.Strings(users)
		shown := users
		if len(shown) > 8 {
			shown = append(shown[:8:8], fmt.Sprintf("and %d more", len(users)-8))
		}
		t.Errorf("%d declaration(s) name stability-since %s, but pkg/version.Version is %s.\n"+
			"A since-version may only be this release or the next one, because the badge is a "+
			"claim about which release changed the contract.\n"+
			"If the release numbering changed, update the declarations (cmd/stability_pre_1_0.go, "+
			"cmd/breakpoint_helpers.go, cmd/inventory.go) and run `make stability-manifest`.\n"+
			"Declared by: %s",
			len(users), since, version.Version, strings.Join(shown, ", "))
	}
}

// parseVersion accepts X.Y.Z with an optional prerelease suffix, which is
// dropped: 0.39.0-rc.1 is a candidate for the 0.39.0 contract, not a release of
// its own.
func parseVersion(s string) ([3]int, error) {
	var out [3]int
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("%q is not X.Y.Z", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("%q is not X.Y.Z", s)
		}
		out[i] = n
	}
	return out, nil
}

func first(s []string) string {
	if len(s) == 0 {
		return "(nothing)"
	}
	sort.Strings(s)
	return s[0]
}
