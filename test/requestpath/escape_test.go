// Package requestpath_test is the build-time half of dtctl's defence against a
// resource id that addresses the wrong resource.
//
// httpclient.CheckRequestPath is the run-time half: it refuses a path that has
// already been assembled wrong. This test is why the assembly stays right. It
// reads every Go source file in both modules and requires that a value
// interpolated into a request path goes through httpclient.PathSegment, because
// the failure it prevents is silent -- an id containing '/' or '?' simply names
// something else, and the API answers about that something else.
//
// It lives in test/ rather than beside either module because the pattern spans
// both: sdk/api/* holds most of it, pkg/exec and pkg/resources hold the rest.
//
// It matches a literal format string, which is how nearly every handler spells
// its path. A handler that builds the format from a constant instead is not
// covered here and falls to httpclient.CheckRequestPath at run time -- so a
// failure of this test is a bug, but a pass is not a proof.
package requestpath_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pathSprintf matches a single-line `fmt.Sprintf("/…", args…)` whose format
// string is a request path. Multi-line calls are matched by their first line
// and their arguments read from the lines that follow.
//
// A path is also spelled `fmt.Sprintf("%s/%s", basePath, id)`, where a constant
// holds the leading portion. That form is matched too, and it is the one that
// hid ten unescaped ids through two rounds of this fix: the settings-backed
// handlers and the cloud monitoring-config handlers all build their paths that
// way, so a guard that looked only for a literal "/" could not see them. The
// leading "%s" is the constant and is not an id, so it is not counted -- see
// stringVerbs.
//
// "%s/%s" is also how a User-Agent, a display label and a browser URL are
// spelled, and none of those is a request path. So that form counts only when
// the call is an argument to an HTTP verb on the same line, which is how every
// handler in both modules writes it. A literal "/…" needs no such proof.
var pathSprintf = regexp.MustCompile(`fmt\.Sprintf\("(/[^"]*)"(.*)$`)

// basePathSprintf matches the `fmt.Sprintf("%s/…", basePath, …)` form.
var basePathSprintf = regexp.MustCompile(`fmt\.Sprintf\("(%s/[^"]*)"(.*)$`)

// httpVerb reports a resty call that sends the path it is given.
// A chained call puts the verb at the start of a continuation line, with the
// dot on the line before, so a leading verb counts too.
var httpVerb = regexp.MustCompile(`(?:\.|^\s*)(Get|Post|Put|Patch|Delete|Head|Options)\(`)

// skipDirs are trees with no request paths in them; walking them only invites
// false positives from documentation samples and fixtures.
var skipDirs = map[string]bool{
	".git": true, "docs": true, "test": true, "dist": true, "node_modules": true,
}

func TestEveryInterpolatedPathValueIsEscaped(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			m := pathSprintf.FindStringSubmatch(line)
			if m == nil {
				// The base-path form is a request path only when it is handed
				// straight to an HTTP verb.
				if !httpVerb.MatchString(line) {
					continue
				}
				m = basePathSprintf.FindStringSubmatch(line)
			}
			if m == nil {
				continue
			}
			format, args := m[1], m[2]
			// A multi-line call carries its arguments on the following lines.
			for j := i + 1; j < len(lines) && !strings.Contains(args, ")"); j++ {
				args += lines[j]
			}
			if n := len(stringVerbs(format)); n > 0 && countEscapers(args) < n {
				offenders = append(offenders, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	}))

	require.Empty(t, offenders, "a %%s in a request path must be wrapped in httpclient.PathSegment:\n%s\n\n"+
		"An id reaches dtctl from a command line or an applied file. Interpolated raw, one "+
		"containing '/', '?' or '#' addresses a different resource and the request succeeds "+
		"against it -- see httpclient.CheckRequestPath.", strings.Join(offenders, "\n"))
}

// escapers are the calls that count as escaping one interpolated value. Each is
// matched as a substring, so "PathSegment(" covers the qualified
// "httpclient.PathSegment(" too.
//
// httpclient.PathSegment is the rule, and url.PathEscape is deliberately absent:
// PathSegment is PathEscape plus the colon, and a bare PathEscape is exactly the
// gap it closes -- several collections spell an action as a suffix on the
// resource segment ("/analyzers/%s:poll"), so an id carrying a colon names an
// operation rather than a resource. A call site that reaches for PathEscape
// fails here instead of reopening that hole quietly.
//
// escapeFunctionName is the single exception, and it is named here rather than
// exempted by file so that adding another one is a deliberate edit to this list:
// an app function name is a path by design -- parseFullFunctionName splits
// "app-id/a/b/c" at the first slash only, so the slashes left in "a/b/c" are
// separators the server expects. Escaping the whole name would spell them "%2F"
// and address a function that does not exist, so that one value is escaped part
// by part instead.
var escapers = []string{"PathSegment(", "escapeFunctionName("}

// countEscapers reports how many of a call's arguments are escaped.
func countEscapers(args string) int {
	n := 0
	for _, e := range escapers {
		n += strings.Count(args, e)
	}
	return n
}

// stringVerbs returns the %s verbs in a format string that stand for an
// interpolated id. %d and the rest cannot carry a path separator, so they need
// no escaping.
//
// A format that opens with "%s/" names a base path in a constant, and that
// first verb is not an id, so it is skipped. Every later verb is one.
func stringVerbs(format string) []string {
	var out []string
	if strings.HasPrefix(format, "%s/") {
		format = format[len("%s"):]
	}
	for i := 0; i < len(format)-1; i++ {
		if format[i] != '%' {
			continue
		}
		switch format[i+1] {
		case '%':
			i++
		case 's', 'v', 'q':
			out = append(out, format[i:i+2])
		}
	}
	return out
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "sdk")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "could not find the repository root")
		dir = parent
	}
}
