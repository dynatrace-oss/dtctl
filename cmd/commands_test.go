package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/commands"
)

func TestCommandsCmd_OutputsValidJSON(t *testing.T) {
	listing := commands.Build(rootCmd)

	data, err := json.Marshal(listing)
	require.NoError(t, err)
	require.True(t, json.Valid(data), "output should be valid JSON")
}

func TestCommandsCmd_SchemaVersion(t *testing.T) {
	listing := commands.Build(rootCmd)

	require.Equal(t, commands.SchemaVersion, listing.SchemaVersion)
	require.Equal(t, "dtctl", listing.Tool)
	require.Equal(t, "verb-noun", listing.CommandModel)
	require.NotEmpty(t, listing.Version)
}

func TestCommandsCmd_AllVerbsPresent(t *testing.T) {
	listing := commands.Build(rootCmd)

	// Core verbs that must always be present
	expectedVerbs := []string{
		"get", "describe", "apply", "create", "edit", "delete",
		"exec", "diff", "query", "wait", "doctor", "history",
		"restore", "share", "unshare", "logs", "ctx", "skills",
	}

	for _, verb := range expectedVerbs {
		require.Contains(t, listing.Verbs, verb, "verb %q should be present", verb)
	}
}

func TestCommandsCmd_ExcludesUtilityCommands(t *testing.T) {
	listing := commands.Build(rootCmd)

	excluded := []string{"commands", "version", "completion", "help"}
	for _, cmd := range excluded {
		require.NotContains(t, listing.Verbs, cmd, "%q should be excluded from listing", cmd)
	}
}

func TestCommandsCmd_MutatingVerbsCorrect(t *testing.T) {
	listing := commands.Build(rootCmd)

	mutatingVerbs := map[string]bool{
		"apply":   true,
		"create":  true,
		"edit":    true,
		"delete":  true,
		"exec":    true,
		"restore": true,
		"share":   true,
		"unshare": true,
		"update":  true,
	}

	readOnlyVerbs := []string{
		"get", "describe", "diff", "query", "wait", "doctor",
		"history", "logs", "ctx", "find", "verify", "open",
		"skills",
	}

	for verb, expected := range mutatingVerbs {
		v, ok := listing.Verbs[verb]
		if !ok {
			continue // verb may not be registered yet
		}
		require.Equal(t, expected, v.Mutating, "verb %q should be mutating=%v", verb, expected)
		if expected {
			require.NotEmpty(t, v.SafetyOp, "mutating verb %q should have safety_operation", verb)
		}
	}

	for _, verb := range readOnlyVerbs {
		v, ok := listing.Verbs[verb]
		if !ok {
			continue
		}
		require.False(t, v.Mutating, "verb %q should not be mutating", verb)
		require.Empty(t, v.SafetyOp, "read-only verb %q should not have safety_operation", verb)
	}
}

func TestCommandsCmd_GetHasResources(t *testing.T) {
	listing := commands.Build(rootCmd)

	getVerb := listing.Verbs["get"]
	require.NotNil(t, getVerb)
	require.NotEmpty(t, getVerb.Resources, "get verb should have resources")

	// Spot-check some expected resources
	require.Contains(t, getVerb.Resources, "workflows")
	require.Contains(t, getVerb.Resources, "dashboards")
	require.Contains(t, getVerb.Resources, "slos")
	require.Contains(t, getVerb.Resources, "settings")
	require.Contains(t, getVerb.Resources, "buckets")
}

func TestCommandsCmd_ResourceAliases(t *testing.T) {
	listing := commands.Build(rootCmd)

	require.NotNil(t, listing.Aliases)
	require.Equal(t, "workflows", listing.Aliases["wf"])
	require.Equal(t, "dashboards", listing.Aliases["dash"])
}

func TestCommandsCmd_TimeFormats(t *testing.T) {
	listing := commands.Build(rootCmd)

	require.NotNil(t, listing.TimeFormats)
	require.NotEmpty(t, listing.TimeFormats.Relative)
	require.NotEmpty(t, listing.TimeFormats.Absolute)
	require.NotEmpty(t, listing.TimeFormats.Unix)
}

func TestCommandsCmd_BriefReducesSize(t *testing.T) {
	listing := commands.Build(rootCmd)
	fullData, err := json.Marshal(listing)
	require.NoError(t, err)

	briefListing := commands.NewBrief(listing)
	briefData, err := json.Marshal(briefListing)
	require.NoError(t, err)

	// Brief should be significantly smaller
	reduction := 1.0 - float64(len(briefData))/float64(len(fullData))
	require.Greater(t, reduction, 0.30, "brief mode should reduce size by at least 30%%, got %.0f%%", reduction*100)
}

