// Package pii provides PII (Personally Identifiable Information) detection
// and redaction for query results. It supports two modes:
//
//   - Lite mode: replaces detected PII with category placeholders like [EMAIL],
//     [PERSON], [IP_ADDRESS]. No state, no external dependencies.
//
//   - Full mode: replaces PII with stable, session-scoped pseudonyms like
//     <EMAIL_0>, <PERSON_1> that allow correlation across records within a
//     query. Optionally integrates with Microsoft Presidio for NER-based
//     detection of PII in free-text fields. Sessions are persisted to disk
//     so a separate resolve tool can map pseudonyms back to originals.
//
// The package is designed to be modular: it has no coupling to the rest of
// dtctl and can be used independently.
package pii

import (
	"fmt"
	"os"
	"strings"
)

// Mode controls the PII redaction behavior.
type Mode string

const (
	// ModeOff disables PII redaction (default).
	ModeOff Mode = ""
	// ModeLite replaces PII with category placeholders: [EMAIL], [PERSON], etc.
	ModeLite Mode = "lite"
	// ModeFull replaces PII with stable pseudonyms: <EMAIL_0>, <PERSON_1>, etc.
	// Sessions are persisted to disk for later resolution.
	ModeFull Mode = "full"
)

// ParseMode parses a mode string, returning ModeOff for empty/unknown values.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lite":
		return ModeLite
	case "full":
		return ModeFull
	default:
		return ModeOff
	}
}

// Config holds PII redaction configuration.
type Config struct {
	Mode        Mode         // Redaction mode: lite or full
	PresidioURL string       // Optional Presidio API URL for NER (full mode only)
	Context     string       // Dynatrace context name (used in session metadata)
	CustomRules []CustomRule // User-defined field rules (.dtctl-pii.yaml + config)
}

// Redactor orchestrates PII detection and replacement across query records.
// It is the main entry point for the pii package.
type Redactor struct {
	mode        Mode
	patterns    []Pattern
	customRules []CustomRule    // user-defined field rules (checked before built-in patterns)
	store       *PseudonymStore // nil in lite mode
	presidio    *PresidioClient // nil if Presidio is not configured
	stats       RedactionStats
}

// RedactionStats tracks what was redacted in a session.
type RedactionStats struct {
	TotalRecords   int            // Number of records processed
	RedactedFields int            // Number of fields redacted
	ByCategory     map[string]int // Redaction count per PII category
}

// NewRedactor creates a Redactor with the given configuration.
// In full mode, it initializes a PseudonymStore and optionally a Presidio client.
func NewRedactor(cfg Config) (*Redactor, error) {
	if cfg.Mode == ModeOff {
		return nil, fmt.Errorf("cannot create redactor with mode off")
	}

	r := &Redactor{
		mode:        cfg.Mode,
		patterns:    DefaultPatterns(),
		customRules: cfg.CustomRules,
		stats: RedactionStats{
			ByCategory: make(map[string]int),
		},
	}

	if cfg.Mode == ModeFull {
		store, err := NewPseudonymStore(cfg.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to create pseudonym store: %w", err)
		}
		r.store = store

		// Initialize Presidio client if URL is configured
		if cfg.PresidioURL != "" {
			r.presidio = NewPresidioClient(cfg.PresidioURL)
		}
	}

	return r, nil
}

// RedactRecords applies PII redaction to a slice of query result records.
// Each record is a map[string]interface{} as returned by DQL queries.
// Returns the redacted records (modified in place).
func (r *Redactor) RedactRecords(records []map[string]interface{}) []map[string]interface{} {
	if r == nil || len(records) == 0 {
		return records
	}

	// Phase 1+2: Walk records, detect PII by field name and value patterns
	for i := range records {
		r.stats.TotalRecords++
		r.walkRecord(records[i])
	}

	// Phase 3 (full mode only): Presidio NER on collected free-text fields
	if r.mode == ModeFull && r.presidio != nil {
		r.applyPresidioNER(records)
	}

	return records
}

// Stats returns the redaction statistics for this session.
func (r *Redactor) Stats() RedactionStats {
	if r == nil {
		return RedactionStats{ByCategory: make(map[string]int)}
	}
	return r.stats
}

// SessionID returns the session ID (full mode only, empty in lite mode).
func (r *Redactor) SessionID() string {
	if r.store != nil {
		return r.store.SessionID()
	}
	return ""
}

// Close persists the pseudonym store to disk (full mode only).
// Must be called after RedactRecords to save the session.
func (r *Redactor) Close() error {
	if r.store != nil {
		return r.store.Save()
	}
	return nil
}

// replace returns the redacted replacement for a PII value.
// In lite mode, returns a category placeholder like [EMAIL].
// In full mode, returns a stable pseudonym like <EMAIL_0>.
func (r *Redactor) replace(category, original string) string {
	r.stats.RedactedFields++
	r.stats.ByCategory[category]++

	if r.mode == ModeFull && r.store != nil {
		return r.store.GetOrCreate(category, original)
	}

	return "[" + category + "]"
}

// ResolveMode determines the PII mode from flag, env var, and config preference.
// Precedence: flag > env > config.
//
//   - flagValue: the value from --pii flag ("" if not set, "lite" for bare --pii, "full" for --pii=full)
//   - flagChanged: whether --pii was explicitly set on the command line
//   - noPII: whether --no-pii was set (overrides everything)
//   - configMode: the mode from config preferences
func ResolveMode(flagValue string, flagChanged bool, noPII bool, configMode string) Mode {
	// --no-pii overrides everything
	if noPII {
		return ModeOff
	}

	// Explicit --pii flag takes precedence
	if flagChanged {
		return ParseMode(flagValue)
	}

	// Environment variable
	if envVal := os.Getenv("DTCTL_PII"); envVal != "" {
		return ParseMode(envVal)
	}

	// Config preference
	return ParseMode(configMode)
}
