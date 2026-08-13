package apispec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopeVerb(t *testing.T) {
	cases := map[string]string{
		"document:documents:read":             "read",
		"document:trash.documents:restore":    "restore",
		"storage:bucket-definitions:truncate": "truncate",
		"iam:service-users:use":               "use",
		// Not in <service>:<resource>:<verb> form: no verb to extract.
		"openid":         "",
		"offline_access": "",
		"trailing:":      "",
		"":               "",
	}

	for scope, want := range cases {
		t.Run(scope, func(t *testing.T) {
			require.Equal(t, want, ScopeVerb(scope))
		})
	}
}

func TestAccessForScopes(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		want   Access
		why    string
	}{
		{
			name: "no declaration",
			want: AccessUnknown,
			why:  "an empty set means the document said nothing, not that nothing is needed",
		},
		{
			name:   "read",
			scopes: []string{"widget:widgets:read"},
			want:   AccessRead,
		},
		{
			name:   "use reads without changing",
			scopes: []string{"iam:service-users:use"},
			want:   AccessRead,
		},
		{
			name:   "write",
			scopes: []string{"widget:widgets:write"},
			want:   AccessWrite,
		},
		{
			name:   "restore changes state without creating or destroying",
			scopes: []string{"document:trash.documents:restore"},
			want:   AccessWrite,
		},
		{
			name:   "delete",
			scopes: []string{"widget:widgets:delete"},
			want:   AccessDelete,
		},
		{
			name:   "truncate destroys contents",
			scopes: []string{"storage:bucket-definitions:truncate"},
			want:   AccessDelete,
			why:    "truncation is data loss, so it classifies with deletion",
		},
		{
			name:   "the strongest verb in a set decides",
			scopes: []string{"widget:widgets:read", "widget:widgets:delete"},
			want:   AccessDelete,
			why:    "a caller holding both can exercise both, so the gate must cover the stronger",
		},
		{
			name:   "an unrecognised verb invalidates the whole set",
			scopes: []string{"widget:widgets:read", "widget:widgets:teleport"},
			want:   AccessUnknown,
			why:    "one unclassifiable scope makes any conclusion from the others unsound",
		},
		{
			name:   "admin is deliberately unclassified",
			scopes: []string{"storage:filter-segments:admin"},
			want:   AccessUnknown,
			why:    "admin is unbounded; it must not be read as a mere write",
		},
		{
			name:   "execute is deliberately unclassified",
			scopes: []string{"davis:analyzers:execute"},
			want:   AccessUnknown,
			why:    "execution says nothing about whether the executed thing mutates",
		},
		{
			name:   "a scope that is not in verb form",
			scopes: []string{"openid"},
			want:   AccessUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, AccessForScopes(tc.scopes), tc.why)
		})
	}
}

// TestMethodFloorsAreOneSided pins that the method contributes a bound and never
// a classification. POST is the case that matters: measured across 64 specified
// operations, 12 POSTs needed only a `:read` scope and one needed `:delete`, so
// inferring from POST is wrong in both directions — including the direction that
// would permit a delete as a create.
func TestMethodFloorsAreOneSided(t *testing.T) {
	cases := []struct {
		method       string
		read         bool
		alwaysMutate bool
	}{
		{"GET", true, false},
		{"get", true, false},
		{"HEAD", true, false},
		{"PUT", false, true},
		{"PATCH", false, true},
		{"DELETE", false, true},
		{"POST", false, false},
		{"OPTIONS", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			require.Equal(t, tc.read, MethodIsRead(tc.method))
			require.Equal(t, tc.alwaysMutate, MethodIsAlwaysMutating(tc.method))
		})
	}

	require.False(t, MethodIsRead("POST"))
	require.False(t, MethodIsAlwaysMutating("POST"),
		"POST must carry no information in either direction")
}
