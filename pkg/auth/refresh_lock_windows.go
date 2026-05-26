//go:build windows

package auth

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

// windowsRefreshLocks holds one in-process mutex per (environment, tokenName)
// key. This serialises concurrent goroutines within the same process on Windows,
// matching the behaviour provided by flock(2) on Unix.
//
// Cross-process serialisation is not implemented on Windows — LockFileEx-based
// locking is left as a future improvement. The existing best-effort behaviour
// (one process may still race another) is preserved for cross-process scenarios.
var windowsRefreshLocks sync.Map // map[string]*sync.Mutex

// acquireRefreshLock serialises token refresh within the current process on
// Windows. It uses a per-key in-process mutex instead of flock(2) (which is
// not available on Windows). Cross-process races are still possible but
// uncommon given dtctl's primary usage on macOS/Linux.
//
// Declared as a var (not a bare function) to mirror the Unix implementation
// and keep the override hook available for tests on both platforms.
var acquireRefreshLock = func(environment, tokenName string) (unlock func(), err error) {
	key := windowsRefreshLockKey(environment, tokenName)
	actual, _ := windowsRefreshLocks.LoadOrStore(key, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }, nil
}

// windowsRefreshLockKey returns a short hash key for the given environment and
// token name. It mirrors the naming logic in the Unix implementation so the
// two are conceptually consistent, though on Windows the key is only used as
// an in-process map key rather than a filesystem path.
func windowsRefreshLockKey(environment, tokenName string) string {
	h := sha256.Sum256([]byte(environment + ":" + tokenName))
	return fmt.Sprintf("%x", h[:8])
}
