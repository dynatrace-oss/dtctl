package session

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-resty/resty/v2"
)

// connCountingServer starts an HTTP/1.1 test server that counts the TCP
// connections clients open to it.
func connCountingServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	conns := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			mu.Lock()
			conns++
			mu.Unlock()
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return conns
	}
}

// TestNewClient_ReusesConnectionsAcrossClients is #576: a long-lived consumer
// builds a client per command, and each one used to carry a private
// http.Transport, so every command dialed the tenant afresh. Two clients built
// one after the other must now ride one connection.
func TestNewClient_ReusesConnectionsAcrossClients(t *testing.T) {
	srv, conns := connCountingServer(t)

	for i := range 3 {
		c, err := NewForTesting(srv.URL, "test-token")
		if err != nil {
			t.Fatalf("client %d: NewForTesting() error = %v", i, err)
		}
		resp, err := c.HTTP().R().Get("/test")
		if err != nil {
			t.Fatalf("client %d: request failed: %v", i, err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("client %d: status = %d, want 200", i, resp.StatusCode())
		}
	}

	if got := conns(); got != 1 {
		t.Errorf("3 sequential clients opened %d connections, want 1 (idle pool not shared)", got)
	}
}

// TestNewClient_TransportMutatorsDoNotLeakAcrossClients pins why the shared
// pool sits behind a wrapper: resty's SetTLSClientConfig/SetProxy edit the
// client's *http.Transport in place, and on a bare shared transport one
// tenant's client would rewrite every other client's TLS trust or proxy.
func TestNewClient_TransportMutatorsDoNotLeakAcrossClients(t *testing.T) {
	a, err := NewClient("https://a.example.invalid", "token-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewClient("https://b.example.invalid", "token-b")
	if err != nil {
		t.Fatal(err)
	}

	a.HTTP().
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}). //nolint:gosec // the point of the test: must not reach the shared transport
		SetProxy("http://proxy.example.invalid:3128").
		SetCertificates(tls.Certificate{})

	pooled, ok := sharedTransport.(*pooledTransport)
	if !ok {
		t.Fatalf("shared transport is %T; resty's in-place mutators can edit it", sharedTransport)
	}
	shared := pooled.t
	// net/http fills in TLSClientConfig itself (NextProtos for HTTP/2) once
	// the transport is used, so check for what the client tried to set.
	if tc := shared.TLSClientConfig; tc != nil && (tc.InsecureSkipVerify || len(tc.Certificates) > 0) {
		t.Errorf("a client's SetTLSClientConfig/SetCertificates reached the shared transport: InsecureSkipVerify=%v certificates=%d",
			tc.InsecureSkipVerify, len(tc.Certificates))
	}
	req := httptest.NewRequest(http.MethodGet, "https://b.example.invalid/", nil)
	if proxyURL, _ := shared.Proxy(req); proxyURL != nil && proxyURL.Host == "proxy.example.invalid:3128" {
		t.Error("a client's SetProxy reached the shared transport")
	}
	if _, err := b.HTTP().Transport(); err == nil {
		t.Error("resty can reach the shared *http.Transport; its in-place mutators would leak across clients")
	}
}

