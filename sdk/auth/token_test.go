package auth

import (
	"encoding/base64"
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
