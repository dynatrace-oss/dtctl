//go:build linux

package config

import (
	"context"
	"fmt"
	"time"

	dbus "github.com/godbus/dbus/v5"
	ss "github.com/zalando/go-keyring/secret_service"
)

// EnsureKeyringCollection checks whether a usable Secret Service collection
// exists and, if not, creates a persistent "login" collection.
// On Linux/WSL gnome-keyring may start with only a transient "session"
// collection; this function creates the permanent one, which may trigger
// an OS password prompt.
//
// The provided context allows the caller to cancel the operation (e.g. via
// Ctrl+C); without cancellation the function polls for up to 2 minutes
// waiting for the user to complete the password prompt.
func EnsureKeyringCollection(ctx context.Context) error {
	svc, err := ss.NewSecretService()
	if err != nil {
		return fmt.Errorf("cannot connect to Secret Service: %w", err)
	}
	// NOTE: Do NOT call svc.Conn.Close(). NewSecretService() uses
	// dbus.SessionBus() which returns a shared, process-wide connection.
	// The dbus docs explicitly warn: "This method must not be called on
	// shared connections." Closing it could break go-keyring (called via
	// CheckKeyring right after this function). The connection is cleaned
	// up automatically when the process exits.

	// If the "login" collection already exists, nothing to do.
	loginPath := dbus.ObjectPath("/org/freedesktop/secrets/collection/login")
	if svc.CheckCollectionPath(loginPath) == nil {
		return nil
	}

	// Create a persistent collection via D-Bus with alias "default".
	// We use a raw D-Bus call instead of svc.CreateCollection(label) because
	// the library method does not accept an alias parameter. The "default"
	// alias is required so that gnome-keyring's GetLoginCollection() can
	// discover the collection via /org/freedesktop/secrets/aliases/default.
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("Login"),
	}
	var collectionPath, promptPath dbus.ObjectPath
	obj := svc.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")
	err = obj.Call("org.freedesktop.Secret.Service.CreateCollection", 0, props, "default").
		Store(&collectionPath, &promptPath)
	if err != nil {
		return fmt.Errorf("failed to create keyring collection: %w", err)
	}

	// If no prompt was returned, the collection was created immediately.
	if promptPath == dbus.ObjectPath("/") {
		return nil
	}

	// A prompt was returned — trigger it so the OS displays a password dialog.
	promptObj := svc.Object("org.freedesktop.secrets", promptPath)
	if err := promptObj.Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Err; err != nil {
		return fmt.Errorf("failed to trigger keyring prompt: %w", err)
	}

	// Poll until the default alias points to a real collection, indicating
	// the user completed the password prompt. D-Bus signal delivery is
	// unreliable in some environments (notably WSL), so polling is more
	// robust than waiting for the Prompt.Completed signal.
	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("keyring collection creation cancelled: %w", ctx.Err())
		case <-deadline:
			return fmt.Errorf("timed out waiting for keyring password prompt to complete")
		case <-ticker.C:
			var alias dbus.ObjectPath
			call := obj.Call("org.freedesktop.Secret.Service.ReadAlias", 0, "default")
			if call.Err == nil {
				_ = call.Store(&alias)
				if alias != "/" && alias != "" {
					return nil
				}
			}
		}
	}
}
