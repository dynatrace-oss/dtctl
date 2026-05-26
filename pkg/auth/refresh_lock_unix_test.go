//go:build !windows

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDoAcquireRefreshLock_HappyPath verifies that the lock can be acquired
// and released, and that the lock file is created under os.TempDir().
func TestDoAcquireRefreshLock_HappyPath(t *testing.T) {
	t.Parallel()

	env := "prod"
	tokenName := t.Name()

	unlock, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}
	defer unlock()

	// Verify the lock file exists.
	lockPath := refreshLockPath(env, tokenName)
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Errorf("lock file not found at %s: %v", lockPath, statErr)
	}
}

// TestDoAcquireRefreshLock_Contention verifies that a second caller blocks
// while the first holds the lock and succeeds once the first releases it.
func TestDoAcquireRefreshLock_Contention(t *testing.T) {
	t.Parallel()

	env := "sprint"
	tokenName := t.Name()

	// Acquire the lock from the test goroutine first.
	unlock1, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Start a second goroutine that tries to acquire the same lock. It must
	// not succeed until after unlock1() is called.
	type result struct {
		unlock func()
		err    error
	}
	ch := make(chan result, 1)
	started := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Close started before entering doAcquireRefreshLock so the test
		// goroutine knows we have reached the contention point. We then sleep
		// a short grace period below to allow the first flock attempt to
		// actually return EWOULDBLOCK and enter the retry loop.
		close(started)
		u, e := doAcquireRefreshLock(env, tokenName, 5*time.Second, 10*time.Millisecond)
		ch <- result{u, e}
	}()

	// Wait for the goroutine to be scheduled, then give a small grace period
	// so that the first flock(LOCK_NB) call inside doAcquireRefreshLock has
	// returned EWOULDBLOCK and the goroutine is sleeping in the retry loop.
	<-started
	time.Sleep(50 * time.Millisecond)

	// The channel must still be empty — the goroutine is waiting.
	select {
	case res := <-ch:
		t.Fatalf("second acquire returned before first unlock: err=%v", res.err)
	default:
		// expected: still blocked
	}

	// Release the first lock; the goroutine should now unblock.
	unlock1()

	wg.Wait()
	res := <-ch
	if res.err != nil {
		t.Fatalf("second acquire failed after first unlock: %v", res.err)
	}
	res.unlock()
}

// TestDoAcquireRefreshLock_Timeout verifies that doAcquireRefreshLock returns
// an error when it cannot acquire the lock within the given timeout.
func TestDoAcquireRefreshLock_Timeout(t *testing.T) {
	t.Parallel()

	env := "dev"
	tokenName := t.Name()

	// Hold the lock from the test goroutine.
	unlock, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("initial acquire failed: %v", err)
	}
	defer unlock()

	// A second acquire with a very short timeout must fail.
	start := time.Now()
	_, err2 := doAcquireRefreshLock(env, tokenName, 80*time.Millisecond, 10*time.Millisecond)
	elapsed := time.Since(start)

	if err2 == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// Allow generous headroom (×5) for slow CI machines.
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout took too long: %v (expected ~80ms)", elapsed)
	}
}

// TestDoAcquireRefreshLock_SymlinkRejected verifies that O_NOFOLLOW causes
// the open to fail when the lock path is a symlink, preventing a local
// attacker from redirecting the lock to an arbitrary file.
func TestDoAcquireRefreshLock_SymlinkRejected(t *testing.T) {
	t.Parallel()

	env := "prod"
	tokenName := t.Name()
	lockPath := refreshLockPath(env, tokenName)

	// Create an innocuous target file and a symlink at the lock path.
	target := lockPath + ".target"
	if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
		t.Fatalf("failed to create symlink target: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(target) })

	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lockPath) })

	_, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when lock path is a symlink, got nil")
	}
}

// TestRefreshLockPath_Uniqueness verifies that different (environment,
// tokenName) pairs produce different lock file paths, and that the same pair
// always produces the same path.
func TestRefreshLockPath_Uniqueness(t *testing.T) {
	t.Parallel()

	cases := [][2]string{
		{"prod", "oauth:default"},
		{"dev", "oauth:default"},
		{"sprint", "oauth:default"},
		{"prod", "oauth:other"},
	}

	seen := make(map[string]bool)
	for _, c := range cases {
		p := refreshLockPath(c[0], c[1])
		key := c[0] + "|" + c[1]

		if seen[p] {
			t.Errorf("path collision: %s returned %s which was already seen", key, p)
		}
		seen[p] = true

		// Path must be directly inside os.TempDir(). Use filepath.Clean on
		// both sides because os.TempDir() may return a trailing slash on
		// some platforms (e.g. macOS).
		if dir := filepath.Dir(p); dir != filepath.Clean(os.TempDir()) {
			t.Errorf("path %s is not under TempDir (%s)", p, os.TempDir())
		}

		// Same inputs must produce the same path.
		if p2 := refreshLockPath(c[0], c[1]); p2 != p {
			t.Errorf("non-deterministic path for %s: got %s then %s", key, p, p2)
		}
	}
}

