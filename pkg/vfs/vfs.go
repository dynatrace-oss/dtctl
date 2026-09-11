// Package vfs is the file-access seam between dtctl commands and the
// filesystem. CLI invocations read and write the host filesystem directly
// (the default). Embedded invocations (the service engine, `dtctl serve`)
// install a per-request FS — typically a MapFS built from the request's
// virtual files — so `apply -f x.yaml` works against files that exist only
// in the request, and writebacks (e.g. `apply --write-id`) land back in the
// request instead of on the host (docs/dev/SERVICE_ENGINE_DESIGN.md).
//
// Only user-supplied paths go through this seam. Internal scratch files
// (editor round-trip temp files, spill buffers, caches) deliberately stay on
// the host filesystem: they are implementation detail, not request state.
package vfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
)

// FS is the minimal file-access contract dtctl commands need for
// user-supplied paths: whole-file reads and whole-file writes.
type FS interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

// osFS is the host filesystem.
type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// active is the installed implementation. Package-level (rather than
// threaded through every call chain) because dtctl serializes invocations —
// see cmd.Run — and the swap happens under that serialization.
var active FS = osFS{}

// SetActive installs f as the file-access implementation and returns the
// previous one so callers can restore it. nil selects the host filesystem.
// Callers own serialization: install/restore must not race with an executing
// invocation.
func SetActive(f FS) FS {
	prev := active
	if f == nil {
		f = osFS{}
	}
	active = f
	return prev
}

// ReadFile reads a user-supplied path through the active FS.
func ReadFile(name string) ([]byte, error) { return active.ReadFile(name) }

// WriteFile writes a user-supplied path through the active FS.
func WriteFile(name string, data []byte, perm fs.FileMode) error {
	return active.WriteFile(name, data, perm)
}

// ReadFileOrStdin reads name through the active FS, with "-" meaning the
// process stdin. It is the shared implementation behind the CLI's
// file-or-stdin flag convention.
func ReadFileOrStdin(name string) ([]byte, error) {
	if name == "-" {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		return content, nil
	}
	return ReadFile(name)
}
