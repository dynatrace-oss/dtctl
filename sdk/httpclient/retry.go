package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/go-resty/resty/v2"
)

// IsRetryable reports whether a request that ended in r and err is safe to
// send again automatically. It is the retry condition [New] installs, and the
// session client in sdk/session uses it too.
//
// A request is resent only when sending it twice cannot do twice the work:
//
//   - 429 Too Many Requests, for any method: the server turned the request
//     away without processing it.
//   - 503 Service Unavailable carrying a Retry-After header, for any method:
//     the server is explicitly asking to be called again later.
//   - A connection that could not be established (a dial error), for any
//     method: the request never reached the server.
//   - Any other 5xx, or any other transport error, only for idempotent methods
//     (GET, HEAD, OPTIONS, TRACE, PUT, DELETE; RFC 9110 section 9.2.2).
//
// A POST or PATCH that fails with a 500, a 502, a 504 or a dropped connection
// is not resent: the server may already have acted on it, and a second
// attempt could start a second workflow run, create a duplicate object, or
// run (and bill) a DQL query twice.
//
// A request that never produced a response (r or r.Request is nil) is never
// retried, and neither is one whose context was cancelled or timed out.
func IsRetryable(r *resty.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		// A nil response means the request never left the process: resty's own
		// parseRequestURL, or the request-path guard, rejected it before a transport
		// was involved. Retrying cannot change that outcome -- and resty consults
		// the retry conditions even for an error it has marked non-retryable, then
		// dereferences the nil response while preparing the retry, so answering
		// "yes" here is a panic rather than a wasted attempt.
		if r == nil || r.Request == nil {
			return false
		}
		return isIdempotent(r.Request.Method) || isDialError(err)
	}
	if r == nil {
		return false
	}

	switch statusCode := r.StatusCode(); {
	case statusCode == http.StatusTooManyRequests:
		return true
	case statusCode == http.StatusServiceUnavailable && r.Header().Get("Retry-After") != "":
		return true
	case statusCode >= 500:
		return r.Request != nil && isIdempotent(r.Request.Method)
	default:
		return false
	}
}

// isIdempotent reports whether method is idempotent per RFC 9110 section
// 9.2.2, i.e. whether sending the same request twice has the same effect on
// the server as sending it once.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace,
		http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isDialError reports whether err is a failure to open the connection
// (connection refused, DNS lookup failure, dial timeout). No byte of the
// request was written, so the server cannot have acted on it.
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}