// TestNewClient_KeepsPerClientState checks that only the RoundTripper is
// shared: timeout and cookie jar stay per client, so neither one tenant's
// timeout nor its session cookies reach another's client.
func TestNewClient_KeepsPerClientState(t *testing.T) {
	a, err := NewClient("https://a.example.invalid", "token-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewClient("https://b.example.invalid", "token-b")
	if err != nil {
		t.Fatal(err)
	}

	ha, hb := a.HTTP().GetClient(), b.HTTP().GetClient()
	if ha == hb {
		t.Fatal("clients share one *http.Client")
	}
	if ha.Jar == nil || hb.Jar == nil {
		t.Fatal("a client lost its cookie jar")
	}
	if ha.Jar == hb.Jar {
		t.Error("clients share one cookie jar; cookies would leak across tenants")
	}
	if ha.Transport != sharedTransport || hb.Transport != sharedTransport {
		t.Error("clients do not use the shared transport")
	}

	a.HTTP().SetTimeout(1)
	if hb.Timeout == ha.Timeout {
		t.Error("one client's timeout changed another's")
	}

	// A client that needs its own TLS or proxy settings replaces its
	// RoundTripper; that must not touch anyone else's.
	own := &http.Transport{}
	a.HTTP().SetTransport(own)
	if ha.Transport != own || hb.Transport != sharedTransport {
		t.Error("SetTransport on one client affected another")
	}
}

// TestDefaultTransportMatchesResty pins "behaviour is identical except for
// connection reuse": the shared transport carries the settings resty.New
// would have given each client.
func TestDefaultTransportMatchesResty(t *testing.T) {
	want, err := resty.New().Transport()
	if err != nil {
		t.Fatalf("resty default transport: %v", err)
	}
	got := newDefaultTransport()

	if got.ForceAttemptHTTP2 != want.ForceAttemptHTTP2 ||
		got.MaxIdleConns != want.MaxIdleConns ||
		got.MaxIdleConnsPerHost != want.MaxIdleConnsPerHost ||
		got.MaxConnsPerHost != want.MaxConnsPerHost ||
		got.IdleConnTimeout != want.IdleConnTimeout ||
		got.TLSHandshakeTimeout != want.TLSHandshakeTimeout ||
		got.ExpectContinueTimeout != want.ExpectContinueTimeout ||
		got.ResponseHeaderTimeout != want.ResponseHeaderTimeout ||
		got.DisableKeepAlives != want.DisableKeepAlives ||
		got.DisableCompression != want.DisableCompression ||
		(got.TLSClientConfig == nil) != (want.TLSClientConfig == nil) ||
		(got.Proxy == nil) != (want.Proxy == nil) ||
		(got.DialContext == nil) != (want.DialContext == nil) {
		t.Errorf("shared transport drifted from resty's default:\n got  %+v\n want %+v", got, want)
	}
}

// TestNewClient_CloseIdleConnectionsReachesThePool: http.Client's
// CloseIdleConnections only works if the RoundTripper implements it, and the
// wrapper hiding the shared transport from resty must not swallow it — a
// consumer calling it to release connections expects the next request to dial
// afresh.
func TestNewClient_CloseIdleConnectionsReachesThePool(t *testing.T) {
	srv, conns := connCountingServer(t)

	c, err := NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	get := func() {
		t.Helper()
		resp, err := c.HTTP().R().Get("/test")
		if err != nil || resp.StatusCode() != http.StatusOK {
			t.Fatalf("request failed: err=%v resp=%v", err, resp)
		}
	}

	get()
	get()
	if got := conns(); got != 1 {
		t.Fatalf("before CloseIdleConnections: %d connections, want 1", got)
	}

	c.HTTP().GetClient().CloseIdleConnections()
	get()
	if got := conns(); got != 2 {
		t.Errorf("after CloseIdleConnections: %d connections, want 2 (idle connection was not closed)", got)
	}
}

// TestNewClient_RefusedTransportSetterFailsRequests is the other half of the
// wrapper: a setter that cannot reach the shared pool must not be dropped
// silently either. A caller who asked for an egress proxy (or asked to bypass
// one, or for a pinned CA) and then sent requests without it would never
// know. Every later request on that client fails with
// ErrSharedTransportSetting instead — and sends nothing — while other clients
// are unaffected.
func TestNewClient_RefusedTransportSetterFailsRequests(t *testing.T) {
	if restyTransportRefusal == "" {
		t.Fatal("resty no longer refuses to mutate a non-*http.Transport; the guard cannot detect setter calls")
	}

	setters := map[string]func(*resty.Client){
		"SetProxy":           func(c *resty.Client) { c.SetProxy("http://proxy.example.invalid:3128") },
		"RemoveProxy":        func(c *resty.Client) { c.RemoveProxy() },
		"SetTLSClientConfig": func(c *resty.Client) { c.SetTLSClientConfig(&tls.Config{MinVersion: tls.VersionTLS13}) },
		"SetCertificates":    func(c *resty.Client) { c.SetCertificates(tls.Certificate{}) },
		"SetRootCertificateFromString": func(c *resty.Client) {
			c.SetRootCertificateFromString("-----BEGIN CERTIFICATE-----")
		},
		"SetClientRootCertificateFromString": func(c *resty.Client) {
			c.SetClientRootCertificateFromString("-----BEGIN CERTIFICATE-----")
		},
	}
	for name, set := range setters {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			hits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				hits++
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)

			a, err := NewForTesting(srv.URL, "token-a")
			if err != nil {
				t.Fatal(err)
			}
			b, err := NewForTesting(srv.URL, "token-b")
			if err != nil {
				t.Fatal(err)
			}

			set(a.HTTP())

			if _, err := a.HTTP().R().Get("/test"); !errors.Is(err, ErrSharedTransportSetting) {
				t.Errorf("request after a refused %s: err = %v, want ErrSharedTransportSetting", name, err)
			}
			mu.Lock()
			sent := hits
			mu.Unlock()
			if sent != 0 {
				t.Errorf("a request without the requested %s setting reached the server", name)
			}

			resp, err := b.HTTP().R().Get("/test")
			if err != nil || resp.StatusCode() != http.StatusOK {
				t.Errorf("another client was affected by %s: err=%v", name, err)
			}
		})
	}
}

// TestNewClient_PrivateTransportTakesSetters pins the documented per-client
// path: install NewTransport, then resty's setters apply to that client alone
// and requests go through.
func TestNewClient_PrivateTransportTakesSetters(t *testing.T) {
	srv, _ := connCountingServer(t)

	c, err := NewForTesting(srv.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP().SetTransport(NewTransport()).RemoveProxy()

	own, err := c.HTTP().Transport()
	if err != nil {
		t.Fatalf("private transport not visible to resty: %v", err)
	}
	if own.Proxy != nil {
		t.Error("RemoveProxy did not apply to the private transport")
	}
	if pooled := sharedTransport.(*pooledTransport).t; pooled.Proxy == nil {
		t.Error("RemoveProxy on a private transport reached the shared pool")
	}

	resp, err := c.HTTP().R().Get("/test")
	if err != nil || resp.StatusCode() != http.StatusOK {
		t.Fatalf("request over the private transport failed: err=%v", err)
	}
}
