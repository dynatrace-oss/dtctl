package pii

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// PseudonymStore manages session-scoped pseudonym mappings.
// It maps (category, original_value) → stable pseudonym within a session.
// Sessions are persisted to disk as JSON files so a separate resolve tool
// can map pseudonyms back to originals.
type PseudonymStore struct {
	mu        sync.Mutex
	sessionID string
	context   string
	createdAt time.Time

	// forward maps "CATEGORY:original" → "<CATEGORY_N>"
	forward map[string]string

	// reverse maps "<CATEGORY_N>" → Mapping
	reverse map[string]Mapping

	// counters tracks the next index per category
	counters map[string]int
}

// Mapping represents a single pseudonym mapping.
type Mapping struct {
	Category  string `json:"category"`
	Original  string `json:"original"`
	Pseudonym string `json:"pseudonym"`
}

// SessionFile represents the on-disk format of a pseudonym session.
type SessionFile struct {
	SessionID string             `json:"session_id"`
	Context   string             `json:"context"`
	CreatedAt string             `json:"created_at"`
	Mappings  map[string][]Entry `json:"mappings"` // category → entries
}

// Entry is a single mapping entry in the session file.
type Entry struct {
	Original  string `json:"original"`
	Pseudonym string `json:"pseudonym"`
}

// NewPseudonymStore creates a new store for the given context.
func NewPseudonymStore(context string) (*PseudonymStore, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	return &PseudonymStore{
		sessionID: id,
		context:   context,
		createdAt: time.Now().UTC(),
		forward:   make(map[string]string),
		reverse:   make(map[string]Mapping),
		counters:  make(map[string]int),
	}, nil
}

// SessionID returns the session identifier.
func (s *PseudonymStore) SessionID() string {
	return s.sessionID
}

// GetOrCreate returns the pseudonym for the given (category, original) pair.
// If a pseudonym already exists for this pair, it is returned (stable).
// Otherwise, a new pseudonym is created with the next available index.
func (s *PseudonymStore) GetOrCreate(category, original string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := category + ":" + original
	if pseudonym, ok := s.forward[key]; ok {
		return pseudonym
	}

	// Create new pseudonym: <CATEGORY_N>
	idx := s.counters[category]
	s.counters[category] = idx + 1
	pseudonym := fmt.Sprintf("<%s_%d>", category, idx)

	s.forward[key] = pseudonym
	s.reverse[pseudonym] = Mapping{
		Category:  category,
		Original:  original,
		Pseudonym: pseudonym,
	}

	return pseudonym
}

// Resolve returns the original value for a pseudonym.
// Returns empty Mapping and false if not found.
func (s *PseudonymStore) Resolve(pseudonym string) (Mapping, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.reverse[pseudonym]
	return m, ok
}

// AllMappings returns all mappings in the store.
func (s *PseudonymStore) AllMappings() []Mapping {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Mapping, 0, len(s.reverse))
	for _, m := range s.reverse {
		result = append(result, m)
	}
	return result
}

// Save persists the session to disk as a JSON file.
// Files are stored at: XDG_DATA_HOME/dtctl/pii/sessions/<session-id>.json
func (s *PseudonymStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.forward) == 0 {
		return nil // Nothing to save
	}

	dir := SessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create PII session directory: %w", err)
	}

	// Build session file
	sf := SessionFile{
		SessionID: s.sessionID,
		Context:   s.context,
		CreatedAt: s.createdAt.Format(time.RFC3339),
		Mappings:  make(map[string][]Entry),
	}

	for _, m := range s.reverse {
		sf.Mappings[m.Category] = append(sf.Mappings[m.Category], Entry{
			Original:  m.Original,
			Pseudonym: m.Pseudonym,
		})
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	path := filepath.Join(dir, s.sessionID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// SessionDir returns the directory where PII sessions are stored.
// It is a variable so tests can override it.
var SessionDir = func() string {
	return filepath.Join(config.DataDir(), "pii", "sessions")
}

// LoadSession loads a session file from disk.
func LoadSession(sessionID string) (*SessionFile, error) {
	path := filepath.Join(SessionDir(), sessionID+".json")
	return LoadSessionFrom(path)
}

// LoadSessionFrom loads a session file from a specific path.
func LoadSessionFrom(path string) (*SessionFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var sf SessionFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &sf, nil
}

// ListSessions returns all session files in the session directory.
func ListSessions() ([]SessionFile, error) {
	dir := SessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	var sessions []SessionFile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		sf, err := LoadSessionFrom(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // Skip malformed files
		}
		sessions = append(sessions, *sf)
	}

	return sessions, nil
}

// PurgeSessions deletes sessions older than the given duration.
// Returns the number of sessions deleted.
func PurgeSessions(maxAge time.Duration) (int, error) {
	dir := SessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read session directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, e.Name())
		sf, err := LoadSessionFrom(path)
		if err != nil {
			continue
		}

		created, err := time.Parse(time.RFC3339, sf.CreatedAt)
		if err != nil {
			continue
		}

		if created.Before(cutoff) {
			if err := os.Remove(path); err == nil {
				deleted++
			}
		}
	}

	return deleted, nil
}

// generateSessionID creates a unique session identifier.
// Format: pii_YYYYMMDD_HHMMSS_<random>
func generateSessionID() (string, error) {
	now := time.Now().UTC()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("pii_%s_%s",
		now.Format("20060102_150405"),
		hex.EncodeToString(b),
	), nil
}

// ResolveInSession resolves a pseudonym from a loaded session file.
func (sf *SessionFile) ResolveInSession(pseudonym string) (Entry, string, bool) {
	for category, entries := range sf.Mappings {
		for _, e := range entries {
			if e.Pseudonym == pseudonym {
				return e, category, true
			}
		}
	}
	return Entry{}, "", false
}

// TotalMappings returns the total number of mappings in the session.
func (sf *SessionFile) TotalMappings() int {
	total := 0
	for _, entries := range sf.Mappings {
		total += len(entries)
	}
	return total
}
