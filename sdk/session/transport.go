package session

import (
	"net"
	"net/http"
	"runtime"
	"time"
)

// sharedTransport is the one connection pool every Client built by NewClient
// sends through.
//
// resty.New installs a private http.Transport per client, and so a private,
// empty idle pool. dtctl builds a client per command, so in a long-lived
// process (pkg/engine, `dtctl serve`) every request paid a fresh TCP + TLS
// handshake to a tenant it had just talked to (#576). Sharing the transport is
// what lets the second request reuse the first one's connection.
//
// Sharing is safe across tenants because nothing tenant-specific lives in it:
//
//   - The credential is a per-request Authorization header, not connection
//     state. There are no client certificates, so a pooled connection is
//     authenticated to no one.
//   - net/http keys the pool by proxy, scheme and host:port, so two
//     environments (two hosts) never share a connection; repeated calls for
//     one environment are exactly the case that should. Requests that do share
//     a connection each carry their own Authorization header.
//   - Everything else that is per client stays per client: each Client still
//     gets its own http.Client (timeout, redirect policy) and its own cookie
//     jar, because resty.New still builds those — only the RoundTripper is
//     replaced.
//
// The transport is wrapped in pooledTransport rather than handed to resty as
// a bare *http.Transport on purpose. resty's in-place mutators — SetProxy,
// RemoveProxy, SetTLSClientConfig, SetCertificates and the
// Set(Client)RootCertificate family — edit whatever *http.Transport the
// client holds. Shared, one tenant's client calling them would rewrite the
// proxy or TLS trust of every other client in the process, racing with
// requests in flight. Behind the wrapper resty's Transport() reports "not an
// *http.Transport" and those calls become no-ops (their error goes to the
// client's resty logger, which NewClient silences): they fail closed rather
// than leak. A consumer that needs custom TLS or proxy settings installs its
// own transport with Client.HTTP().SetTransport, which replaces only that
// client's RoundTripper.
var sharedTransport http.RoundTripper = &pooledTransport{t: newDefaultTransport()}

// pooledTransport hides the shared *http.Transport from resty's type
// assertion (see sharedTransport).
type pooledTransport struct {
	t *http.Transport
}

// RoundTrip implements http.RoundTripper. *http.Transport is safe for
// concurrent use, so the wrapper needs no locking of its own.
func (p *pooledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return p.t.RoundTrip(req)
}

// CloseIdleConnections lets http.Client.CloseIdleConnections keep working on
// a session client: a consumer that calls it to release connections promptly
// (shutdown, a network change, a test) gets exactly that. Because the pool is
// shared, it closes the idle connections of every Client in the process, not
// just the caller's. That costs the others at most one fresh handshake each:
// an idle connection is a cache, not state — in-flight requests are
// untouched, and nothing tenant-specific lives on a connection (see
// sharedTransport).
func (p *pooledTransport) CloseIdleConnections() {
	p.t.CloseIdleConnections()
}

// newDefaultTransport builds a transport with the settings resty v2's
// createTransport uses, so the only observable change from sharing it is
// connection reuse. (resty also sets the deprecated net.Dialer.DualStack,
// which has been the default behavior since Go 1.12.)
func newDefaultTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
	}
}
