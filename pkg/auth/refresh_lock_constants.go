package auth

import "time"

// These constants are declared in a non-build-constrained file so that they
// compile on all platforms, but they are only consumed by the Unix
// implementation (refresh_lock_unix.go). The Windows no-op
// (refresh_lock_windows.go) does not reference them.
const (
	// refreshLockTimeout is the maximum time to wait for a cross-process token
	// refresh lock before giving up and proceeding without it. A hung refresh
	// process (e.g. OAuth endpoint unresponsive) therefore cannot block other
	// dtctl invocations for more than this duration; they fall back to the
	// pre-lock racy behavior, which was always the worst case.
	refreshLockTimeout = 30 * time.Second

	// refreshLockRetryInterval is the polling interval for the LOCK_NB retry loop.
	refreshLockRetryInterval = 50 * time.Millisecond
)
