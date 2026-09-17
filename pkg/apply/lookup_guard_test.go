package apply

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every applier resolves create vs update from an existence lookup, and the
// duplicate-creating shape is always the same: the lookup error is discarded,
// so "the lookup failed" reads as "the object is absent" and apply creates a
// second copy (issue #491). It was written five times before it was noticed.
//
// The two shapes below are the ones that actually occurred. A lookup error must
// either be inspected (only a 404 or a package's not-found sentinel means
// create) or returned.
func TestLookupErrorsAreNeverDiscarded(t *testing.T) {
	files, err := filepath.Glob("apply_*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		ast.Inspect(parsed, func(n ast.Node) bool {
			cond, ok := n.(*ast.IfStmt)
			if !ok || cond.Cond == nil {
				return true
			}
			text := string(src[fset.Position(cond.Cond.Pos()).Offset:fset.Position(cond.Cond.End()).Offset])
			// `if err == nil && existing != nil` — the error is never read.
			// The binding is always named `existing`; a best-effort read that
			// feeds no create decision (the ShowDiff fetch) is not this shape.
			if strings.Contains(text, "err == nil &&") && strings.Contains(text, "existing !=") {
				t.Errorf("%s: %s\n\t`err == nil && ...` discards the lookup error: a 403 or 5xx then reads as "+
					"\"absent\" and apply creates a duplicate (#491). Inspect the error instead.",
					fset.Position(cond.Cond.Pos()), text)
			}
			return true
		})

		// `stderrWarn(...)` inside an `if err != nil` that falls through to a
		// create — warn-and-create, the anomaly detector's original bug.
		for _, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "stderrWarn") &&
				(strings.Contains(line, "Failed to lookup") || strings.Contains(line, "could not check for an existing")) {
				t.Errorf("%s: %s\n\twarning on a failed lookup and continuing creates a duplicate (#491). Return the error.",
					file, strings.TrimSpace(line))
			}
		}
	}
}
