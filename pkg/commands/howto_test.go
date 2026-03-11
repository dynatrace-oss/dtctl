package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateHowto(t *testing.T) {
	root := newTestRoot()
	listing := Build(root)

	var buf bytes.Buffer
	err := GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()

	// Check required sections
	require.Contains(t, output, "# dtctl Quick Reference")
	require.Contains(t, output, "## Command Model")
	require.Contains(t, output, "verb-noun")
	require.Contains(t, output, "## Verbs")
	require.Contains(t, output, "## Common Workflows")
	require.Contains(t, output, "## Safety Levels")
	require.Contains(t, output, "## Resource Aliases")
	require.Contains(t, output, "## Time Formats")
	require.Contains(t, output, "## Output Formats")
	require.Contains(t, output, "## Recommended Patterns")
	require.Contains(t, output, "## Antipatterns")
}

func TestGenerateHowto_ContainsVerbs(t *testing.T) {
	root := newTestRoot()
	listing := Build(root)

	var buf bytes.Buffer
	err := GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "**get**")
	require.Contains(t, output, "**apply**")
	require.Contains(t, output, "**delete**")
}

func TestGenerateHowto_MutatingLabel(t *testing.T) {
	root := newTestRoot()
	listing := Build(root)

	var buf bytes.Buffer
	err := GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()

	// Find the apply line — should have (mutating) label
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "**apply**") {
			require.Contains(t, line, "(mutating)")
		}
		if strings.Contains(line, "**get**") {
			require.NotContains(t, line, "(mutating)")
		}
	}
}

func TestGenerateHowto_ContainsAliases(t *testing.T) {
	root := newTestRoot()
	listing := Build(root)

	var buf bytes.Buffer
	err := GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "`wf`")
	require.Contains(t, output, "workflows")
}

func TestGenerateHowto_ContainsSafetyLevels(t *testing.T) {
	root := newTestRoot()
	listing := Build(root)

	var buf bytes.Buffer
	err := GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "readonly")
	require.Contains(t, output, "readwrite-mine")
	require.Contains(t, output, "readwrite-all")
	require.Contains(t, output, "dangerously-unrestricted")
}
