package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// freshTokenHTTPDo returns a fake httpDo that answers every refresh with the
// given access/refresh token pair and counts calls into *calls.
func freshTokenHTTPDo(calls *int, mu *sync.Mutex, accessToken, refreshToken string) func(*http.Request) (*http.Response, error) {
	return func(_ *http.Request) (*http.Response, error) {
		mu.Lock()
		*calls++
		mu.Unlock()
		body := fmt.Sprintf(`{
			"access_token":  %q,
			"refresh_token": %q,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "openid"
		}`, accessToken, refreshToken)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

func seedStoredToken(t *testing.T, tm *TokenManager, store map[string]string, tokenName, accessToken, refreshToken string, expiresAt time.Time) {
	t.Helper()
	data, err := json.Marshal(&StoredToken{
		Name: tokenName,
		TokenSet: TokenSet{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		},
	})
	if err != nil {
		t.Fatalf("marshal seed token: %v", err)
	}
	store[tm.getKeyringName(tokenName)] = string(data)
}

func TestTokenManager_RefreshToken_ReusesConcurrentRefresh(t *testing.T) {
	// A forced refresh that has to wait on the cross-process lock must reuse
	// the token another process saved while it waited, instead of spending a
	// second refresh — refresh tokens rotate on use, and refreshing with the
	// already-rotated token yields invalid_grant.
	//
	// Cannot run in parallel: we swap the package-level acquireRefreshLock.

	originalLock := acquireRefreshLock
	t.Cleanup(func() { acquireRefreshLock = originalLock })

	reached := make(chan struct{})
	release := make(chan struct{})
	acquireRefreshLock = func(_, _ string) (func(), error) {
		close(reached)
		<-release
		return func() {}, nil
	}

	tm, store := newTMWithFakeKeyring(t)
	var mu sync.Mutex
	var providerCalls int
	tm.flow.httpDo = freshTokenHTTPDo(&providerCalls, &mu, "should-not-be-used", "should-not-be-used")

	seedStoredToken(t, tm, store, "my-token", "stale-access", "refresh-1", time.Now().Add(1*time.Minute))

	type result struct {
		tokens *TokenSet
		err    error
	}
	done := make(chan result, 1)
	go func() {
		tokens, err := tm.RefreshToken("my-token")
		done <- result{tokens, err}
	}()

	// The goroutine has taken its pre-lock snapshot and is now blocked on the
	// lock. Simulate another process refreshing and saving in the meantime.
	<-reached
	seedStoredToken(t, tm, store, "my-token", "fresh-access", "refresh-2", time.Now().Add(1*time.Hour))
	close(release)

	res := <-done
	if res.err != nil {
		t.Fatalf("RefreshToken() error = %v, want nil", res.err)
	}
	if res.tokens.AccessToken != "fresh-access" {
		t.Errorf("RefreshToken() access token = %q, want %q (must reuse the concurrent refresh)", res.tokens.AccessToken, "fresh-access")
	}
	if res.tokens.RefreshToken != "refresh-2" {
		t.Errorf("RefreshToken() refresh token = %q, want %q", res.tokens.RefreshToken, "refresh-2")
	}
	mu.Lock()
	defer mu.Unlock()
	if providerCalls != 0 {
		t.Errorf("OAuth endpoint calls = %d, want 0 (concurrent refresh must be reused)", providerCalls)
	}
}

func TestTokenManager_RefreshToken_UnchangedStore_RefreshesUnderLock(t *testing.T) {
	// When nothing changed while waiting on the lock, a forced refresh must
	// actually refresh — even if the stored token still looks valid. This is
	// the 401-retry contract: the server just rejected this token, so
	// returning it unrefreshed would loop the caller into the same 401.
	//
	// Cannot run in parallel: we swap the package-level acquireRefreshLock.

	originalLock := acquireRefreshLock
	t.Cleanup(func() { acquireRefreshLock = originalLock })

	var lockCalls, unlockCalls int
	acquireRefreshLock = func(_, _ string) (func(), error) {
		lockCalls++
		return func() { unlockCalls++ }, nil
	}

	tm, store := newTMWithFakeKeyring(t)
	var mu sync.Mutex
	var providerCalls int
	tm.flow.httpDo = freshTokenHTTPDo(&providerCalls, &mu, "new-access", "refresh-2")

	// Looks valid (expires in an hour) — the server bounced it anyway.
	seedStoredToken(t, tm, store, "my-token", "rejected-access", "refresh-1", time.Now().Add(1*time.Hour))

	tokens, err := tm.RefreshToken("my-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v, want nil", err)
	}
	if tokens.AccessToken != "new-access" {
		t.Errorf("RefreshToken() access token = %q, want %q (forced refresh must not short-circuit)", tokens.AccessToken, "new-access")
	}
	if providerCalls != 1 {
		t.Errorf("OAuth endpoint calls = %d, want 1", providerCalls)
	}
	if lockCalls != 1 || unlockCalls != 1 {
		t.Errorf("lock/unlock calls = %d/%d, want 1/1", lockCalls, unlockCalls)
	}

	// The rotated set must have been persisted for other processes.
	var saved StoredToken
	if err := json.Unmarshal([]byte(store[tm.getKeyringName("my-token")]), &saved); err != nil {
		t.Fatalf("unmarshal saved token: %v", err)
	}
	if saved.RefreshToken != "refresh-2" {
		t.Errorf("persisted refresh token = %q, want %q", saved.RefreshToken, "refresh-2")
	}
}

func TestTokenManager_RefreshToken_LockFailure_StillRefreshes(t *testing.T) {
	// The lock is best-effort: a lock failure must not turn a forced refresh
	// into an error.
	//
	// Cannot run in parallel: we swap the package-level acquireRefreshLock.

	originalLock := acquireRefreshLock
	t.Cleanup(func() { acquireRefreshLock = originalLock })
	acquireRefreshLock = func(_, _ string) (func(), error) {
		return func() {}, fmt.Errorf("simulated lock failure")
	}

	// Silence the operator warning on stderr.
	restoreStderr := silenceStderr(t)
	defer restoreStderr()

	tm, store := newTMWithFakeKeyring(t)
	var mu sync.Mutex
	var providerCalls int
	tm.flow.httpDo = freshTokenHTTPDo(&providerCalls, &mu, "new-access", "refresh-2")

	seedStoredToken(t, tm, store, "my-token", "stale-access", "refresh-1", time.Now().Add(-1*time.Minute))

	tokens, err := tm.RefreshToken("my-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v, want nil (lock failure must not abort)", err)
	}
	if tokens.AccessToken != "new-access" {
		t.Errorf("RefreshToken() access token = %q, want %q", tokens.AccessToken, "new-access")
	}
	if providerCalls != 1 {
		t.Errorf("OAuth endpoint calls = %d, want 1", providerCalls)
	}
}

