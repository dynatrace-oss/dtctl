package cmd

import (
	"reflect"
	"testing"
)

func TestNormalizeScopes(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "comma-separated with spaces",
			input: []string{"scope1, scope2, scope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "space-separated",
			input: []string{"scope1 scope2 scope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "comma-separated no spaces",
			input: []string{"scope1,scope2,scope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "multiple flag values",
			input: []string{"scope1", "scope2", "scope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "mixed comma and space",
			input: []string{"scope1, scope2", "scope3 scope4"},
			want:  []string{"scope1", "scope2", "scope3", "scope4"},
		},
		{
			name:  "newline-separated",
			input: []string{"scope1\nscope2\nscope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "mixed whitespace",
			input: []string{"scope1\t scope2\n scope3"},
			want:  []string{"scope1", "scope2", "scope3"},
		},
		{
			name:  "single scope",
			input: []string{"scope1"},
			want:  []string{"scope1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScopes(tt.input)
			if !reflect.DeepEqual(tt.want, got) {
				t.Errorf("normalizeScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}
