// Package httpclient provides an HTTP client for Dynatrace APIs.
//
// It wraps [resty] with Dynatrace-specific defaults: automatic retry on
// 429/5xx, Bearer/Api-Token auth scheme selection, User-Agent with AI agent
// detection, gzip, and redacted verbose logging.
//
// # Constructor
//
// Use [New] with functional [Option] values:
//
//	client, err := httpclient.New("https://abc.apps.dynatrace.com",
//	    httpclient.WithToken("dt0c01.xxx"),
//	    httpclient.WithUserAgent("myapp/1.0"),
//	)
//
// # Errors
//
// API errors are returned as [*APIError] and can be inspected with
// errors.As. Sentinel errors ([ErrUnauthorized], [ErrNotFound], etc.)
// support errors.Is for common status codes.
package httpclient
