package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// TokenType represents the classification of a Dynatrace token.
type TokenType int

const (
	// TokenTypeBearer is an OAuth or JWT token, sent as "Bearer <token>".
	TokenTypeBearer TokenType = iota
	// TokenTypeAPIToken is a Dynatrace API token (dt0c01.* prefix), sent as "Api-Token <token>".
	TokenTypeAPIToken
	// TokenTypePlatform is a Dynatrace platform token (dt0s16.* prefix), sent as "Bearer <token>".
	TokenTypePlatform
)

const (
	// APITokenPrefix is the prefix for Dynatrace API tokens.
	APITokenPrefix = "dt0c01."
	// PlatformTokenPrefix is the prefix for Dynatrace platform tokens.
	PlatformTokenPrefix = "dt0s16."
)

// Classify determines the type of a Dynatrace token based on its prefix.
func Classify(token string) TokenType {
	switch {
	case strings.HasPrefix(token, APITokenPrefix):
		return TokenTypeAPIToken
	case strings.HasPrefix(token, PlatformTokenPrefix):
		return TokenTypePlatform
	default:
		return TokenTypeBearer
	}
}

// AuthScheme returns the HTTP Authorization scheme for the given token.
// API tokens use "Api-Token"; all others use "Bearer".
func AuthScheme(token string) string {
	if Classify(token) == TokenTypeAPIToken {
		return "Api-Token"
	}
	return "Bearer"
}

// AuthHeader returns the full Authorization header value for a token.
func AuthHeader(token string) string {
	return AuthScheme(token) + " " + token
}

// IsAPIToken reports whether the token is a Dynatrace API token (dt0c01.* prefix).
func IsAPIToken(token string) bool {
	return Classify(token) == TokenTypeAPIToken
}

// IsPlatformToken reports whether the token is a Dynatrace platform token (dt0s16.* prefix).
func IsPlatformToken(token string) bool {
	return Classify(token) == TokenTypePlatform
}

// ExtractJWTSubject extracts the "sub" claim from a JWT token's payload.
// Returns an error if the token is not a valid JWT or has no "sub" claim.
// Platform tokens (dt0s16.*) are rejected since they are not JWTs.
func ExtractJWTSubject(token string) (string, error) {
	if IsPlatformToken(token) {
		return "", fmt.Errorf("cannot extract subject: token is a Dynatrace platform token, not a JWT")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT token format")
	}

	payload := parts[1]
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

// ExtractJWTScopes extracts the granted scopes from a JWT access token. It reads
// the "scope" claim (a space-delimited string) or, failing that, the "scp"
// claim (either a space-delimited string or an array of strings). It returns
// nil when the token is not a decodable JWT, is a Dynatrace platform token
// (dt0s16.*, not a JWT), or carries no recognizable scope claim. Callers use
// this as a best-effort fallback for displaying scopes when the scope string
// was dropped from compact keyring storage.
func ExtractJWTScopes(token string) []string {
	if token == "" || IsPlatformToken(token) {
		return nil
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}

	payload := parts[1]
	if pad := len(payload) % 4; pad > 0 {
		payload += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}

	var claims struct {
		Scope string          `json:"scope"`
		Scp   json.RawMessage `json:"scp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil
	}

	if scopes := strings.Fields(claims.Scope); len(scopes) > 0 {
		return scopes
	}

	// "scp" may be encoded as a JSON array of strings or as a single
	// space-delimited string, depending on the issuer.
	if len(claims.Scp) > 0 {
		var arr []string
		if err := json.Unmarshal(claims.Scp, &arr); err == nil && len(arr) > 0 {
			return arr
		}
		var str string
		if err := json.Unmarshal(claims.Scp, &str); err == nil {
			if scopes := strings.Fields(str); len(scopes) > 0 {
				return scopes
			}
		}
	}

	return nil
}
