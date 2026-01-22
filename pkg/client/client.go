package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/version"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// Client is the base HTTP client for dtctl
type Client struct {
	http    *resty.Client
	baseURL string
	token   string
	logger  *logrus.Logger
}

// NewFromConfig creates a new client from config
func NewFromConfig(cfg *config.Config) (*Client, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}

	token, err := cfg.GetToken(ctx.TokenRef)
	if err != nil {
		return nil, err
	}

	return New(ctx.Environment, token)
}

// New creates a new client with base URL and token
func New(baseURL, token string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetAuthScheme("Bearer").
		SetAuthToken(token).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(10*time.Second).
		AddRetryCondition(isRetryable).
		SetTimeout(6*time.Minute). // Allow for long-running Grail queries (up to 5 min)
		SetHeader("User-Agent", fmt.Sprintf("dtctl/%s", version.Version))

	return &Client{
		http:    httpClient,
		baseURL: baseURL,
		token:   token,
		logger:  logger,
	}, nil
}

// isRetryable determines if a request should be retried
func isRetryable(r *resty.Response, err error) bool {
	if err != nil {
		// Don't retry on context deadline exceeded - this is expected for long-running
		// queries that should use polling instead of resubmitting the query
		if errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}

	// Retry on rate limit or server errors
	statusCode := r.StatusCode()
	return statusCode == 429 || statusCode >= 500
}

// HTTP returns the underlying resty client
func (c *Client) HTTP() *resty.Client {
	return c.http
}

// sensitiveHeaders lists headers that should always be redacted in debug output
var sensitiveHeaders = []string{"authorization", "x-api-key", "cookie", "set-cookie"}

// isSensitiveHeader checks if a header name should be redacted
func isSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, h := range sensitiveHeaders {
		if lower == h {
			return true
		}
	}
	return false
}

// SetVerbosity sets the verbosity level for logging
// Level 0: normal (no debug output)
// Level 1: show request/response summary
// Level 2+: show full request/response details (sensitive headers always redacted)
func (c *Client) SetVerbosity(level int) {
	if level <= 0 {
		return
	}
	c.logger.SetLevel(logrus.DebugLevel)
	c.logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
	})

	c.http.SetPreRequestHook(func(client *resty.Client, req *http.Request) error {
		var sb strings.Builder
		sb.WriteString("===> REQUEST <===\n")
		sb.WriteString(fmt.Sprintf("%s %s\n", req.Method, req.URL))
		if level >= 2 {
			sb.WriteString("HEADERS:\n")
			for k, v := range req.Header {
				if isSensitiveHeader(k) {
					sb.WriteString(fmt.Sprintf("    %s: [REDACTED]\n", k))
				} else {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", k, strings.Join(v, ", ")))
				}
			}
		}
		fmt.Print(sb.String())
		return nil
	})

	c.http.OnAfterResponse(func(client *resty.Client, resp *resty.Response) error {
		var sb strings.Builder
		sb.WriteString("===> RESPONSE <===\n")
		sb.WriteString(fmt.Sprintf("STATUS: %d %s\n", resp.StatusCode(), resp.Status()))
		sb.WriteString(fmt.Sprintf("TIME: %s\n", resp.Time()))
		if level >= 2 {
			sb.WriteString("HEADERS:\n")
			for k, v := range resp.Header() {
				if isSensitiveHeader(k) {
					sb.WriteString(fmt.Sprintf("    %s: [REDACTED]\n", k))
				} else {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", k, strings.Join(v, ", ")))
				}
			}
			sb.WriteString(fmt.Sprintf("BODY:\n%s\n", resp.String()))
		}
		fmt.Print(sb.String())
		return nil
	})
}

// SetLogger sets a custom logger
func (c *Client) SetLogger(logger *logrus.Logger) {
	c.logger = logger
}

// Logger returns the client logger
func (c *Client) Logger() *logrus.Logger {
	return c.logger
}

// BaseURL returns the base URL of the Dynatrace environment
func (c *Client) BaseURL() string {
	return c.baseURL
}

// UserInfo contains information about the current user
type UserInfo struct {
	UserName     string `json:"userName"`
	UserID       string `json:"userId"`
	EmailAddress string `json:"emailAddress"`
}

// CurrentUser fetches the current user info from the metadata API.
// Requires scope: app-engine:apps:run
func (c *Client) CurrentUser() (*UserInfo, error) {
	var userInfo UserInfo
	resp, err := c.http.R().
		SetResult(&userInfo).
		Get("/platform/metadata/v1/user")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to fetch user info: %s", resp.Status())
	}
	return &userInfo, nil
}

// CurrentUserID returns the current user's ID.
// First tries the metadata API, falls back to JWT token decoding.
func (c *Client) CurrentUserID() (string, error) {
	// Try metadata API first
	userInfo, err := c.CurrentUser()
	if err == nil && userInfo.UserID != "" {
		return userInfo.UserID, nil
	}

	// Fallback to JWT decoding
	return ExtractUserIDFromToken(c.token)
}

// ExtractUserIDFromToken extracts the user ID (sub claim) from a JWT token.
func ExtractUserIDFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT token format")
	}

	payload := parts[1]
	// Add padding if needed for base64 decoding
	if pad := len(payload) % 4; pad > 0 {
		payload += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	if claims.Sub == "" {
		return "", fmt.Errorf("JWT token does not contain a 'sub' claim")
	}

	return claims.Sub, nil
}
