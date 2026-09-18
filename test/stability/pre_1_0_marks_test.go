package stability_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/cmd"
)

// pre10Demotion is one flag the pre-1.0 audit took off the stable contract, and
// the accepted breaking change that forced it.
type pre10Demotion struct {
	command string
	flag    string
	driver  string
}

// pre10Demotions is the audit's result, written out so it can be checked rather
// than trusted.
//
// stability.MarkFlag is a silent no-op when the named flag does not exist, so a
// rename in cmd/ would drop a mark and quietly re-promise a flag that 1.0
// removes. That failure is invisible in review and invisible at runtime, which
// is exactly why this list is a test: the mark has to be provably in effect,
// not merely written down.
var pre10Demotions = []pre10Demotion{
	// breaking-changes/cloud-flags-kebab-case.md — every cloud flag becomes
	// kebab-case and the spelling aliases (including the `aplicationID` typo)
	// are removed.
	{"create aws connection", "--roleArn", "cloud-flags-kebab-case"},
	{"update aws connection", "--roleArn", "cloud-flags-kebab-case"},
	{"enable aws monitoring", "--roleArn", "cloud-flags-kebab-case"},
	{"create aws monitoring", "--featureSets", "cloud-flags-kebab-case"},
	{"update aws monitoring", "--featureSets", "cloud-flags-kebab-case"},

	{"create azure connection", "--directoryId", "cloud-flags-kebab-case"},
	{"create azure connection", "--applicationId", "cloud-flags-kebab-case"},
	{"create azure connection", "--clientSecret", "cloud-flags-kebab-case"},
	{"update azure connection", "--directoryId", "cloud-flags-kebab-case"},
	{"update azure connection", "--directoryID", "cloud-flags-kebab-case"},
	{"update azure connection", "--applicationId", "cloud-flags-kebab-case"},
	{"update azure connection", "--applicationID", "cloud-flags-kebab-case"},
	{"update azure connection", "--aplicationID", "cloud-flags-kebab-case"},
	{"update azure connection", "--clientSecret", "cloud-flags-kebab-case"},
	{"enable azure monitoring", "--directoryId", "cloud-flags-kebab-case"},
	{"enable azure monitoring", "--applicationId", "cloud-flags-kebab-case"},
	{"create azure monitoring", "--locationFiltering", "cloud-flags-kebab-case"},
	{"create azure monitoring", "--featureSets", "cloud-flags-kebab-case"},
	{"create azure monitoring", "--featuresets", "cloud-flags-kebab-case"},
	{"update azure monitoring", "--locationFiltering", "cloud-flags-kebab-case"},
	{"update azure monitoring", "--featureSets", "cloud-flags-kebab-case"},
	{"update azure monitoring", "--featuresets", "cloud-flags-kebab-case"},

	{"create gcp connection", "--serviceAccountId", "cloud-flags-kebab-case"},
	{"create gcp connection", "--serviceaccountid", "cloud-flags-kebab-case"},
	{"update gcp connection", "--serviceAccountId", "cloud-flags-kebab-case"},
	{"update gcp connection", "--serviceaccountid", "cloud-flags-kebab-case"},
	{"enable gcp monitoring", "--serviceAccountId", "cloud-flags-kebab-case"},
	{"create gcp monitoring", "--locationFiltering", "cloud-flags-kebab-case"},
	{"create gcp monitoring", "--featureSets", "cloud-flags-kebab-case"},
	{"create gcp monitoring", "--featuresets", "cloud-flags-kebab-case"},
	{"update gcp monitoring", "--locationFiltering", "cloud-flags-kebab-case"},
	{"update gcp monitoring", "--featureSets", "cloud-flags-kebab-case"},
	{"update gcp monitoring", "--featuresets", "cloud-flags-kebab-case"},

	// breaking-changes/short-flag-f.md — `-f` means `--file` on every command,
	// so these lose their short form or the whole flag.
	{"restore workflow", "--force", "short-flag-f"},
	{"restore dashboard", "--force", "short-flag-f"},
	{"restore notebook", "--force", "short-flag-f"},
	{"restore document", "--force", "short-flag-f"},
	{"logs workflow-execution", "--follow", "short-flag-f"},

	// breaking-changes/timeout-duration.md — every --timeout takes a Go
	// duration and a bare integer becomes an error.
	{"exec analyzer", "--timeout", "timeout-duration"},
	{"exec slo", "--timeout", "timeout-duration"},

	// breaking-changes/unshadow-global-flags.md — a local flag may not hide a
	// global one, so these are renamed or removed.
	{"diff", "--context", "unshadow-global-flags"},
	{"diff", "--output", "unshadow-global-flags"},
	{"exec copilot", "--context", "unshadow-global-flags"},
	{"verify openpipeline-matcher", "--context", "unshadow-global-flags"},
	{"wait query", "--verbose", "unshadow-global-flags"},
}