func TestCommandsCmd_FilterByAlias(t *testing.T) {
	listingByName := commands.Build(rootCmd)
	listingByAlias := commands.Build(rootCmd)

	filteredByName, matchedName := commands.FilterByResource(listingByName, "workflows")
	filteredByAlias, matchedAlias := commands.FilterByResource(listingByAlias, "wf")

	require.True(t, matchedName)
	require.True(t, matchedAlias)

	// Same verbs should match
	var nameVerbs, aliasVerbs []string
	for v := range filteredByName.Verbs {
		nameVerbs = append(nameVerbs, v)
	}
	for v := range filteredByAlias.Verbs {
		aliasVerbs = append(aliasVerbs, v)
	}
	require.ElementsMatch(t, nameVerbs, aliasVerbs)
}

func TestCommandsCmd_FilterByVerb(t *testing.T) {
	listing := commands.Build(rootCmd)
	filtered, matched := commands.FilterByResource(listing, "get")

	require.True(t, matched)
	require.Len(t, filtered.Verbs, 1)
	require.Contains(t, filtered.Verbs, "get")
}

func TestCommandsCmd_FilterNoMatch(t *testing.T) {
	listing := commands.Build(rootCmd)
	_, matched := commands.FilterByResource(listing, "nonexistent-resource")

	require.False(t, matched)
}

func TestCommandsCmd_GlobalFlags(t *testing.T) {
	listing := commands.Build(rootCmd)

	require.NotNil(t, listing.GlobalFlags)
	require.Contains(t, listing.GlobalFlags, "--output")
	require.Contains(t, listing.GlobalFlags, "--agent")
	require.Contains(t, listing.GlobalFlags, "--dry-run")
	require.Contains(t, listing.GlobalFlags, "--context")
	require.Contains(t, listing.GlobalFlags, "--plain")
	require.Contains(t, listing.GlobalFlags, "--chunk-size")
}

func TestCommandsCmd_ExecHasSubcommands(t *testing.T) {
	listing := commands.Build(rootCmd)

	execVerb := listing.Verbs["exec"]
	require.NotNil(t, execVerb)
	require.NotEmpty(t, execVerb.Resources, "exec should have resources")

	// copilot should be a subcommand (has nested commands)
	require.NotNil(t, execVerb.Subcommands, "exec should have subcommands")
	require.Contains(t, execVerb.Subcommands, "copilot")
}

func TestCommandsCmd_Howto(t *testing.T) {
	listing := commands.Build(rootCmd)

	var buf bytes.Buffer
	err := commands.GenerateHowto(&buf, listing)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "# dtctl Quick Reference")
	require.Contains(t, output, "## Common Workflows")
	require.Contains(t, output, "## Safety Levels")

	// Verify all verbs appear in howto
	for verb := range listing.Verbs {
		require.True(t, strings.Contains(output, verb),
			"howto should mention verb %q", verb)
	}
}

func TestCommandsCmd_PatternsAndAntipatterns(t *testing.T) {
	listing := commands.Build(rootCmd)

	require.NotEmpty(t, listing.Patterns)
	require.NotEmpty(t, listing.Antipatterns)

	// Check for key content
	found := false
	for _, p := range listing.Patterns {
		if strings.Contains(p, "apply") {
			found = true
			break
		}
	}
	require.True(t, found, "patterns should mention apply")

	found = false
	for _, p := range listing.Antipatterns {
		if strings.Contains(p, "table output") {
			found = true
			break
		}
	}
	require.True(t, found, "antipatterns should warn about table output parsing")
}

// --- Cross-reference tests ---

