package httpclient

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/go-resty/resty/v2"
)

// PathSegment escapes v for use as one complete segment of a request path.
//
// It is [net/url.PathEscape] plus the colon. PathEscape leaves ':' intact, and
// several Dynatrace collections spell an action as a suffix on the resource
// segment -- "/analyzers/%s:poll", "/bucket-definitions/%s:truncate". An id
// carrying a colon therefore names an *operation* rather than a resource:
//
//	dtctl describe analyzer "foo:poll"  ->  GET /analyzers/foo:poll
//
// which polls the analyzer "foo" instead of looking up the one that was named.
// That is the same retargeting as '/', '?' and '#', and it is invisible for the
// same reason: the request succeeds, against something else.
//
// Escaping the colon costs nothing, because an action suffix is a literal in
// the format string and never passes through here -- only the interpolated
// value does. The other characters PathEscape leaves alone ('$', '&', '+', '-',
// '.', '=', '@', '_', '~') route nothing in a path segment: '?' and '#' are
// already escaped, so '&' and '=' cannot begin a query, and ".." is refused by
// [CheckRequestPath]. They stay readable on the wire.
//
// A value that is a path by design does not belong here. sdk/api/appengine
// escapes a nested function name part by part instead.
func PathSegment(v string) string {
	return strings.ReplaceAll(url.PathEscape(v), ":", "%3A")
}

// CheckRequestPath reports whether reqPath addresses what it spells.
//
// Resource ids reach dtctl as command-line arguments and as fields in applied
// files, and every API handler interpolates them into a request path. A value
// carrying a character that has meaning in a URL then addresses a *different*
// resource than the caller named -- and the request succeeds against it:
//
//	dtctl delete slo "<id>#x"  ->  DELETE /platform/slo/v1/slos/<id>
//
// because net/http drops the fragment before the request goes out. The caller
// is told the id they typed was deleted; the object that was deleted is the one
// they did not type.
//
// Handlers escape every interpolated value with [net/url.PathEscape], which
// turns such a value into a 404 and leaves the decision to the API. This check
// is the other half, and the reason the first half cannot be quietly dropped: it
// runs on every request the SDK makes, so a handler written tomorrow without the
// escape fails loudly here instead of acting on the wrong object.
//
// It deliberately does not reject '?'. `dtctl exec api` passes a caller-written
// path through to the environment, where a query string is legitimate, and
// pkg/resources/api.CanonicalRequestPath vets that spelling on its own terms. An
// unescaped '?' arriving from a resource id is caught by the escape at the call
// site, not here.
func CheckRequestPath(reqPath string) error {
	// An empty URL resolves to the base URL, and resty rejects that itself.
	if reqPath == "" {
		return nil
	}

	// resty takes either a path relative to the base URL or a whole URL --
	// sdk/api/livedebugger builds the latter, because its GraphQL endpoint lives
	// on a different host than the context's environment. Only the path portion
	// can be vetted: the "//" in a scheme is not a dropped path segment.
	bare := reqPath
	if i := strings.Index(bare, "://"); i >= 0 {
		j := strings.IndexByte(bare[i+len("://"):], '/')
		if j < 0 {
			return nil // scheme and host only; there is no path to get wrong
		}
		bare = bare[i+len("://")+j:]
	}

	// A fragment never reaches the server, so the request silently targets
	// whatever precedes it. No dtctl request path contains one deliberately.
	if i := strings.IndexByte(bare, '#'); i >= 0 {
		return fmt.Errorf("invalid request path %q: '#' starts a URL fragment, which is not sent, so "+
			"this request would target %q instead; a resource id containing '#' has to be "+
			"percent-escaped", bare, bare[:i])
	}

	// Everything below concerns the path proper, not the query.
	if i := strings.IndexByte(bare, '?'); i >= 0 {
		bare = bare[:i]
	}

	// An empty, "." or ".." id survives url.PathEscape untouched -- '.' is an
	// unreserved character, and an empty string escapes to itself -- and then
	// resolves to a *shorter* path the moment a server, proxy or router
	// normalizes it: the collection endpoint, or its parent. path.Clean reports
	// exactly that divergence, and there is no escaping that makes such an id
	// name a resource.
	if cleaned := path.Clean(bare); cleaned != bare {
		return fmt.Errorf("invalid request path %q: it normalizes to %q, so the request would not act "+
			"on the resource it names; an empty, \".\" or \"..\" resource id cannot be escaped into a "+
			"safe one", bare, cleaned)
	}

	return nil
}

// GuardRequestPaths installs [CheckRequestPath] on rc, so that no request it
// sends can be retargeted by an unescaped resource id. Both SDK client
// constructors call it; a hand-built resty client should too.
//
// The hook is registered with OnBeforeRequest rather than SetPreRequestHook for
// two reasons: resty keeps only one pre-request hook (the SDK's verbose logging
// owns it), and OnBeforeRequest runs before resty's own parseRequestURL, so
// Request.URL is still the literal path the handler passed -- which is the
// string that has to be vetted, since url.Parse would have already split the
// fragment off into a field nobody looks at.
func GuardRequestPaths(rc *resty.Client) *resty.Client {
	return rc.OnBeforeRequest(func(_ *resty.Client, r *resty.Request) error {
		return CheckRequestPath(r.URL)
	})
}
