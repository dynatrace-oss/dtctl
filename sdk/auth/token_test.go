package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		token string
		want  TokenType
	}{
		{"dt0c01.SOME_TOKEN_VALUE", TokenTypeAPIToken},
		{"dt0s16.SOME_PLATFORM_TOKEN", TokenTypePlatform},
		{"eyJhbGciOiJSUzI1NiJ9.payload.sig", TokenTypeBearer},
		{"some-other-token", TokenTypeBearer},
		{"", TokenTypeBearer},
	}

	for _, tt := range tests {
		got := Classify(tt.token)
		if got != tt.want {
			t.Errorf("Classify(%q) = %d, want %d", tt.token, got, tt.want)
		}
	}
}

func TestAuthScheme(t *testing.T) {
	if got := AuthScheme("dt0c01.test"); got != "Api-Token" {
		t.Errorf("AuthScheme(API token) = %q, want Api-Token", got)
	}
	if got := AuthScheme("dt0s16.test"); got != "Bearer" {
		t.Errorf("AuthScheme(platform token) = %q, want Bearer", got)
	}
	if got := AuthScheme("eyJhbGci.payload.sig"); got != "Bearer" {
		t.Errorf("AuthScheme(JWT) = %q, want Bearer", got)
	}
}

func TestAuthHeader(t *testing.T) {
	if got := AuthHeader("dt0c01.xyz"); got != "Api-Token dt0c01.xyz" {
		t.Errorf("AuthHeader = %q", got)
	}
	if got := AuthHeader("some-bearer"); got != "Bearer some-bearer" {
		t.Errorf("AuthHeader = %q", got)
	}
}

func TestIsAPIToken(t *testing.T) {
	if !IsAPIToken("dt0c01.test") {
		t.Error("expected true for dt0c01 prefix")
	}
	if IsAPIToken("dt0s16.test") {
		t.Error("expected false for dt0s16 prefix")
	}
}

func TestIsPlatformToken(t *testing.T) {
	if !IsPlatformToken("dt0s16.test") {
		t.Error("expected true for dt0s16 prefix")
	}
	if IsPlatformToken("dt0c01.test") {
		t.Error("expected false for dt0c01 prefix")
	}
}

func TestExtractJWTSubject(t *testing.T) {
	// Build a valid JWT with sub claim
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user@example.invalid"}`))
	jwt := "eyJhbGciOiJSUzI1NiJ9." + payload + ".signature"

	sub, err := ExtractJWTSubject(jwt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub != "user@example.invalid" {
		t.Errorf("sub = %q, want user@example.invalid", sub)
	}

	// Platform token should fail
	_, err = ExtractJWTSubject("dt0s16.not-a-jwt")
	if err == nil {
		t.Error("expected error for platform token")
	}

	// Invalid JWT format
	_, err = ExtractJWTSubject("not-a-jwt")
	if err == nil {
		t.Error("expected error for invalid JWT")
	}
}

// makeJWT builds a minimal unsigned JWT (header.payload.signature) whose
// payload is the JSON encoding of claims. Only the payload is decoded by
// ExtractJWTScopes, so header and signature are placeholders.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}

func TestExtractJWTScopes(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  []string
	}{
		{
			name:  "scope claim, space-delimited",
			token: makeJWT(t, map[string]any{"scope": "storage:logs:read storage:events:read"}),
			want:  []string{"storage:logs:read", "storage:events:read"},
		},
		{
			name:  "scp claim as array",
			token: makeJWT(t, map[string]any{"scp": []string{"openid", "offline_access"}}),
			want:  []string{"openid", "offline_access"},
		},
		{
			name:  "scp claim as string",
			token: makeJWT(t, map[string]any{"scp": "openid offline_access"}),
			want:  []string{"openid", "offline_access"},
		},
		{
			name:  "scope preferred over scp",
			token: makeJWT(t, map[string]any{"scope": "a b", "scp": []string{"c"}}),
			want:  []string{"a", "b"},
		},
		{name: "no scope claim", token: makeJWT(t, map[string]any{"sub": "user-1"}), want: nil},
		{name: "empty token", token: "", want: nil},
		{name: "not a jwt", token: "not-a-jwt", want: nil},
		{name: "platform token is not a jwt", token: "dt0s16.ABC.DEF", want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractJWTScopes(tc.token)
			if len(got) != len(tc.want) {
				t.Fatalf("ExtractJWTScopes() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("scope[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