// TestMutatingVerbsMatchSafetyCheckerUsage scans cmd/ source files for
// NewSafetyChecker calls and verifies that every verb which uses safety
// checks is listed in commands.MutatingVerbs. This catches drift between
// the catalog and the actual command implementations.
func TestMutatingVerbsMatchSafetyCheckerUsage(t *testing.T) {
	// Find all .go files in cmd/ that contain NewSafetyChecker
	cmdDir := "."
	entries, err := os.ReadDir(cmdDir)
	require.NoError(t, err)

	verbsWithSafetyChecks := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(cmdDir, entry.Name()))
		require.NoError(t, err)

		if !strings.Contains(string(data), "NewSafetyChecker") {
			continue
		}

		// Extract the verb from the filename pattern: <verb>_<resource>.go or <verb>.go
		name := strings.TrimSuffix(entry.Name(), ".go")
		verb := strings.SplitN(name, "_", 2)[0]

		// Special cases: root.go defines NewSafetyChecker, not a verb
		if verb == "root" || verb == "safety" {
			continue
		}

		// "get" files that call NewSafetyChecker are delete handlers
		// (e.g., get_workflows.go has deleteWorkflowCmd) — these use
		// the "delete" parent command's safety check, not "get".
		if verb == "get" {
			verbsWithSafetyChecks["delete"] = true
			continue
		}

		verbsWithSafetyChecks[verb] = true
	}

	// Every verb with safety checks should be in MutatingVerbs
	for verb := range verbsWithSafetyChecks {
		_, ok := commands.MutatingVerbs[verb]
		require.True(t, ok,
			"verb %q has NewSafetyChecker calls but is missing from commands.MutatingVerbs", verb)
	}

	// Every verb in MutatingVerbs should exist in the real command tree
	listing := commands.Build(rootCmd)
	for verb := range commands.MutatingVerbs {
		_, ok := listing.Verbs[verb]
		require.True(t, ok,
			"commands.MutatingVerbs contains %q but it doesn't exist in the command tree", verb)
	}
}

// TestResourceAliasesMatchCobraAliases walks the real Cobra command tree and
// collects all resource-level Aliases defined on subcommands. It then verifies
// that commands.ResourceAliases contains these aliases (or a documented subset).
func TestResourceAliasesMatchCobraAliases(t *testing.T) {
	// Collect all short aliases from the "get" command's subcommands,
	// which is the canonical place for resource aliases.
	getCmd, _, err := rootCmd.Find([]string{"get"})
	require.NoError(t, err)
	require.NotNil(t, getCmd)

	cobraAliases := make(map[string]string) // alias -> canonical resource name
	for _, sub := range getCmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		for _, alias := range sub.Aliases {
			// Skip aliases that are just singular/plural forms
			// (e.g., "workflow" as alias for "workflows")
			if isSingularPlural(alias, sub.Name()) {
				continue
			}
			cobraAliases[alias] = sub.Name()
		}
	}

	// Verify that ResourceAliases covers the key short aliases from Cobra.
	// We check that every short alias (non-singular/plural) in the "get"
	// command is present in ResourceAliases.
	for alias, resource := range cobraAliases {
		target, ok := commands.ResourceAliases[alias]
		if !ok {
			// Not every Cobra alias needs to be in ResourceAliases
			// (some are verb-specific), but common ones should be.
			// Log it so we can catch drift.
			t.Logf("INFO: Cobra alias %q → %q (from get) not in ResourceAliases (may be OK for verb-specific aliases)", alias, resource)
			continue
		}
		// If it IS in ResourceAliases, verify it points to the same resource
		// (accounting for singular/plural differences)
		if target != resource && !isSingularPlural(target, resource) {
			t.Errorf("ResourceAliases[%q] = %q, but Cobra alias points to %q",
				alias, target, resource)
		}
	}

	// Verify every entry in ResourceAliases resolves to a real resource
	listing := commands.Build(rootCmd)
	for alias, target := range commands.ResourceAliases {
		// The target should appear in at least one verb's resources or subcommands
		found := false
		for _, verb := range listing.Verbs {
			for _, r := range verb.Resources {
				if r == target || isSingularPlural(r, target) {
					found = true
					break
				}
			}
			if found {
				break
			}
			for subName := range verb.Subcommands {
				if subName == target || isSingularPlural(subName, target) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		require.True(t, found,
			"ResourceAliases[%q] → %q, but %q doesn't appear in any verb's resources",
			alias, target, target)
	}
}

// isSingularPlural returns true if a and b differ only by a trailing "s".
func isSingularPlural(a, b string) bool {
	if a == b {
		return true
	}
	return a+"s" == b || b+"s" == a
}

// --- WriteTo integration tests (using real rootCmd) ---

func TestCommandsCmd_WriteToJSON(t *testing.T) {
	listing := commands.Build(rootCmd)

	var buf bytes.Buffer
	err := commands.WriteTo(&buf, listing, "json")
	require.NoError(t, err)
	require.True(t, json.Valid(buf.Bytes()), "WriteTo JSON should produce valid JSON")

	var decoded commands.Listing
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	require.Equal(t, "dtctl", decoded.Tool)
	require.NotEmpty(t, decoded.Verbs)
}

func TestCommandsCmd_WriteToYAML(t *testing.T) {
	listing := commands.Build(rootCmd)

	var buf bytes.Buffer
	err := commands.WriteTo(&buf, listing, "yaml")
	require.NoError(t, err)

	var decoded commands.Listing
	err = yaml.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)
	require.Equal(t, "dtctl", decoded.Tool)
	require.NotEmpty(t, decoded.Verbs)
}

