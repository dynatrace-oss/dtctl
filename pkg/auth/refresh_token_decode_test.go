package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	sig := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return strings.Join([]string{header, payload, sig}, ".")
}

func TestDecodeRefreshTokenExpiry(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour).Unix()

	tests := []struct {
		name    string
		token   string
		wantOk  bool
		wantExp int64
	}{
		{
			name:    "valid JWT with exp",
			token:   makeJWT(t, map[string]any{"exp": future, "sub": "user-1"}),
			wantOk:  true,
			wantExp: future,
		},
		{
			name:   "JWT without exp",
			token:  makeJWT(t, map[string]any{"sub": "user-1"}),
			wantOk: false,
		},
		{
			name:   "empty token",
			token:  "",
			wantOk: false,
		},
		{
			name:   "opaque string (not JWT)",
			token:  "this-is-an-opaque-refresh-token",
			wantOk: false,
		},
		{
			name:   "malformed JWT (two parts)",
			token:  "aaa.bbb",
			wantOk: false,
		},
		{
			name:   "malformed JWT (bad base64)",
			token:  "aaa.!!!not-base64!!!.ccc",
			wantOk: false,
		},
		{
			name:   "malformed JWT (bad json)",
			token:  "aaa." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".ccc",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DecodeRefreshTokenExpiry(tt.token)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got.Unix() != tt.wantExp {
				t.Errorf("exp = %d, want %d", got.Unix(), tt.wantExp)
			}
			if !ok && !got.IsZero() {
				t.Errorf("expected zero time when ok=false, got %v", got)
			}
		})
	}
}
