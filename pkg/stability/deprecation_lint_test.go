package stability

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// Below 1.0 the version number cannot announce a break, so the deprecation
// record is the whole notice a caller gets. These are the ways it can fail to
// be one.
func TestDeprecationLint(t *testing.T) {
	tests := []struct {
		name    string
		dep     Deprecation
		wantErr string
	}{
		{
			name: "a complete record two minors out",
			dep:  Deprecation{Since: "0.39.0", RemoveIn: "0.41.0", Replacement: "get workflows"},
		},
		{
			name: "further out than the window is fine",
			dep:  Deprecation{Since: "0.39.0", RemoveIn: "1.0.0"},
		},
		{
			name:    "no since-version",
			dep:     Deprecation{RemoveIn: "0.41.0"},
			wantErr: "deprecated without a since-version",
		},
		{
			name:    "no removal version",
			dep:     Deprecation{Since: "0.39.0"},
			wantErr: "deprecated without a remove-in version",
		},
		{
			name:    "removal in the very next release",
			dep:     Deprecation{Since: "0.39.0", RemoveIn: "0.40.0"},
			wantErr: "at least 2 minor releases after the deprecation",
		},
		{
			name:    "removal in the same release that deprecates it",
			dep:     Deprecation{Since: "0.39.0", RemoveIn: "0.39.0"},
			wantErr: "at least 2 minor releases after the deprecation",
		},
		{
			name:    "removal before the deprecation",
			dep:     Deprecation{Since: "0.41.0", RemoveIn: "0.39.0"},
			wantErr: "at least 2 minor releases after the deprecation",
		},
		{
			name:    "a patch bump is not a window",
			dep:     Deprecation{Since: "0.39.0", RemoveIn: "0.39.1"},
			wantErr: "at least 2 minor releases after the deprecation",
		},
		{
			name:    "an unparseable version",
			dep:     Deprecation{Since: "0.39", RemoveIn: "0.41.0"},
			wantErr: "deprecation versions must be X.Y.Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dtctl"}
			child := &cobra.Command{Use: "get", RunE: func(*cobra.Command, []string) error { return nil }}
			MarkStable(child)
			Deprecate(child, tt.dep)
			root.AddCommand(child)

			var found []string
			for _, err := range Lint(root) {
				found = append(found, err.Error())
			}
			joined := strings.Join(found, "\n")

			if tt.wantErr == "" {
				require.Empty(t, found, "expected a clean lint")
				return
			}
			require.Contains(t, joined, tt.wantErr)
			require.Contains(t, joined, "get", "the problem must name the command")
		})
	}
}

// A deprecation records a removal date; it must not read as a demotion. The
// command stays stable and satisfies a stable floor right up to the removal.
func TestDeprecationDoesNotChangeTheTier(t *testing.T) {
	cmd := &cobra.Command{Use: "get"}
	MarkStable(cmd)
	Deprecate(cmd, Deprecation{Since: "0.39.0", RemoveIn: "0.41.0"})

	require.Equal(t, Stable, Of(cmd))
	require.Equal(t, Stable, Effective(cmd))
}
