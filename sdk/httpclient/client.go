package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/dynatrace-oss/dtctl/sdk/auth"
)

// Client is an HTTP client for Dynatrace APIs.
type Client struct {
	http    *resty.Client
	baseURL string
	token   string
	logger  Logger
}

// Option configures a Client.
type Option func(*Client)

// WithToken sets the authentication token. The auth scheme (Bearer vs Api-Token)
// is determined automatically from the token prefix.
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = token
	}
}

// WithRetry configures retry behaviour.
func WithRetry(maxRetries int, waitTime, maxWaitTime time.Duration) Option {
	return func(c *Client) {
		c.http.SetRetryCount(maxRetries)
		c.http.SetRetryWaitTime(waitTime)
		c.http.SetRetryMaxWaitTime(maxWaitTime)
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.http.SetTimeout(d)
	}
}

// WithUserAgent sets the User-Agent header prefix. AI agent detection
// suffix is appended automatically.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		fullUA := ua
		if aiSuffix := agentmode.UserAgentSuffix(); aiSuffix != "" {
			fullUA += aiSuffix
		}
		c.http.SetHeader("User-Agent", fullUA)
	}
}

// WithLogger sets the logger for debug output.
func WithLogger(l Logger) Option {
	return func(c *Client) {
		c.logger = l
	}
}

// WithHTTPProxy sets an HTTP proxy URL.
func WithHTTPProxy(proxyURL string) Option {
	return func(c *Client) {
		c.http.SetProxy(proxyURL)
	}
}

// WithTransport sets a custom http.RoundTripper, useful for injecting
// OpenTelemetry or other middleware.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) {
		c.http.SetTransport(rt)
	}
}

// noopRestyLogger discards all resty-internal log output.
type noopRestyLogger struct{}

func (noopRestyLogger) Errorf(string, ...interface{}) {}
func (noopRestyLogger) Warnf(string, ...interface{})  {}
func (noopRestyLogger) Debugf(string, ...interface{}) {}

// New creates a new Dynatrace HTTP client.
//
// The baseURL is required (e.g. "https://abc.apps.dynatrace.com").
// At minimum, provide WithToken to authenticate requests.
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	c := &Client{
		http:    resty.New(),
		baseURL: baseURL,
		logger:  noopLogger{},
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Configure resty
	c.http.
		SetLogger(&noopRestyLogger{}).
		SetBaseURL(baseURL).
		SetHeader("Accept-Encoding", "gzip")

	// Set auth if token provided
	if c.token != "" {
		c.http.SetAuthScheme(auth.AuthScheme(c.token))
		c.http.SetAuthToken(c.token)
	}

	// Set defaults if not overridden
	if c.http.RetryCount == 0 {
		c.http.SetRetryCount(3)
		c.http.SetRetryWaitTime(1 * time.Second)
		c.http.SetRetryMaxWaitTime(10 * time.Second)
	}
	c.http.AddRetryCondition(isRetryable)

	if c.http.GetClient().Timeout == 0 {
		c.http.SetTimeout(6 * time.Minute)
	}

	return c, nil
}

// isRetryable determines if a request should be retried.
func isRetryable(r *resty.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	statusCode := r.StatusCode()
	return statusCode == 429 || statusCode >= 500
}

// HTTP returns the underlying resty client for advanced use cases.
// Prefer the typed methods (Do, GetJSON, etc.) when possible.
func (c *Client) HTTP() *resty.Client {
	return c.http
}

// SetToken updates the authentication token.
func (c *Client) SetToken(token string) {
	c.token = token
	c.http.SetAuthScheme(auth.AuthScheme(token))
	c.http.SetAuthToken(token)
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// sensitiveHeaders lists headers that should always be redacted in debug output.
var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"x-api-key":     true,
	"cookie":        true,
	"set-cookie":    true,
}

// EnableVerboseLogging enables request/response debug logging.
// Level 1: summary. Level 2+: full headers and body (sensitive headers redacted).
func (c *Client) EnableVerboseLogging(level int, w io.Writer) {
	if level <= 0 || w == nil {
		return
	}

	c.http.SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
		var sb strings.Builder
		sb.WriteString("===> REQUEST <===\n")
		sb.WriteString(fmt.Sprintf("%s %s\n", req.Method, req.URL))
		if level >= 2 {
			sb.WriteString("HEADERS:\n")
			for k, v := range req.Header {
				if sensitiveHeaders[strings.ToLower(k)] {
					sb.WriteString(fmt.Sprintf("    %s: [REDACTED]\n", k))
				} else {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", k, strings.Join(v, ", ")))
				}
			}
			if bodyText := readRequestBodyForDebug(req); bodyText != "" {
				sb.WriteString(fmt.Sprintf("BODY:\n%s\n", bodyText))
			}
		}
		fmt.Fprint(w, sb.String())
		return nil
	})

	c.http.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		var sb strings.Builder
		sb.WriteString("===> RESPONSE <===\n")
		sb.WriteString(fmt.Sprintf("STATUS: %d %s\n", resp.StatusCode(), resp.Status()))
		sb.WriteString(fmt.Sprintf("TIME: %s\n", resp.Time()))
		if level >= 2 {
			sb.WriteString("HEADERS:\n")
			for k, v := range resp.Header() {
				if sensitiveHeaders[strings.ToLower(k)] {
					sb.WriteString(fmt.Sprintf("    %s: [REDACTED]\n", k))
				} else {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", k, strings.Join(v, ", ")))
				}
			}
			sb.WriteString(fmt.Sprintf("BODY:\n%s\n", resp.String()))
		}
		fmt.Fprint(w, sb.String())
		return nil
	})
}

func readRequestBodyForDebug(req *http.Request) string {
	defer func() { _ = recover() }()

	if req == nil {
		return ""
	}

	if req.GetBody != nil {
		clone, err := req.GetBody()
		if err == nil && clone != nil {
			defer clone.Close()
			body, readErr := io.ReadAll(clone)
			if readErr == nil && len(body) > 0 {
				return string(body)
			}
		}
	}

	if req.Body == nil {
		return ""
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return ""
	}
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	if len(body) == 0 {
		return ""
	}

	return string(body)
}
