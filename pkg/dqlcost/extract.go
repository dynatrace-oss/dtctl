package dqlcost

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// TileQuery is a single DQL query extracted from a dashboard or notebook
// document together with a human-readable locator (e.g. "tiles.1.query").
type TileQuery struct {
	Path  string // dot/bracket path inside the document
	Query string // the DQL query text
}

// ExtractTileQueries walks a YAML or JSON dashboard/notebook document and
// returns every DQL query string it finds. Dashboards nest queries under
// `content.tiles.<key>.query`; notebooks under `cells[*].query`. The walker
// is schema-agnostic: any string-valued field named "query" at a reasonable
// depth is returned — callers can discard spurious entries.
func ExtractTileQueries(data []byte) ([]TileQuery, error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		// Fall back to JSON.
		if jErr := json.Unmarshal(data, &root); jErr != nil {
			return nil, fmt.Errorf("parse as yaml or json: %w", err)
		}
	}
	var out []TileQuery
	walk(root, "", &out)
	return out, nil
}

func walk(v any, path string, out *[]TileQuery) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			p := joinPath(path, k)
			if k == "query" {
				if s, ok := child.(string); ok && s != "" {
					*out = append(*out, TileQuery{Path: p, Query: s})
					continue
				}
			}
			walk(child, p, out)
		}
	case map[any]any:
		// yaml.v3 with "any" key type occasionally produces this shape.
		for k, child := range x {
			ks, _ := k.(string)
			p := joinPath(path, ks)
			if ks == "query" {
				if s, ok := child.(string); ok && s != "" {
					*out = append(*out, TileQuery{Path: p, Query: s})
					continue
				}
			}
			walk(child, p, out)
		}
	case []any:
		for i, child := range x {
			walk(child, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// DocumentReport is the aggregated lint result for a full document.
type DocumentReport struct {
	Tiles []TileReport
}

// TileReport is the per-query lint result with its source path.
type TileReport struct {
	Path     string
	Query    string
	Findings []Finding
}

// HasWarnOrHigher reports whether any tile has a warn-or-higher finding.
func (r DocumentReport) HasWarnOrHigher() bool {
	for _, t := range r.Tiles {
		if MaxSeverity(t.Findings) >= SeverityWarn {
			return true
		}
	}
	return false
}

// LintDocument extracts every tile query and lints each. Tiles with no
// findings are omitted from the report so callers only see signal.
func LintDocument(data []byte) (DocumentReport, error) {
	tiles, err := ExtractTileQueries(data)
	if err != nil {
		return DocumentReport{}, err
	}
	var rep DocumentReport
	for _, t := range tiles {
		findings := Lint(t.Query)
		if len(findings) == 0 {
			continue
		}
		rep.Tiles = append(rep.Tiles, TileReport{
			Path:     t.Path,
			Query:    t.Query,
			Findings: findings,
		})
	}
	return rep, nil
}
