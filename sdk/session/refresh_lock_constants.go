package session

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// These constants are declared in a non-build-constrained file so that they
// compile on all platforms. They are consumed by both lock implementations:
// flock(2) on Unix (refresh_lock_unix.go) and LockFileEx on Windows
// (refresh_lock_windows.go).
const (
	// refreshLockTimeout is the maximum time to wait for a cross-process token
	// refresh lock before giving up and proceeding without it. A hung refresh
	// process (e.g. OAuth endpoint unresponsive) therefore cannot block other
	// dtctl invocations for more than this duration; they fall back to the
	// pre-lock racy behavior, which was always the worst case.
	refreshLockTimeout = 30 * time.Second

	// refreshLockRetryInterval is the polling interval for the non-blocking
	// lock retry loop.
	refreshLockRetryInterval = 50 * time.Millisecond
)

// refreshLockPath returns the advisory lock file path for the given environment
// and token name. The filename is a truncated SHA-256 hash of their combination
// so that different contexts never share a lock file and the path itself does
// not contain any credential material. The path is identical for every binary
// built on this package (dtctl, dynatui, plugins), which is what makes the
// lock effective across different consumers sharing the same token store.
func refreshLockPath(environment, tokenName string) string {
	h := sha256.Sum256([]byte(environment + ":" + tokenName))
	name := fmt.Sprintf("dtctl-token-refresh-%x.lock", h[:8])
	return filepath.Join(os.TempDir(), name)
}
