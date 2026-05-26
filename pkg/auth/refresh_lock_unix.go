//go:build !windows

package auth

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// acquireRefreshLock acquires a cross-process exclusive lock for token refresh.
// Only one process at a time may refresh a given (environment, tokenName) pair,
// preventing the concurrent-refresh race where multiple parallel dtctl
// invocations all see a compact token (no access_token), simultaneously call
// the OAuth token endpoint with the same refresh_token, and all but one fail
// because OAuth refresh-token rotation invalidates the token after first use.
//
// The lock is a regular file under the OS temp directory, keyed by a SHA-256
// hash of environment+tokenName so different contexts never contend.
//
// Security notes:
//   - O_NOFOLLOW is set so that a pre-placed symlink in /tmp does not redirect
//     the open to an attacker-controlled file (relevant on Linux where /tmp is
//     world-writable). If the open fails because the path is a symlink, the
//     function returns a lock-failure, and the caller proceeds without the lock.
//   - The lock file is created with mode 0600 (owner read/write only).
//   - No token material is written to the lock file; it is used purely as a
//     synchronisation primitive.
//   - A local attacker who can predict the lock file name (the hash input is
//     public: environment identifier + token name) could pre-create the file
//     owned by themselves, causing an EACCES on open and a lock-failure. This
//     degrades back to the pre-fix behavior (racy concurrent refresh) but does
//     not expose credentials. This is an accepted residual risk given the
//     best-effort nature of the lock.
//   - If the lock file is deleted while held (e.g. /tmp cleanup, tmpwatch), a
//     new process can create a fresh inode at the same path and acquire an
//     independent lock while the original holder still holds the old inode.
//     Mutual exclusion is silently broken and both processes may call the OAuth
//     endpoint. The result is a possible invalid_grant error for one of them,
//     not a credential exposure. This is an accepted limitation of file-based
//     advisory locking.
//   - Hard links are not blocked by O_NOFOLLOW. An attacker with the same UID
//     could create a hard link to a user-owned file at the lock path. We never
//     write to the file descriptor, only flock(2) it, so no data is modified
//     or exposed. This is an accepted residual risk.
//
// Returns an unlock function and nil on success, or a no-op function and an
// error if the lock cannot be acquired. Callers must still proceed on error —
// the lock is best-effort; a failure to lock is not a hard error.
// acquireRefreshLock is a package-level variable (not a bare function) so
// tests can override it to exercise the best-effort fallthrough path in
// TokenManager.GetToken without forcing a real lock-acquisition failure.
var acquireRefreshLock = func(environment, tokenName string) (unlock func(), err error) {
	return doAcquireRefreshLock(environment, tokenName, refreshLockTimeout, refreshLockRetryInterval)
}

// doAcquireRefreshLock is the testable core of acquireRefreshLock. It accepts
// explicit timeout and retryInterval so tests can exercise the timeout and
// contention paths without waiting the full 30-second production timeout.
//
// The returned unlock function must be called exactly once.
func doAcquireRefreshLock(environment, tokenName string, timeout, retryInterval time.Duration) (unlock func(), err error) {
	lockPath := refreshLockPath(environment, tokenName)

	// O_NOFOLLOW prevents following a symlink that a local attacker may have
	// placed at this path to redirect the open to a sensitive file.
	// O_RDONLY follows the principle of least privilege: flock(2) only needs
	// an open file descriptor, not write access, and we never write to the
	// file — it is purely a synchronisation primitive.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return func() {}, fmt.Errorf("refresh lock: open %s: %w", lockPath, err)
	}

	// Use LOCK_NB (non-blocking) with a retry loop so that a hung process
	// holding the lock (e.g. OAuth endpoint is down) does not block all
	// waiters indefinitely. The deadline check fires before each sleep so the
	// actual wait never exceeds timeout by more than one syscall round-trip.
	deadline := time.Now().Add(timeout)
	for {
		lockErr := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if lockErr == nil {
			break
		}
		if !errors.Is(lockErr, unix.EWOULDBLOCK) {
			_ = f.Close()
			return func() {}, fmt.Errorf("refresh lock: flock %s: %w", lockPath, lockErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return func() {}, fmt.Errorf("refresh lock: timed out waiting for %s after %s", lockPath, timeout)
		}
		time.Sleep(retryInterval)
	}

	// The returned function must be called exactly once.
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}

// refreshLockPath returns the advisory lock file path for the given environment
// and token name. The filename is a truncated SHA-256 hash of their combination
// so that different contexts never share a lock file and the path itself does
// not contain any credential material.
func refreshLockPath(environment, tokenName string) string {
	h := sha256.Sum256([]byte(environment + ":" + tokenName))
	name := fmt.Sprintf("dtctl-token-refresh-%x.lock", h[:8])
	return filepath.Join(os.TempDir(), name)
}