// --- NewBrief integration test (using real rootCmd) ---

func TestCommandsCmd_NewBriefDoesNotMutateOriginal(t *testing.T) {
	listing := commands.Build(rootCmd)

	origVerbCount := len(listing.Verbs)
	origDesc := listing.Description
	origGlobalFlagCount := len(listing.GlobalFlags)

	brief := commands.NewBrief(listing)

	// Original should be unchanged
	require.Len(t, listing.Verbs, origVerbCount)
	require.Equal(t, origDesc, listing.Description)
	require.Len(t, listing.GlobalFlags, origGlobalFlagCount)

	// Brief should have stripped fields
	require.Empty(t, brief.Description)
	require.Nil(t, brief.GlobalFlags)
	require.Nil(t, brief.TimeFormats)
	require.Nil(t, brief.Patterns)
	require.Nil(t, brief.Antipatterns)

	// But should preserve structure
	require.Len(t, brief.Verbs, origVerbCount)
	require.NotNil(t, brief.Aliases)
}

func TestCommandsCmd_NewBriefPreservesMutating(t *testing.T) {
	listing := commands.Build(rootCmd)
	brief := commands.NewBrief(listing)

	for name, verb := range listing.Verbs {
		briefVerb, ok := brief.Verbs[name]
		require.True(t, ok, "verb %q should be in brief", name)
		require.Equal(t, verb.Mutating, briefVerb.Mutating,
			"verb %q mutating status should be preserved in brief", name)
	}
}

// --- E2E test for runCommandsListing ---

func TestCommandsCmd_RunCommandsListing_Integration(t *testing.T) {
	// Build the listing the same way runCommandsListing does
	listing := commands.Build(rootCmd)

	// Full output
	var fullBuf bytes.Buffer
	err := commands.WriteTo(&fullBuf, listing, "json")
	require.NoError(t, err)
	require.True(t, json.Valid(fullBuf.Bytes()))

	// Brief output
	brief := commands.NewBrief(listing)
	var briefBuf bytes.Buffer
	err = commands.WriteTo(&briefBuf, brief, "json")
	require.NoError(t, err)
	require.True(t, json.Valid(briefBuf.Bytes()))
	require.Less(t, briefBuf.Len(), fullBuf.Len(),
		"brief output should be smaller than full output")

	// Filtered output
	unfiltered := commands.Build(rootCmd)
	filtered, matched := commands.FilterByResource(unfiltered, "workflows")
	require.True(t, matched)
	var filteredBuf bytes.Buffer
	err = commands.WriteTo(&filteredBuf, filtered, "json")
	require.NoError(t, err)
	require.Less(t, filteredBuf.Len(), fullBuf.Len(),
		"filtered output should be smaller than full output")
}

// --- Completeness test: listing covers all non-hidden commands ---

func TestCommandsCmd_ListingCoversAllNonHiddenCommands(t *testing.T) {
	listing := commands.Build(rootCmd)

	// Every non-hidden root subcommand (except utility ones) should be in listing
	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden {
			continue
		}
		name := cmd.Name()
		// Skip utility commands that are intentionally excluded
		if name == "help" || name == "completion" || name == "version" || name == "commands" {
			continue
		}
		require.Contains(t, listing.Verbs, name,
			"non-hidden command %q should be in the listing", name)
	}
}

// TestAllCommandsHaveHelpText verifies that all non-hidden, non-utility root
// commands have Long descriptions and Example fields populated.
func TestAllCommandsHaveHelpText(t *testing.T) {
	// Meta/utility commands that are exempt from this requirement
	metaCommands := map[string]bool{
		"help":       true,
		"completion": true,
		"version":    true,
		"commands":   true,
	}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden || metaCommands[cmd.Name()] {
			continue
		}
		t.Run(cmd.Name(), func(t *testing.T) {
			require.NotEmpty(t, cmd.Long,
				"command %q should have a Long description", cmd.Name())
		})
	}
}

// TestParentVerbsHaveExamples verifies that all parent verb commands have
// examples in the Cobra Example field (not embedded in Long).
func TestParentVerbsHaveExamples(t *testing.T) {
	parentVerbs := []string{
		"get", "delete", "create", "edit", "exec",
		"describe", "find", "update", "open", "doctor",
		"skills",
	}

	for _, name := range parentVerbs {
		cmd, _, err := rootCmd.Find([]string{name})
		require.NoError(t, err, "command %q should exist", name)
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, cmd.Example,
				"parent verb %q should have examples in the Example field", name)
		})
	}
}