// TestPre10DemotionsAreInEffect asserts every audited flag really carries the
// weaker contract on the live tree.
func TestPre10DemotionsAreInEffect(t *testing.T) {
	levels := manifestFlagLevels(t, cmd.StabilityManifest())

	for _, d := range pre10Demotions {
		key := d.command + " " + d.flag
		got, ok := levels[key]
		if !ok {
			t.Errorf("%s: not on the command tree; if %s implemented the rename, drop this entry and its stability.MarkFlag call",
				key, d.driver)
			continue
		}
		if got != "experimental" {
			t.Errorf("%s: is %q, want %q (%s changes it in 1.0, so stable would be a promise dtctl cannot keep)",
				key, got, "experimental", d.driver)
		}
	}
}

// TestPre10StableExclusionsStayStable pins the flags the same documents
// deliberately leave alone. The documents say "no change" for these, and a
// blanket demotion of everything that merely looks similar would cost users the
// stable contract for nothing.
func TestPre10StableExclusionsStayStable(t *testing.T) {
	levels := manifestFlagLevels(t, cmd.StabilityManifest())

	// The reason each one is spared, per the driving document.
	exclusions := map[string]string{
		// short-flag-f.md: not a confirmation flag, no short form.
		"restore trash --force": "not a confirmation flag",
		// timeout-duration.md: already a duration.
		"exec workflow --timeout": "already a duration",
		"wait query --timeout":    "already a duration",
		// timeout-duration.md: the name states the unit.
		"query --fetch-timeout-seconds": "unit is in the name",
		// timeout-duration.md changes this flag's declared type from string to
		// duration, but it is already parsed as a duration: "5m" works before
		// and after, and "300" is rejected before and after. Nothing a caller
		// can type changes meaning, so demoting it would refuse a working
		// command line under a stable floor to guard against a non-break.
		"auth login --timeout": "only the declared type changes",
		// unshadow-global-flags.md: the local flag goes away but the global one
		// has the same effect, so no command line changes meaning.
		"apply --dry-run":           "global --dry-run is equivalent",
		"update document --dry-run": "global --dry-run is equivalent",
		// cloud-flags-kebab-case.md: already kebab-case or unchanged.
		"create aws connection --name":    "no change",
		"create aws monitoring --regions": "no change",
		"create azure connection --type":  "no change",
		"create gcp connection --name":    "no change",
	}

	for key, why := range exclusions {
		got, ok := levels[key]
		if !ok {
			t.Errorf("%s: not on the command tree; this exclusion no longer describes anything", key)
			continue
		}
		if got != "stable" {
			t.Errorf("%s: is %q, want %q — the breaking change spares it (%s)", key, got, "stable", why)
		}
	}
}

// manifestRow matches either a command line or an indented flag line in the
// manifest's Surface block.
var manifestRow = regexp.MustCompile(`^(\s*)(\S+(?: \S+)*?)\s{2,}(stable|experimental|development)\b`)

// manifestFlagLevels maps "<command path> <--flag>" to the tier the manifest
// reports for it.
//
// The assertion runs against the rendered manifest rather than the cobra tree
// because the manifest is the checked-in artifact: if a demotion is real
// everywhere except in the file reviewers read, it is not real.
func manifestFlagLevels(t *testing.T, manifest string) map[string]string {
	t.Helper()

	_, surface, found := strings.Cut(manifest, "## Surface")
	if !found {
		t.Fatal("manifest has no Surface section")
	}

	levels := map[string]string{}
	command := ""
	for _, line := range strings.Split(surface, "\n") {
		m := manifestRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, name, level := m[1], m[2], m[3]
		if indent == "" {
			command = name
			continue
		}
		if strings.HasPrefix(name, "--") && command != "" {
			levels[command+" "+name] = level
		}
	}
	if len(levels) == 0 {
		t.Fatal("parsed no flags out of the manifest Surface section")
	}
	return levels
}

// pre10DemotedCommands are the commands the audit demoted whole, because every
// path through them requires a flag an accepted breaking change takes away.
//
// A flag-level mark is the right tool when a usable stable invocation survives
// it — `create azure connection --type federatedIdentityCredential` still works
// with the three clientSecret flags hidden. It is the wrong tool here: with the
// flags hidden, `dtctl update azure connection --help` offers only `--name`,
// and every invocation then fails with "at least one of --directoryId,
// --applicationId, or --clientSecret is required" — three flags the floor just
// removed from the help. Demoting the command turns that dead end into a
// `stability_blocked` error that names the exception which would admit it.
var pre10DemotedCommands = map[string]string{
	"update aws connection":   "--roleArn is required",
	"update azure connection": "needs one of --directoryId/--applicationId/--clientSecret",
	"update azure monitoring": "needs one of --locationFiltering/--featureSets",
	"update gcp connection":   "--serviceAccountId is required",
	"update gcp monitoring":   "needs one of --locationFiltering/--featureSets",
}

