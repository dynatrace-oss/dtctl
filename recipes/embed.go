// Package recipes holds dtctl's built-in recipe content: YAML under <domain>/ plus
// domains, scopes and fragments. pkg/recipes loads it; see recipes/README.md.
package recipes

import (
	"embed"
	"io/fs"
)

//go:embed _domains.yaml _scopes.yaml _fragments */*.yaml
var content embed.FS

// FS returns the built-in recipe tree.
func FS() fs.FS { return content }
