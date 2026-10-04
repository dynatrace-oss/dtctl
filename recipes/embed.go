// Package recipes holds dtctl's built-in recipe content: one YAML file per
// recipe under <domain>/, the domain registry (_domains.yaml), the scope
// dimensions (_scopes.yaml) and shared template fragments (_fragments/).
//
// This is content, not code. pkg/recipes loads and validates it; see
// docs/dev/RECIPES_DESIGN.md and recipes/README.md for authoring rules.
package recipes

import (
	"embed"
	"io/fs"
)

//go:embed _domains.yaml _scopes.yaml _fragments */*.yaml
var content embed.FS

// FS returns the built-in recipe tree.
func FS() fs.FS { return content }
