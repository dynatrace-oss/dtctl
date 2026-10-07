package session

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
)

func isolateDataDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv(EnvTokenStorage, "")
	t.Setenv(EnvDisableKeyring, "")
	xdg.Reload()
	t.Cleanup(xdg.Reload)
}

func TestIsKeyringAbsent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no secret service", errors.New("keyring probe failed: The name org.freedesktop.secrets was not provided by any .service files"), true},
		{"no session bus", errors.New("keyring probe failed: dbus: couldn't determine address of session bus"), true},
		{"locked collection", errors.New("keyring probe failed: " + ErrMsgCollectionUnlock), false},
		{"disabled", errors.New("keyring disabled via " + EnvDisableKeyring + " environment variable"), false},
		{"other failure", errors.New("keyring probe failed: boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKeyringAbsent(tt.err); got != tt.want {
				t.Errorf("IsKeyringAbsent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileTokenStorageConsent(t *testing.T) {
	isolateDataDir(t)

	if IsFileTokenStorage() {
		t.Fatal("file storage must be off before any choice is made")
	}
	if err := PersistFileTokenStorage(); err != nil {
		t.Fatal(err)
	}
	if !IsFileTokenStorage() {
		t.Error("persisted consent should enable file storage")
	}
	info, err := os.Stat(FileTokenStorageConsentPath())
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not model POSIX permission bits.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("consent marker mode = %o, want 600", perm)
	}

	t.Run("env keyring overrides consent", func(t *testing.T) {
		t.Setenv(EnvTokenStorage, "keyring")
		if IsFileTokenStorage() {
			t.Error("DTCTL_TOKEN_STORAGE=keyring must win over the persisted choice")
		}
	})
	t.Run("disabled keyring ignores consent", func(t *testing.T) {
		t.Setenv(EnvDisableKeyring, "1")
		if IsFileTokenStorage() {
			t.Error("embedded sessions (keyring disabled) must not pick up host consent")
		}
	})
	t.Run("env file works without consent", func(t *testing.T) {
		_ = os.Remove(FileTokenStorageConsentPath())
		t.Setenv(EnvTokenStorage, "file")
		if !IsFileTokenStorage() {
			t.Error("DTCTL_TOKEN_STORAGE=file must still enable file storage")
		}
	})
}
