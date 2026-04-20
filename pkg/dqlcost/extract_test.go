package dqlcost

import "testing"

func TestExtractTileQueries_Dashboard(t *testing.T) {
	doc := []byte(`
name: "Test"
type: dashboard
content:
  tiles:
    "1":
      title: "Errors"
      type: data
      query: |
        fetch logs | limit 10
    "2":
      title: "Markdown"
      type: markdown
      content: "## header"
`)
	tiles, err := ExtractTileQueries(doc)
	if err != nil {
		t.Fatalf("ExtractTileQueries: %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("expected 1 tile query, got %d: %+v", len(tiles), tiles)
	}
	if tiles[0].Path == "" || tiles[0].Query == "" {
		t.Fatalf("missing path or query: %+v", tiles[0])
	}
}

func TestLintDocument_FlagsBadTiles(t *testing.T) {
	doc := []byte(`
name: "Test"
type: dashboard
content:
  tiles:
    bad:
      type: data
      query: |
        fetch logs | filter loglevel == "ERROR"
    good:
      type: data
      query: |
        fetch logs, from:now()-1h, scanLimitGBytes:50 | filter loglevel == "ERROR"
`)
	rep, err := LintDocument(doc)
	if err != nil {
		t.Fatalf("LintDocument: %v", err)
	}
	// Bad tile should report COST001/COST008. Good tile should be silent enough
	// not to hit those.
	found001 := false
	for _, tile := range rep.Tiles {
		for _, f := range tile.Findings {
			if f.Rule == "COST001" {
				found001 = true
			}
		}
	}
	if !found001 {
		t.Fatalf("expected COST001 on bad tile, got report=%+v", rep)
	}
	if !rep.HasWarnOrHigher() {
		t.Fatalf("HasWarnOrHigher should be true")
	}
}