// TestDoAcquireRefreshLock_LockFileMode verifies that the lock file is created
// with mode 0600 (owner read/write only, no group/world access).
func TestDoAcquireRefreshLock_LockFileMode(t *testing.T) {
	t.Parallel()

	env := "prod"
	tokenName := t.Name()

	unlock, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unlock()

	lockPath := refreshLockPath(env, tokenName)
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}

	// Mask to permission bits only (strip file-type bits).
	const wantMode = os.FileMode(0600)
	gotMode := info.Mode().Perm()
	if gotMode != wantMode {
		t.Errorf("lock file mode = %04o, want %04o", gotMode, wantMode)
	}
}

// TestDoAcquireRefreshLock_UnlockReleasesLock verifies that after unlock() is
// called, a new caller can immediately acquire the same lock, demonstrating
// that the flock is actually released rather than only the file being closed.
func TestDoAcquireRefreshLock_UnlockReleasesLock(t *testing.T) {
	t.Parallel()

	env := "dev"
	tokenName := t.Name()

	unlock1, err := doAcquireRefreshLock(env, tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	unlock1()

	// After unlock the lock file descriptor is closed; a new flock must
	// succeed immediately (very short timeout to catch regressions fast).
	unlock2, err := doAcquireRefreshLock(env, tokenName, 200*time.Millisecond, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("second acquire after unlock failed: %v", err)
	}
	defer unlock2()
}

// TestDoAcquireRefreshLock_SerializationOrder verifies the key invariant: N
// concurrent goroutines all acquire the same lock sequentially, never
// overlapping. Each goroutine records [enter, exit] timestamps, and the test
// asserts that no two intervals overlap.
func TestDoAcquireRefreshLock_SerializationOrder(t *testing.T) {
	t.Parallel()

	const n = 5
	env := "prod"
	tokenName := t.Name()

	type interval struct{ enter, exit time.Time }
	results := make([]interval, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			unlock, err := doAcquireRefreshLock(env, tokenName, 10*time.Second, 5*time.Millisecond)
			if err != nil {
				t.Errorf("goroutine %d: acquire failed: %v", idx, err)
				return
			}
			results[idx].enter = time.Now()
			time.Sleep(5 * time.Millisecond) // simulate a short refresh
			results[idx].exit = time.Now()
			unlock()
		}(i)
	}

	wg.Wait()

	// Verify no two non-zero intervals overlap.
	for i := range n {
		if results[i].enter.IsZero() {
			continue // goroutine failed, already reported
		}
		for j := i + 1; j < n; j++ {
			if results[j].enter.IsZero() {
				continue
			}
			aEnter, aExit := results[i].enter, results[i].exit
			bEnter, bExit := results[j].enter, results[j].exit
			overlap := aEnter.Before(bExit) && bEnter.Before(aExit)
			if overlap {
				t.Errorf(
					"goroutines %d and %d overlapped: [%v,%v] vs [%v,%v]",
					i, j, aEnter, aExit, bEnter, bExit,
				)
			}
		}
	}
}

// TestRefreshLockPath_DoesNotContainCredentialMaterial verifies that the lock
// file path does not embed the raw environment or token name strings, ensuring
// that observing the file path does not reveal credential context.
func TestRefreshLockPath_DoesNotContainCredentialMaterial(t *testing.T) {
	t.Parallel()

	sensitive := []string{
		"production.example.com",
		"oauth:my-secret-token",
		"my-secret-token",
	}

	p := refreshLockPath("production.example.com", "oauth:my-secret-token")
	base := filepath.Base(p)

	for _, s := range sensitive {
		if strings.Contains(base, s) {
			t.Errorf("lock file name %q contains sensitive string %q", base, s)
		}
	}
}

// TestDoAcquireRefreshLock_DifferentEnvironmentsDoNotContest verifies that
// locks for different (environment, tokenName) pairs are independent — holding
// a lock for "prod" does not block acquiring a lock for "dev".
func TestDoAcquireRefreshLock_DifferentEnvironmentsDoNotContest(t *testing.T) {
	t.Parallel()

	tokenName := t.Name()

	unlockProd, err := doAcquireRefreshLock("prod", tokenName, 1*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("prod acquire failed: %v", err)
	}
	defer unlockProd()

	// Acquiring a lock for a different environment must not block.
	unlockDev, err := doAcquireRefreshLock("dev", tokenName, 200*time.Millisecond, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("dev acquire failed while prod lock held: %v", err)
	}
	defer unlockDev()
}
