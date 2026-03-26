package pii

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPseudonymStoreGetOrCreate(t *testing.T) {
	store, err := NewPseudonymStore("test")
	require.NoError(t, err)

	// First call creates a new pseudonym
	p1 := store.GetOrCreate(CategoryEmail, "alice@example.com")
	assert.Equal(t, "<EMAIL_0>", p1)

	// Same value returns the same pseudonym (stable)
	p2 := store.GetOrCreate(CategoryEmail, "alice@example.com")
	assert.Equal(t, "<EMAIL_0>", p2)

	// Different value gets a new index
	p3 := store.GetOrCreate(CategoryEmail, "bob@example.com")
	assert.Equal(t, "<EMAIL_1>", p3)

	// Different category starts at 0
	p4 := store.GetOrCreate(CategoryPerson, "Alice Smith")
	assert.Equal(t, "<PERSON_0>", p4)
}

func TestPseudonymStoreResolve(t *testing.T) {
	store, err := NewPseudonymStore("test")
	require.NoError(t, err)

	store.GetOrCreate(CategoryEmail, "alice@example.com")
	store.GetOrCreate(CategoryPerson, "Alice Smith")

	// Resolve known pseudonyms
	m, ok := store.Resolve("<EMAIL_0>")
	assert.True(t, ok)
	assert.Equal(t, CategoryEmail, m.Category)
	assert.Equal(t, "alice@example.com", m.Original)

	m, ok = store.Resolve("<PERSON_0>")
	assert.True(t, ok)
	assert.Equal(t, "Alice Smith", m.Original)

	// Resolve unknown pseudonym
	_, ok = store.Resolve("<EMAIL_99>")
	assert.False(t, ok)
}

func TestPseudonymStoreAllMappings(t *testing.T) {
	store, err := NewPseudonymStore("test")
	require.NoError(t, err)

	store.GetOrCreate(CategoryEmail, "a@example.com")
	store.GetOrCreate(CategoryEmail, "b@example.com")
	store.GetOrCreate(CategoryPerson, "Alice")

	mappings := store.AllMappings()
	assert.Len(t, mappings, 3)
}

func TestPseudonymStoreSaveAndLoad(t *testing.T) {
	// Use a temp directory for session files
	tmpDir := t.TempDir()
	origSessionDir := SessionDir
	SessionDir = func() string { return tmpDir }
	defer func() { SessionDir = origSessionDir }()

	store, err := NewPseudonymStore("test-ctx")
	require.NoError(t, err)

	store.GetOrCreate(CategoryEmail, "alice@example.com")
	store.GetOrCreate(CategoryPerson, "Alice Smith")

	err = store.Save()
	require.NoError(t, err)

	// Verify file exists
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Load the session
	sf, err := LoadSessionFrom(filepath.Join(tmpDir, files[0].Name()))
	require.NoError(t, err)

	assert.Equal(t, store.SessionID(), sf.SessionID)
	assert.Equal(t, "test-ctx", sf.Context)
	assert.NotEmpty(t, sf.CreatedAt)
	assert.Equal(t, 2, sf.TotalMappings())

	// Resolve from loaded session
	entry, cat, ok := sf.ResolveInSession("<EMAIL_0>")
	assert.True(t, ok)
	assert.Equal(t, CategoryEmail, cat)
	assert.Equal(t, "alice@example.com", entry.Original)

	// Unknown pseudonym
	_, _, ok = sf.ResolveInSession("<PHONE_99>")
	assert.False(t, ok)
}

func TestPseudonymStoreSaveEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	origSessionDir := SessionDir
	SessionDir = func() string { return tmpDir }
	defer func() { SessionDir = origSessionDir }()

	store, err := NewPseudonymStore("test")
	require.NoError(t, err)

	// Save with no mappings should be a no-op
	err = store.Save()
	require.NoError(t, err)

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	origSessionDir := SessionDir
	SessionDir = func() string { return tmpDir }
	defer func() { SessionDir = origSessionDir }()

	// Create two sessions
	s1, _ := NewPseudonymStore("ctx1")
	s1.GetOrCreate(CategoryEmail, "a@test.com")
	require.NoError(t, s1.Save())

	s2, _ := NewPseudonymStore("ctx2")
	s2.GetOrCreate(CategoryPerson, "Bob")
	require.NoError(t, s2.Save())

	sessions, err := ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestListSessionsEmptyDir(t *testing.T) {
	origSessionDir := SessionDir
	SessionDir = func() string { return "/nonexistent/path/that/does/not/exist" }
	defer func() { SessionDir = origSessionDir }()

	sessions, err := ListSessions()
	require.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestPurgeSessions(t *testing.T) {
	tmpDir := t.TempDir()
	origSessionDir := SessionDir
	SessionDir = func() string { return tmpDir }
	defer func() { SessionDir = origSessionDir }()

	// Create a session
	s, _ := NewPseudonymStore("test")
	s.GetOrCreate(CategoryEmail, "test@test.com")
	require.NoError(t, s.Save())

	// Purge with 0 duration should delete everything (all sessions are older than "now")
	deleted, err := PurgeSessions(0)
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Verify empty
	files, _ := os.ReadDir(tmpDir)
	assert.Empty(t, files)
}

func TestPurgeSessionsKeepsRecent(t *testing.T) {
	tmpDir := t.TempDir()
	origSessionDir := SessionDir
	SessionDir = func() string { return tmpDir }
	defer func() { SessionDir = origSessionDir }()

	// Create a session
	s, _ := NewPseudonymStore("test")
	s.GetOrCreate(CategoryEmail, "test@test.com")
	require.NoError(t, s.Save())

	// Purge with large max age — session is recent, should be kept
	deleted, err := PurgeSessions(24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)

	files, _ := os.ReadDir(tmpDir)
	assert.Len(t, files, 1)
}

func TestSessionIDFormat(t *testing.T) {
	id, err := generateSessionID()
	require.NoError(t, err)
	assert.Regexp(t, `^pii_\d{8}_\d{6}_[0-9a-f]{8}$`, id)
}
