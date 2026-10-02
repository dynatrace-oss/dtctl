package session

import (
	"fmt"
	"maps"
	"strings"
	"testing"
)

// denyKeyringWrites makes every keyring write fail the way macOS does for an
// unsigned CGO_ENABLED=0 binary (`security` exit status 44), while reads keep
// working: the read-only CheckKeyring probe still reports the keyring as
// reachable. This is the setup behind #393.
func denyKeyringWrites(tm *TokenManager) {
	tm.deps.setToken = func(_ *TokenStore, _, _ string) error {
		return fmt.Errorf("failed to store token in keyring: exit status 44")
	}
}

// newProcessTM returns a TokenManager that sees only what an earlier process
// persisted (the keyring and file-store contents), and none of its in-memory
// state. It models a later `dtctl auth status` or `dtctl doctor` run.
func newProcessTM(t *testing.T, keyring, files map[string]string) *TokenManager {
	t.Helper()
	tm, k, f := newTMWithSizedKeyring(t, -1)
	maps.Copy(k, keyring)
	maps.Copy(f, files)
	return tm
}

func TestGetTokenInfoWithStorage_KeyringWriteDeniedReportsFile(t *testing.T) {
	t.Parallel()
	stored := sampleStoredToken()

	login, keyring, files := newTMWithSizedKeyring(t, -1)
	denyKeyringWrites(login)
	if err := login.saveToken("my-token", stored); err != nil {
		t.Fatalf("saveToken() error = %v, want nil (file fallback)", err)
	}

	// The keyring is still "available" to the probe; the token is not in it.
	status := newProcessTM(t, keyring, files)
	if !status.deps.keyringAvailable() {
		t.Fatal("test setup: keyring probe must report the keyring as reachable")
	}

	got, storage, err := status.GetTokenInfoWithStorage("my-token")
	if err != nil {
		t.Fatalf("GetTokenInfoWithStorage() error = %v", err)
	}
	if storage != TokenStorageFile {
		t.Errorf("storage = %q, want %q: the keyring refused the write, so the token is in the file store", storage, TokenStorageFile)
	}
	if got.RefreshToken != stored.RefreshToken {
		t.Errorf("refresh token = %q, want %q", got.RefreshToken, stored.RefreshToken)
	}
}

func TestGetTokenInfoWithStorage_KeyringWriteSucceedsReportsKeyring(t *testing.T) {
	t.Parallel()

	login, keyring, files := newTMWithSizedKeyring(t, -1)
	if err := login.saveToken("my-token", sampleStoredToken()); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}

	_, storage, err := newProcessTM(t, keyring, files).GetTokenInfoWithStorage("my-token")
	if err != nil {
		t.Fatalf("GetTokenInfoWithStorage() error = %v", err)
	}
	if storage != TokenStorageKeyring {
		t.Errorf("storage = %q, want %q", storage, TokenStorageKeyring)
	}
}

// A token too large for the keyring in every encoding also lands in the file
// store; the report must follow it there too, not only for write denial.
func TestGetTokenInfoWithStorage_SizeLimitFallbackReportsFile(t *testing.T) {
	t.Parallel()

	login, keyring, files := newTMWithSizedKeyring(t, 1) // nothing fits
	if err := login.saveToken("my-token", sampleStoredToken()); err != nil {
		t.Fatalf("saveToken() error = %v, want nil (file fallback)", err)
	}

	_, storage, err := newProcessTM(t, keyring, files).GetTokenInfoWithStorage("my-token")
	if err != nil {
		t.Fatalf("GetTokenInfoWithStorage() error = %v", err)
	}
	if storage != TokenStorageFile {
		t.Errorf("storage = %q, want %q", storage, TokenStorageFile)
	}
}

func TestGetTokenInfoWithStorage_ExplicitFileStorage(t *testing.T) {
	t.Parallel()

	tm, keyring, _ := newTMWithSizedKeyring(t, -1)
	tm.deps.fileStoreAvailable = func() bool { return true }
	if err := tm.saveToken("my-token", sampleStoredToken()); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}
	if len(keyring) != 0 {
		t.Fatalf("explicit file storage must not write the keyring, got %d entries", len(keyring))
	}

	_, storage, err := tm.GetTokenInfoWithStorage("my-token")
	if err != nil {
		t.Fatalf("GetTokenInfoWithStorage() error = %v", err)
	}
	if storage != TokenStorageFile {
		t.Errorf("storage = %q, want %q", storage, TokenStorageFile)
	}
}

func TestGetTokenInfoWithStorage_MissingToken(t *testing.T) {
	t.Parallel()

	tm, _, _ := newTMWithSizedKeyring(t, -1)
	stored, storage, err := tm.GetTokenInfoWithStorage("absent")
	if err == nil {
		t.Fatalf("GetTokenInfoWithStorage() = %+v, want an error for a token that was never saved", stored)
	}
	if storage != "" {
		t.Errorf("storage = %q, want empty when no token was found", storage)
	}
}

func TestTokenStorageLabel(t *testing.T) {
	t.Parallel()

	if got := TokenStorageKeyring.Label(); got != KeyringBackend() {
		t.Errorf("TokenStorageKeyring.Label() = %q, want %q", got, KeyringBackend())
	}
	got := TokenStorageFile.Label()
	if !strings.HasPrefix(got, "file (") || !strings.Contains(got, oauthTokensDir()) {
		t.Errorf("TokenStorageFile.Label() = %q, want file (%s)", got, oauthTokensDir())
	}
}

// TestOAuthStorageBackend_ReflectsWhereTheSaveLanded covers the in-process
// half of #393: right after a login whose keyring write was refused,
// OAuthStorageBackend must name the file store even though the read-only
// keyring probe still succeeds.
//
// Not parallel: it reads and resets the process-wide save record, and the
// testing package runs every t.Parallel test only after the sequential ones.
func TestOAuthStorageBackend_ReflectsWhereTheSaveLanded(t *testing.T) {
	t.Setenv(EnvTokenStorage, "")
	prev := lastSavedStorage()
	recordSavedStorage("")
	t.Cleanup(func() { recordSavedStorage(prev) })

	keyringReachable := func() bool { return true }

	if got := oauthStorageBackend(keyringReachable); got != KeyringBackend() {
		t.Fatalf("before any save: OAuthStorageBackend() = %q, want %q", got, KeyringBackend())
	}

	denied, _, _ := newTMWithSizedKeyring(t, -1)
	denyKeyringWrites(denied)
	if err := denied.saveToken("my-token", sampleStoredToken()); err != nil {
		t.Fatalf("saveToken() error = %v, want nil (file fallback)", err)
	}
	if got := oauthStorageBackend(keyringReachable); got != fileStorageLabel() {
		t.Errorf("after a refused keyring write: OAuthStorageBackend() = %q, want %q", got, fileStorageLabel())
	}

	ok, _, _ := newTMWithSizedKeyring(t, -1)
	if err := ok.saveToken("my-token", sampleStoredToken()); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}
	if got := oauthStorageBackend(keyringReachable); got != KeyringBackend() {
		t.Errorf("after a keyring save: OAuthStorageBackend() = %q, want %q", got, KeyringBackend())
	}
}
