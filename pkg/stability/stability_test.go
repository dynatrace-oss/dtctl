package stability

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/sdk/session"
)

// newTree builds a small command tree: a stable verb with a stable and an
// experimental flag, and an experimental verb with a stable-looking subcommand.
func newTree() (root, stable, experimental, nested *cobra.Command) {
	root = &cobra.Command{Use: "dtctl"}

	stable = &cobra.Command{Use: "query", RunE: func(*cobra.Command, []string) error { return nil }}
	stable.Flags().String("timeframe", "", "")
	stable.Flags().Bool("spill", false, "")
	MarkFlag(stable, "spill", Experimental, "0.38.0")
	root.AddCommand(stable)

	experimental = &cobra.Command{Use: "ingest", RunE: func(*cobra.Command, []string) error { return nil }}
	Mark(experimental, Experimental, "0.38.0")
	nested = &cobra.Command{Use: "logs", RunE: func(*cobra.Command, []string) error { return nil }}
	experimental.AddCommand(nested)
	root.AddCommand(experimental)

	return root, stable, experimental, nested
}

func TestLevelOrdering(t *testing.T) {
	if !Stable.AtLeast(Experimental) {
		t.Error("stable should satisfy an experimental floor")
	}
	if Experimental.AtLeast(Stable) {
		t.Error("experimental must not satisfy a stable floor")
	}
	if !Development.AtLeast(Development) {
		t.Error("a level must satisfy its own floor")
	}
	// The empty level means unset and must read as the default, so an
	// unannotated command is never accidentally the weakest thing in the tree.
	if got := Level("").Rank(); got != Stable.Rank() {
		t.Errorf("unset level ranks %d, want stable's %d", got, Stable.Rank())
	}
}

func TestEffectiveTakesTheWeakestAncestor(t *testing.T) {
	_, stable, experimental, nested := newTree()

	if got := Effective(stable); got != Stable {
		t.Errorf("Effective(query) = %q, want stable", got)
	}
	if got := Effective(experimental); got != Experimental {
		t.Errorf("Effective(ingest) = %q, want experimental", got)
	}
	// The whole point: an unannotated subcommand of an experimental verb is
	// stable in name only, and must not be advertised as stable.
	if got := Effective(nested); got != Experimental {
		t.Errorf("Effective(ingest logs) = %q, want experimental (inherited)", got)
	}
	if got := Of(nested); got != Stable {
		t.Errorf("Of(ingest logs) = %q, want stable (its own declaration)", got)
	}
}

func TestEffectiveFlagIsCappedByItsCommand(t *testing.T) {
	_, stable, experimental, _ := newTree()

	if got := EffectiveFlag(stable, "spill"); got != Experimental {
		t.Errorf("EffectiveFlag(query --spill) = %q, want experimental", got)
	}
	if got := EffectiveFlag(stable, "timeframe"); got != Stable {
		t.Errorf("EffectiveFlag(query --timeframe) = %q, want stable", got)
	}

	// A flag cannot be stronger than the command that carries it.
	experimental.Flags().Bool("wait", false, "")
	MarkFlag(experimental, "wait", Stable, "")
	if got := EffectiveFlag(experimental, "wait"); got != Experimental {
		t.Errorf("EffectiveFlag(ingest --wait) = %q, want experimental (capped)", got)
	}
}

func TestMarkStableIsANoOp(t *testing.T) {
	cmd := &cobra.Command{Use: "get"}
	Mark(cmd, Stable, "0.38.0")
	if len(cmd.Annotations) != 0 {
		t.Errorf("marking stable wrote annotations %v; stable is the default and "+
			"writing it would only make the manifest noisier", cmd.Annotations)
	}
}

func TestLintRequiresSinceAndFeatureKeys(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}

	noSince := &cobra.Command{Use: "ingest"}
	setAnnotation(noSince, AnnotationLevel, string(Experimental))
	root.AddCommand(noSince)

	noFeature := &cobra.Command{Use: "account"}
	setAnnotation(noFeature, AnnotationLevel, string(Development))
	root.AddCommand(noFeature)

	problems := Lint(root)
	if len(problems) != 2 {
		t.Fatalf("Lint found %d problems (%v), want 2", len(problems), problems)
	}
	joined := problems[0].Error() + "\n" + problems[1].Error()
	for _, want := range []string{"without a since-version", "without a development feature key"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Lint output %q missing %q", joined, want)
		}
	}
}

func TestLintIgnoresUnannotatedFlags(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}
	dev := &cobra.Command{Use: "account"}
	MarkDevelopment(dev, "account")
	dev.Flags().String("name", "", "")
	root.AddCommand(dev)

	// An unannotated flag has no opinion of its own; it inherits. Requiring
	// every flag on a development command to repeat the annotation would be
	// pure noise.
	if problems := Lint(root); len(problems) != 0 {
		t.Errorf("Lint flagged an unannotated flag: %v", problems)
	}
}

func TestLintCatchesAFlagStrongerThanItsCommand(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}
	exp := &cobra.Command{Use: "ingest"}
	Mark(exp, Experimental, "0.38.0")
	exp.Flags().Bool("wait", false, "")
	// Declared explicitly, which is what makes it an author error rather than
	// an inherited default.
	exp.Flags().Lookup("wait").Annotations = map[string][]string{
		AnnotationLevel: {string(Stable)},
	}
	root.AddCommand(exp)

	problems := Lint(root)
	if len(problems) != 1 {
		t.Fatalf("Lint found %d problems (%v), want 1", len(problems), problems)
	}
	if !strings.Contains(problems[0].Error(), "never stronger") {
		t.Errorf("unexpected lint message: %v", problems[0])
	}
}

func TestBadgeAndGuarantee(t *testing.T) {
	if Badge(Stable) != "" {
		t.Error("stable must carry no badge; it is the unremarkable case")
	}
	if Badge(Experimental) != "[Experimental]" {
		t.Errorf("Badge(experimental) = %q", Badge(Experimental))
	}
	// Every badge is paired with an explicit guarantee, because AIP-181 defines
	// "experimental" as the weakest level of all and a reader may otherwise
	// take the middle tier to promise less than we intend.
	for _, lvl := range ValidLevels() {
		if Guarantee(lvl) == "" {
			t.Errorf("no guarantee text for %q", lvl)
		}
	}
}

func TestDeprecationNote(t *testing.T) {
	d := Deprecation{Since: "0.38.0", RemoveIn: "1.0.0", Replacement: "get workflows"}
	got := d.Note()
	for _, want := range []string{"0.38.0", "1.0.0", "get workflows"} {
		if !strings.Contains(got, want) {
			t.Errorf("Note() = %q, missing %q", got, want)
		}
	}
}

func TestWeakestIsSymmetric(t *testing.T) {
	for _, a := range ValidLevels() {
		for _, b := range ValidLevels() {
			if session.Weakest(a, b) != session.Weakest(b, a) {
				t.Errorf("Weakest(%q,%q) != Weakest(%q,%q)", a, b, b, a)
			}
		}
	}
}