func TestTokenManager_RefreshToken_ConcurrentRotation(t *testing.T) {
	// End-to-end regression for the rotation landmine: two concurrent forced
	// refreshes (a long-running consumer reacting to a 401 racing a parallel dtctl
	// invocation) against a rotating provider. Uses the real file lock. The
	// provider accepts each refresh token exactly once — without the lock and
	// the re-read, the loser refreshes with an already-rotated token and gets
	// invalid_grant.
	//
	// Cannot run in parallel with lock-swapping tests (uses the real lock).

	tm, store := newTMWithFakeKeyring(t)

	// The fake-keyring map is not goroutine-safe; the concurrent callers
	// require synchronized storage.
	var storeMu sync.Mutex
	tm.deps.getToken = func(_ *TokenStore, name string) (string, error) {
		storeMu.Lock()
		defer storeMu.Unlock()
		v, ok := store[name]
		if !ok {
			return "", fmt.Errorf("token %q not found in keyring", name)
		}
		return v, nil
	}
	tm.deps.setToken = func(_ *TokenStore, name, val string) error {
		storeMu.Lock()
		defer storeMu.Unlock()
		store[name] = val
		return nil
	}
	// saveToken clears the scope-companion entry after a successful write, so
	// deleteToken must share the same lock as get/set.
	tm.deps.deleteToken = func(_ *TokenStore, name string) error {
		storeMu.Lock()
		defer storeMu.Unlock()
		delete(store, name)
		return nil
	}

	// Rotation-faithful provider: each refresh token is single-use.
	var providerMu sync.Mutex
	spent := map[string]bool{}
	generation := 0
	tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		form, _ := url.ParseQuery(string(bodyBytes))
		rt := form.Get("refresh_token")

		providerMu.Lock()
		defer providerMu.Unlock()
		if spent[rt] {
			body := `{"error":"invalid_grant","error_description":"refresh token already rotated"}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}
		spent[rt] = true
		generation++
		body := fmt.Sprintf(`{
			"access_token":  "access-gen-%d",
			"refresh_token": "refresh-gen-%d",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "openid"
		}`, generation, generation)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}

	const tokenName = "concurrent-rotation-token"
	seedStoredToken(t, tm, store, tokenName, "expired-access", "refresh-gen-0", time.Now().Add(-1*time.Minute))

	const callers = 4
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			_, err := tm.RefreshToken(tokenName)
			errs <- err
		}()
	}
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent RefreshToken() error = %v, want nil (rotation race)", err)
		}
	}
}

// silenceStderr redirects os.Stderr to a pipe for the duration of a test so
// expected operator warnings do not pollute test output. The returned restore
// function must be called before the test ends.
func silenceStderr(t *testing.T) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	return func() {
		_ = w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stderr = original
	}
}
