package session

import (
	"errors"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
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
// *http.Transport" and those calls cannot apply; the client's
// transportSetterGuard notices the refusal and fails that client's requests
// with ErrSharedTransportSetting, so they fail closed and loudly rather than
// leak or be silently dropped. A consumer that needs custom TLS or proxy
// settings installs its own transport with
// Client.HTTP().SetTransport(NewTransport()), which replaces only that
// client's RoundTripper, and then calls the setters.
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

// NewTransport returns a new, private *http.Transport with the same settings
// as the shared pool. It is the per-client configuration path for a consumer
// that needs its own proxy or TLS settings: install it with
// Client.HTTP().SetTransport, after which resty's transport setters
// (SetProxy, SetTLSClientConfig, SetCertificates, …) apply to that client
// alone. The client then no longer shares upstream connections with anyone.
func NewTransport() *http.Transport {
	return newDefaultTransport()
}

// ErrSharedTransportSetting is returned by every request on a Client after
// one of resty's transport setters was called on it while it still sent
// through the shared pool (see Client.HTTP).
//
// resty applies those setters by editing the client's *http.Transport in
// place. The shared pool is hidden from that on purpose (see
// sharedTransport), so the setter cannot take effect — and resty reports the
// failure only to the client's logger, while returning the client as if it
// had worked. Letting the client carry on would send requests without the
// proxy or TLS settings its caller asked for: a required egress proxy
// bypassed, a pinned CA ignored. Failing every request instead is the one
// way resty's API leaves to make that detectable.
var ErrSharedTransportSetting = errors.New(
	"session: a resty transport setter (SetProxy, RemoveProxy, SetTLSClientConfig, SetCertificates, " +
		"SetRootCertificate, …) was called on a client that uses the shared connection pool, so the " +
		"setting was not applied; install a private transport first with " +
		"HTTP().SetTransport(session.NewTransport()) and then call the setter")

// restyTransportRefusal is the message of the error resty logs when a
// transport setter finds a RoundTripper that is not an *http.Transport. It is
// taken from resty itself rather than spelled out, so a reworded message in a
// resty upgrade cannot quietly disable the guard; if resty ever stops
// refusing, TestNewClient_TransportMutatorsDoNotLeakAcrossClients fails.
var restyTransportRefusal = func() string {
	_, err := resty.New().SetTransport(&pooledTransport{}).Transport()
	if err == nil {
		return ""
	}
	return err.Error()
}()

// transportSetterGuard is a session client's resty logger and pre-request
// hook. As the logger it discards resty's internal log output (errors reach
// callers as returned values) — except the refusal of a transport setter,
// which it records. As the hook it then fails every request with
// ErrSharedTransportSetting (see there for why).
//
// A consumer that replaces the logger with SetLogger takes over that
// message; it reaches their logger instead of being dropped, so it still
// is not silent.
type transportSetterGuard struct {
	refused atomic.Bool
}

func (g *transportSetterGuard) Errorf(_ string, v ...interface{}) {
	if restyTransportRefusal == "" {
		return
	}
	for _, arg := range v {
		if err, ok := arg.(error); ok && err.Error() == restyTransportRefusal {
			g.refused.Store(true)
		}
	}
}

func (*transportSetterGuard) Warnf(string, ...interface{})  {}
func (*transportSetterGuard) Debugf(string, ...interface{}) {}

func (g *transportSetterGuard) checkRequest(*resty.Client, *resty.Request) error {
	if g.refused.Load() {
		return ErrSharedTransportSetting
	}
	return nil
}
