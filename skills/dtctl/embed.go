// Package dtctlskill embeds the full dtctl skill content (SKILL.md and all
// reference files) so that other packages can access it without filesystem
// reads or build-time code generation.
package dtctlskill

import "embed"

// Content is the embedded filesystem rooted at skills/dtctl/.
// It contains SKILL.md and the references/ subtree.
//
//go:embed SKILL.md references
var Content embed.FS
