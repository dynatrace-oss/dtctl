package session

import (
	"os"
	"testing"
)

// TestMain keeps the session test binary away from the real OS keyring.
//
// Tests such as TestGetTokenWithFallback and TestMigrateTokensToKeyring_NoKeyring
// call Config.SetToken with placeholder references, which writes through to the
// OS keyring whenever one is reachable. Without this guard a plain `go test ./...`
// leaves dummy credentials in the developer's keyring and, on macOS, can block
// the run behind a GUI keychain prompt.
//
// Individual tests may still opt back in with t.Setenv, which restores this
// default when they finish.
func TestMain(m *testing.M) {
	if err := os.Setenv(EnvDisableKeyring, "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
