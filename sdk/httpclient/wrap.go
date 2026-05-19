package httpclient

import "github.com/go-resty/resty/v2"

// Wrap creates a [Client] that wraps an already-configured [*resty.Client].
// This is useful when integrating the SDK into an existing application that
// already manages its own HTTP client (e.g., the dtctl CLI).
//
// The caller is responsible for configuring auth, retries, timeouts, etc. on
// the resty client before passing it in.
func Wrap(rc *resty.Client) *Client {
	if rc == nil {
		panic("httpclient.Wrap: resty client must not be nil")
	}
	return &Client{
		http:    rc,
		baseURL: rc.BaseURL,
		logger:  noopLogger{},
	}
}
