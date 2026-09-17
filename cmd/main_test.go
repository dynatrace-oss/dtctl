package cmd

import (
	"os"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// TestMain keeps the cmd test binary away from the real OS keyring.
//
// Many tests here build a Config and call SetToken with placeholder references
// such as "test-token" or "my-token". Config.SetToken writes through to the OS
// keyring whenever one is reachable, so without this guard a plain
// `go test ./...` stores dummy credentials in the developer's macOS Keychain,
// Secret Service, or Windows Credential Manager — and on macOS can block the
// run behind a GUI keychain prompt.
//
// Individual tests may still opt back in with t.Setenv, which restores this
// default when they finish.
func TestMain(m *testing.M) {
	if err := os.Setenv(config.EnvDisableKeyring, "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