// pre10SurvivingCommands are commands that keep a usable stable invocation even
// with their demoted flags hidden, so they stay stable on purpose. Pinning them
// stops a well-meaning blanket demotion of everything the cloud rename touches.
var pre10SurvivingCommands = map[string]string{
	"create aws connection":   "--roleArn is optional",
	"create azure connection": "the federatedIdentityCredential path needs none of them",
	"create gcp connection":   "--serviceAccountId is optional",
	"create aws monitoring":   "--regions is not renamed",
	"update aws monitoring":   "--regions satisfies the one-of",
	"enable aws monitoring":   "--roleArn is optional",
	"enable azure monitoring": "the credential flags are optional",
	"enable gcp monitoring":   "--serviceAccountId is optional",
	"diff":                    "--format replaces the removed -o",
	"wait query":              "--verbose only adds progress output",
	"exec copilot":            "--context only adds conversation context",
	"restore workflow":        "--force only skips a prompt",
	"logs workflow-execution": "--follow only streams",
	"exec analyzer":           "--timeout has a default",
	"exec slo":                "--timeout has a default",
}

// TestPre10CommandDemotionsMatchTheAudit asserts the command-level split: a
// command is demoted exactly when no stable invocation survives.
func TestPre10CommandDemotionsMatchTheAudit(t *testing.T) {
	levels := manifestCommandLevels(t, cmd.StabilityManifest())

	for path, why := range pre10DemotedCommands {
		got, ok := levels[path]
		if !ok {
			t.Errorf("%s: not on the command tree", path)
			continue
		}
		if got != "experimental" {
			t.Errorf("%s: is %q, want experimental — %s, so a stable floor leaves it unusable",
				path, got, why)
		}
	}

	for path, why := range pre10SurvivingCommands {
		got, ok := levels[path]
		if !ok {
			t.Errorf("%s: not on the command tree", path)
			continue
		}
		if got != "stable" {
			t.Errorf("%s: is %q, want stable — %s, so demoting the whole command "+
				"withdraws more than the breaking change does", path, got, why)
		}
	}
}

// TestGlobalFlagsAreAllStable pins the global flag group to stable, because
// nothing yet enforces a weaker promise there.
//
// The manifest lists the root command's persistent flags so they are on the
// record, but the floor does not police them: blockBelowFloorFlags walks
// visitOwnFlags, which skips flags inherited from an ancestor, and installs its
// guard on the command that *declared* the flag. For a global flag that is
// rootCmd, whose RunE never runs for `dtctl get workflows --dry-run`. Marking a
// global flag experimental today would therefore badge it in help and list it
// here while leaving every subcommand free to accept it — a promise weaker on
// paper than in practice, which is the wrong direction to be wrong in.
//
// So: before demoting a global flag, teach the floor to check inherited
// persistent flags on the command actually being invoked. Then delete this
// test's expectation for that flag.
func TestGlobalFlagsAreAllStable(t *testing.T) {
	levels := manifestFlagLevels(t, cmd.StabilityManifest())

	found := 0
	for key, level := range levels {
		if !strings.HasPrefix(key, "(global) ") {
			continue
		}
		found++
		if level != "stable" {
			t.Errorf("%s: is %q, but the stability floor does not enforce global flags; "+
				"extend blockBelowFloorFlags to inherited persistent flags before demoting it",
				key, level)
		}
	}
	if found == 0 {
		t.Fatal("manifest lists no (global) flags; the group is missing from the generator")
	}
}

// manifestCommandLevels maps a command path to the tier the manifest reports.
func manifestCommandLevels(t *testing.T, manifest string) map[string]string {
	t.Helper()

	_, surface, found := strings.Cut(manifest, "## Surface")
	if !found {
		t.Fatal("manifest has no Surface section")
	}

	levels := map[string]string{}
	for _, line := range strings.Split(surface, "\n") {
		m := manifestRow.FindStringSubmatch(line)
		if m == nil || m[1] != "" {
			continue // blank indent means a command line; indented means a flag
		}
		levels[m[2]] = m[3]
	}
	if len(levels) == 0 {
		t.Fatal("parsed no commands out of the manifest Surface section")
	}
	return levels
}
