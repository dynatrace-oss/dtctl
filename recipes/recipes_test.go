package recipes_test

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/cmd/testutil"
	rcp "github.com/dynatrace-oss/dtctl/pkg/recipes"
	builtin "github.com/dynatrace-oss/dtctl/recipes"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
	dtctlskill "github.com/dynatrace-oss/dtctl/skills/dtctl"
)

func loadBuiltin(t *testing.T) *rcp.Book {
	t.Helper()
	return rcp.Loader{Builtin: rcp.FileLayer{Layer: rcp.LayerBuiltin, FS: builtin.FS(), Root: "recipes"}}.Load()
}

// TestBuiltinRecipesLoadCleanly is the gate every built-in recipe passes:
// schema, template references, scope placement, window rules — and the
// cross-recipe lint (next targets, requires names, unused fragments).
func TestBuiltinRecipesLoadCleanly(t *testing.T) {
	b := loadBuiltin(t)
	for _, p := range b.Problems {
		t.Errorf("%s", p)
	}
	caps := map[string]bool{}
	for name := range inventory.BuiltinDefinitions() {
		caps[name] = true
	}
	for _, issue := range b.Lint(caps) {
		t.Errorf("%s: %s", issue.Recipe, issue.Message)
	}
	require.NotEmpty(t, b.Recipes)
}

// TestSkillNamesRealRecipes: the skill lists recipes by name so an agent
// meets them without a call; a renamed or removed recipe must not leave the
// skill pointing at nothing.
func TestSkillNamesRealRecipes(t *testing.T) {
	b := loadBuiltin(t)
	named := regexp.MustCompile("`(?:dtctl (?:run|describe recipe) )?([a-z0-9]+(?:-[a-z0-9]+)+)`")
	for _, file := range []string{"SKILL.md", "references/DQL-reference.md"} {
		body, err := fs.ReadFile(dtctlskill.Content, file)
		require.NoError(t, err)
		section := string(body)
		if file == "SKILL.md" {
			start := strings.Index(section, "## Recipes")
			require.GreaterOrEqual(t, start, 0, "SKILL.md lost its Recipes section")
			end := strings.Index(section[start+1:], "\n## ")
			section = section[start : start+1+end]
		}
		seen := 0
		for _, m := range named.FindAllStringSubmatch(section, -1) {
			if _, ok := b.Recipes[m[1]]; ok {
				seen++
				continue
			}
			if strings.Contains(m[1], "-") && b.Domains[strings.SplitN(m[1], "-", 2)[0]] != nil {
				t.Errorf("%s names recipe %q, which does not exist", file, m[1])
			}
		}
		if file == "SKILL.md" {
			assert.Greater(t, seen, 20, "the skill's recipe table should name the built-ins")
		}
	}
}

// TestBuiltinRecipeFileLayout keeps the tree navigable: recipes/<domain>/<name>.yaml.
func TestBuiltinRecipeFileLayout(t *testing.T) {
	b := loadBuiltin(t)
	for _, r := range b.Sorted() {
		want := path.Join("recipes", r.Domain(), r.Name()+".yaml")
		assert.Equal(t, want, r.Source.Location, "recipe %s", r.Name())
	}
}

// TestBuiltinRecipesStaySynthetic guards the privacy rule: built-in content
// names no environment, tenant URL or account.
func TestBuiltinRecipesStaySynthetic(t *testing.T) {
	envID := regexp.MustCompile(`\b[a-z]{3}[0-9]{5}\b`)
	tenantURL := regexp.MustCompile(`(?i)\.(live|apps|sprint|dev)\.dynatrace(labs)?\.com`)
	email := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}`)
	err := fs.WalkDir(builtin.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(builtin.FS(), p)
		require.NoError(t, err)
		for _, re := range []*regexp.Regexp{envID, tenantURL, email} {
			if m := re.FindString(string(data)); m != "" {
				t.Errorf("%s contains %q: built-in recipes must stay synthetic", p, m)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

// TestBuiltinRecipesRenderedDQL pins every recipe's DQL as rendered with its
// defaults (required params as placeholders) over a fixed clock. A change to
// a recipe's query shows up as a reviewable diff; refresh with -update.
func TestBuiltinRecipesRenderedDQL(t *testing.T) {
	b := loadBuiltin(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for _, r := range b.Sorted() {
		out, err := b.Example(r, now)
		if !assert.NoError(t, err, r.Name()) {
			continue
		}
		var sb strings.Builder
		sb.WriteString("# " + r.Name() + " v" + itoa(r.Metadata.Version) + " — window: " + r.Spec.Timeframe.Describe() + "\n")
		sb.WriteString(out.DQL + "\n")
		testutil.AssertGolden(t, "rendered/"+r.Name()+".dql", sb.String())
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return digits[i : i+1]
	}
	return itoa(i/10) + digits[i%10:i%10+1]
}
