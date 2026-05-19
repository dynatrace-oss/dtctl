// Package agentmode provides AI agent detection and structured JSON output.
//
// It detects whether the CLI is running under an AI coding agent (e.g.
// Claude Code, Cursor, GitHub Copilot) by checking environment variables,
// and provides the standard Dynatrace CLI agent envelope format:
//
//	{"ok": true, "result": ..., "context": {...}}
package agentmode
