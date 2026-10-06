package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/pkg/version"
	builtinrecipes "github.com/dynatrace-oss/dtctl/recipes"
)

// Recipe state locations; variables so tests can point them at a temp dir.
var (
	recipeUserDir   = func() string { return filepath.Join(config.ConfigDir(), "recipes") }
	recipeCacheRoot = config.CacheDir
)

// recipesFeature is the development feature that registers recipes:
// `dtctl config set development.recipes on` or DTCTL_DEVELOPMENT=recipes.
const recipesFeature = "recipes"

// recipesEnabled reports whether this invocation registered the recipe commands;
// anything that points at a recipe asks first.
func recipesEnabled() bool {
	// By name, not runCmd: runCmd's RunE reaches this function.
	for _, c := range rootCmd.Commands() {
		if c.Name() == "run" {
			return true
		}
	}
	return false
}

// recipeLoad is the merged book for one invocation.
type recipeLoad struct {
	book *recipes.Book
	// notes say why content that should have loaded did not.
	notes []string
}

// loadRecipeBook merges the built-in and user recipes; no network. A session loads built-ins only.
func loadRecipeBook() *recipeLoad {
	l, load := recipeLoader()
	load.book = l.Load()
	return load
}

// recipeLoader is the loader loadRecipeBook runs, for a caller that adds a
// layer of its own first (`verify recipe -f`).
func recipeLoader() (recipes.Loader, *recipeLoad) {
	l := recipes.Loader{
		Builtin:      recipes.FileLayer{Layer: recipes.LayerBuiltin, FS: builtinrecipes.FS(), Root: "builtin"},
		DtctlVersion: releaseVersion(),
	}
	load := &recipeLoad{}
	if runSession != nil {
		return l, load
	}
	userDir := recipeUserDir()
	l.Files = append(l.Files, recipes.FileLayer{Layer: recipes.LayerUser, FS: os.DirFS(userDir), Root: userDir})
	return l, load
}

// releaseVersion is the version recipes' minDtctlVersion is checked against;
// a development build checks nothing.
func releaseVersion() string {
	v := version.Version
	if v == "" || v == "dev" || strings.Contains(v, "dirty") {
		return ""
	}
	return v
}

// safeFileName maps a free-form context name to a file name that cannot escape the cache dir.
func safeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
}

func compactErr(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
