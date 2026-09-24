package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDQLOptionsWithMetadataPropagateVerbose guards --metadata=minimal's -v
// contract: under minimal, an inline agent result drops the spill measurement
// fields unless DQLExecuteOptions.Verbose is set, so every command that forwards
// --metadata must also forward -v, or `-M=minimal -v` silently loses them.
func TestDQLOptionsWithMetadataPropagateVerbose(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "DQLExecuteOptions" {
				return true
			}
			keys := map[string]bool{}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if id, ok := kv.Key.(*ast.Ident); ok {
						keys[id.Name] = true
					}
				}
			}
			if keys["MetadataFields"] {
				checked++
				if !keys["Verbose"] {
					t.Errorf("%s: DQLExecuteOptions sets MetadataFields but not Verbose", fset.Position(lit.Pos()))
				}
			}
			return true
		})
	}
	if checked < 2 {
		t.Fatalf("found %d DQLExecuteOptions literals forwarding --metadata, want at least 2 (query, get snapshots): the guard is not seeing them", checked)
	}
}
