//go:build windows

package session

import (
	"encoding/json"
	"fmt"
	"syscall"
	"testing"
)

// TestIsKeyringFallbackErr_Windows verifies that ERROR_NO_SUCH_LOGON_SESSION
// (errno 1312) triggers the file-storage fallback on Windows. The detection
// uses errors.Is rather than string matching so it is locale-independent.
func TestIsKeyringFallbackErr_Windows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Windows admin logon session — direct errno",
			err:  syscall.Errno(1312),
			want: true,
		},
		{
			name: "Windows admin logon session — wrapped errno",
			err:  fmt.Errorf("failed to store token in keyring: %w", syscall.Errno(1312)),
			want: true,
		},
		{
			name: "other Windows errno — not a fallback trigger",
			err:  syscall.Errno(5), // ERROR_ACCESS_DENIED
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isKeyringFallbackErr(tc.err); got != tc.want {
				t.Errorf("isKeyringFallbackErr() = %v, want %v (err: %v)", got, tc.want, tc.err)
			}
		})
	}
}

// TestSaveToken_FallsBackToFileOnWindowsAdminError covers the Windows
// elevated-session case: keyring.Set returns ERROR_NO_SUCH_LOGON_SESSION (1312).
// All keyring encodings fail, so saveToken must fall back to file storage.
func TestSaveToken_FallsBackToFileOnWindowsAdminError(t *testing.T) {
	t.Parallel()
	stored := sampleStoredToken()

	tm, keyring, files := newTMWithSizedKeyring(t, -1)
	tm.deps.setToken = func(_ *TokenStore, _, _ string) error {
		return fmt.Errorf("failed to store token in keyring: %w", syscall.Errno(1312))
	}

	if err := tm.saveToken("my-token", stored); err != nil {
		t.Fatalf("saveToken() error = %v, want nil (file fallback)", err)
	}

	key := tm.getKeyringName("my-token")
	if _, ok := keyring[key]; ok {
		t.Errorf("keyring entry should be absent when write fails with Windows admin error")
	}
	raw, ok := files[key]
	if !ok {
		t.Fatalf("expected full token in file store, none found")
	}
	var got StoredToken
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.AccessToken != stored.AccessToken {
		t.Errorf("file store access token = %q, want full access token", got.AccessToken)
	}
}
