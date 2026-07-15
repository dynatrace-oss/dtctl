//go:build windows

package session

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// acquireRefreshLock acquires a cross-process exclusive lock for token refresh
// on Windows via LockFileEx, mirroring the flock(2) implementation in
// refresh_lock_unix.go: same lock-file path (so dtctl, dynatui, and plugins
// built on this package serialise against each other), same timeout and retry
// semantics, same best-effort contract (callers proceed on error).
//
// LockFileEx takes a byte-range lock on the first byte of the lock file. The
// lock is per-handle, and every acquisition opens its own handle, so both
// goroutines within one process and separate processes contend correctly.
// The file itself is never read or written — it is purely a synchronisation
// primitive and carries no token material.
//
// Declared as a var (not a bare function) to mirror the Unix implementation
// and keep the override hook available for tests on both platforms.
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

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return func() {}, fmt.Errorf("refresh lock: open %s: %w", lockPath, err)
	}

	// LOCKFILE_FAIL_IMMEDIATELY (the LOCK_NB analogue) with a retry loop so
	// that a hung process holding the lock (e.g. OAuth endpoint is down) does
	// not block all waiters indefinitely. The deadline check fires before
	// each sleep so the actual wait never exceeds timeout by more than one
	// syscall round-trip.
	handle := windows.Handle(f.Fd())
	deadline := time.Now().Add(timeout)
	for {
		ol := new(windows.Overlapped)
		lockErr := windows.LockFileEx(handle,
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, ol)
		if lockErr == nil {
			break
		}
		if !errors.Is(lockErr, windows.ERROR_LOCK_VIOLATION) {
			_ = f.Close()
			return func() {}, fmt.Errorf("refresh lock: LockFileEx %s: %w", lockPath, lockErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return func() {}, fmt.Errorf("refresh lock: timed out waiting for %s after %s", lockPath, timeout)
		}
		time.Sleep(retryInterval)
	}

	// The returned function must be called exactly once.
	return func() {
		ol := new(windows.Overlapped)
		_ = windows.UnlockFileEx(handle, 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}
