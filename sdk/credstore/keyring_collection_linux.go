//go:build linux

package credstore

import (
	"context"
	"fmt"
	"time"

	dbus "github.com/godbus/dbus/v5"
	ss "github.com/zalando/go-keyring/secret_service"
)

// EnsureKeyringCollection checks whether a usable Secret Service collection
// exists and, if not, creates a persistent "login" collection.
//
// On Linux/WSL gnome-keyring may start with only a transient "session"
// collection; this function creates the permanent one, which may trigger
// an OS password prompt.
func EnsureKeyringCollection(ctx context.Context) error {
	svc, err := ss.NewSecretService()
	if err != nil {
		return fmt.Errorf("cannot connect to Secret Service: %w", err)
	}

	loginPath := dbus.ObjectPath("/org/freedesktop/secrets/collection/login")
	if svc.CheckCollectionPath(loginPath) == nil {
		return nil
	}

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

	if promptPath == dbus.ObjectPath("/") {
		return nil
	}

	promptObj := svc.Object("org.freedesktop.secrets", promptPath)
	if err := promptObj.Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Err; err != nil {
		return fmt.Errorf("failed to trigger keyring prompt: %w", err)
	}

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
