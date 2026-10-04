package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// recipesCmd manages where recipes come from. Running them is `dtctl run`;
// listing them is `dtctl get recipes`.
var recipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "Manage recipe sources: declare, sync and pin where recipes come from",
	Long: `Manage recipe sources: where the recipes 'dtctl run' offers come from.

Besides the built-in recipes, dtctl loads the sources you declare:

  git      a GitHub repository at a ref     pinned to a commit
  archive  a .tar.gz at an https URL        pinned to its sha256
  app      the bundles an installed app     pinned per environment to
           ships to the environment         document versions
  dir      a local directory                read live (for authoring)

Sources are declared in ~/.config/dtctl/recipe-sources.yaml, or for a
project in .dtctl/recipes.yaml (found by walking up from the working
directory; commit it together with its .dtctl/recipes.lock so a team gets
identical recipes). 'dtctl recipes sync' fetches each source and records
what it resolved in the lock; 'dtctl run' then reads only the lock and the
local store, never the network. Recipes change only when someone syncs:
'sync' installs exactly what the lock names, 'sync --update' moves pins.

A project's sources are honoured only after you ran 'dtctl recipes sync' in
it, and again after its sources or lock change: recipe text is prompt input
for agents, and a cloned repository must not plant it by being the working
directory.`,
	Example: `  # Follow a GitHub repository's recipes, pinned to the commit 'main' resolves to now
  dtctl recipes add dt-for-ai --git github.com/<owner>/<repo> --ref main --path recipes

  # Enable the recipes an installed app ships to the current environment
  dtctl recipes add genai --app <app-id>

  # Install exactly what the lock names (a new machine, a teammate's checkout)
  dtctl recipes sync

  # See which pins have moved upstream, then move one
  dtctl recipes outdated
  dtctl recipes sync --update dt-for-ai

  # Where every recipe comes from, and at which version
  dtctl get recipe-sources`,
	RunE: requireSkillsSubcommand,
}

var recipesSyncCmd = &cobra.Command{
	Use:   "sync [source...]",
	Short: "Fetch declared recipe sources into the local store and pin them in the lock",
	Long: `Fetch declared recipe sources into the local store and pin them in the lock.

Without --update, sync installs exactly what the lock names and resolves only
sources the lock does not cover yet (new, or whose declaration changed). With
--update it re-resolves every source, or only the ones named, and reports
which recipes the move added, changed or removed.

App sources are synced for the current context's environment; pins for other
environments are kept as they are.`,
	Example: `  dtctl recipes sync
  dtctl recipes sync --update
  dtctl recipes sync --update dt-for-ai`,
	RunE: func(cmd *cobra.Command, args []string) error {
		update, _ := cmd.Flags().GetBool("update")
		if len(args) > 0 && !update {
			return fmt.Errorf("naming sources selects which pins --update moves; without --update, sync always installs every pin in the lock")
		}
		return runRecipeSync(cmd, update, args)
	},
}

var recipesOutdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Show which recipe source pins have moved upstream (changes nothing)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		files := loadRecipeSourceFiles()
		if err := recipeSourceFilesErr(files); err != nil {
			return err
		}
		syncer := newRecipeSyncer(files)
		var rows []recipes.OutdatedResult
		for _, f := range files {
			rows = append(rows, syncer.Outdated(cmdContext(cmd), f.File.Sources, f.Lock)...)
		}
		if len(rows) == 0 {
			output.PrintHint("No recipe sources declared. Add one with 'dtctl recipes add'.")
		}
		printer := NewPrinter()
		enrichAgent(printer, "outdated", "recipe-sources")
		return printer.PrintList(rows)
	},
}

var recipesAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Declare a recipe source and sync it",
	Args:  cobra.ExactArgs(1),
	Example: `  dtctl recipes add dt-for-ai --git github.com/<owner>/<repo> --ref v1.4.0 --path recipes
  dtctl recipes add team --archive https://example.invalid/recipes.tar.gz
  dtctl recipes add genai --app <app-id>
  dtctl recipes add drafts --dir ./recipes --project`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec := recipes.SourceSpec{Name: args[0]}
		spec.Git, _ = cmd.Flags().GetString("git")
		spec.Ref, _ = cmd.Flags().GetString("ref")
		spec.Path, _ = cmd.Flags().GetString("path")
		spec.Archive, _ = cmd.Flags().GetString("archive")
		spec.SHA256, _ = cmd.Flags().GetString("sha256")
		spec.App, _ = cmd.Flags().GetString("app")
		spec.Dir, _ = cmd.Flags().GetString("dir")
		if err := spec.Validate(); err != nil {
			return err
		}
		project, _ := cmd.Flags().GetBool("project")
		f, err := editableSourceFile(project)
		if err != nil {
			return err
		}
		if spec.Dir != "" {
			// A path typed on the command line means the working directory;
			// the file stores it relative to the project root when it lies
			// inside, so a committed project file works in every checkout.
			if spec.Dir, err = declaredDir(spec.Dir, f); err != nil {
				return err
			}
		}
		if f.File.Find(spec.Name) != nil {
			return fmt.Errorf("source %q is already declared in %s (remove it first, or edit the file)", spec.Name, f.Path)
		}
		wasTrusted := f.Origin != "project" || !fileExists(f.Path) || projectTrusted(f)
		f.File.Sources = append(f.File.Sources, spec)
		if err := writeSourcesFile(f); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Added recipe source %q to %s\n", spec.Name, f.Path)
		if noSync, _ := cmd.Flags().GetBool("no-sync"); noSync {
			// Re-trust only a file that was trusted before this edit: adding
			// one source must not silently enable sources someone else put
			// in the file. An untrusted file goes through sync, which shows
			// every source it enables.
			if f.Origin == "project" && wasTrusted {
				return trustProject(f)
			}
			return nil
		}
		return runRecipeSync(cmd, false, nil)
	},
}

var recipesRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a declared recipe source and its pins",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetBool("project")
		f, err := editableSourceFile(project)
		if err != nil {
			return err
		}
		if f.File.Find(args[0]) == nil {
			return fmt.Errorf("no recipe source %q in %s", args[0], f.Path)
		}
		wasTrusted := f.Origin != "project" || projectTrusted(f)
		kept := f.File.Sources[:0]
		for _, s := range f.File.Sources {
			if s.Name != args[0] {
				kept = append(kept, s)
			}
		}
		f.File.Sources = kept
		if f.Lock != nil {
			lk := f.Lock.Sources[:0]
			for _, e := range f.Lock.Sources {
				if e.Name != args[0] {
					lk = append(lk, e)
				}
			}
			f.Lock.Sources = lk
		}
		if err := writeSourcesFile(f); err != nil {
			return err
		}
		if err := writeLockFile(f); err != nil {
			return err
		}
		if f.Origin == "project" && wasTrusted {
			if err := trustProject(f); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "Removed recipe source %q from %s\n", args[0], f.Path)
		return nil
	},
}

// editableSourceFile returns the user file, or with project the nearest
// project file (created in the working directory when there is none).
func editableSourceFile(project bool) (*recipeSourceFile, error) {
	var f *recipeSourceFile
	if project {
		p := findProjectRecipeSources()
		if p == "" {
			wd, err := recipeWorkDir()
			if err != nil {
				return nil, err
			}
			p = filepath.Join(wd, projectRecipeDir, projectSourcesName)
		}
		f = &recipeSourceFile{Origin: "project", Path: p, LockPath: filepath.Join(filepath.Dir(p), projectLockName), Dir: filepath.Dir(filepath.Dir(p))}
	} else {
		p := recipeSourcesPath()
		f = &recipeSourceFile{Origin: "user", Path: p, LockPath: filepath.Join(filepath.Dir(p), userSourcesLockName), Dir: filepath.Dir(p), Trusted: true}
	}
	if !readRecipeSourceFile(f) {
		f.File = &recipes.SourcesFile{APIVersion: recipes.APIVersion, Kind: recipes.KindSources}
		f.Lock = &recipes.LockFile{APIVersion: recipes.APIVersion, Kind: recipes.KindLock}
	}
	if f.Err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, f.Err)
	}
	return f, nil
}

func declaredDir(dir string, f *recipeSourceFile) (string, error) {
	abs := dir
	if !filepath.IsAbs(abs) {
		wd, err := recipeWorkDir()
		if err != nil {
			return "", err
		}
		abs = filepath.Join(wd, dir)
	}
	if f.Origin == "project" {
		if rel, err := filepath.Rel(f.Dir, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel), nil
		}
	}
	return abs, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func writeSourcesFile(f *recipeSourceFile) error {
	data, err := f.File.Marshal()
	if err != nil {
		return err
	}
	header := "# Recipe sources (dtctl recipes --help). Pins live in the lock next to this file.\n"
	return writeFileAtomic(f.Path, append([]byte(header), data...))
}

func writeLockFile(f *recipeSourceFile) error {
	if f.Lock == nil {
		return nil
	}
	data, err := f.Lock.Marshal()
	if err != nil {
		return err
	}
	header := "# Generated by 'dtctl recipes sync'. Commit it with the sources file; do not edit.\n"
	return writeFileAtomic(f.LockPath, append([]byte(header), data...))
}

func recipeSourceFilesErr(files []*recipeSourceFile) error {
	for _, f := range files {
		if f.Err != nil {
			return fmt.Errorf("%s: %w", f.Path, f.Err)
		}
	}
	return nil
}

// newRecipeSyncer wires the syncer to the network and, when an app source
// needs it, to the current context's environment.
func newRecipeSyncer(files []*recipeSourceFile) recipes.Syncer {
	s := recipes.Syncer{Store: recipes.Store{Dir: recipeStoreDir()}, Remote: recipeRemote()}
	needsApp := false
	for _, f := range files {
		for _, spec := range f.File.Sources {
			if spec.Kind() == recipes.SourceApp {
				needsApp = true
			}
		}
	}
	if !needsApp {
		return s
	}
	cfg, err := LoadConfig()
	if err != nil {
		return s
	}
	s.Environment = recipeEnvironmentKey(cfg)
	if c, err := NewClientFromConfig(cfg); err == nil {
		s.Apps = documentBundleSource{c}
	}
	return s
}

// syncRow is one line of `dtctl recipes sync`.
type syncRow struct {
	Source string `json:"source" yaml:"source" table:"SOURCE"`
	Kind   string `json:"kind" yaml:"kind" table:"KIND"`
	Status string `json:"status" yaml:"status" table:"STATUS"`
	Pin    string `json:"pin,omitempty" yaml:"pin,omitempty" table:"PIN"`
	From   string `json:"from,omitempty" yaml:"from,omitempty" table:"FROM,wide"`
	File   string `json:"file" yaml:"file" table:"FILE,wide"`
	Error  string `json:"error,omitempty" yaml:"error,omitempty" table:"ERROR"`
}

func runRecipeSync(cmd *cobra.Command, update bool, names []string) error {
	files := loadRecipeSourceFiles()
	if err := recipeSourceFilesErr(files); err != nil {
		return err
	}
	if len(files) == 0 {
		output.PrintHint("No recipe sources declared. Add one with 'dtctl recipes add'.")
		return nil
	}
	opts := recipes.SyncOptions{UpdateAll: update && len(names) == 0, Update: map[string]bool{}}
	known := map[string]bool{}
	for _, f := range files {
		for _, s := range f.File.Sources {
			known[s.Name] = true
		}
	}
	for _, n := range names {
		if !known[n] {
			return fmt.Errorf("no recipe source %q is declared", n)
		}
		opts.Update[n] = true
	}

	before := recipeFingerprints(cmd)
	syncer := newRecipeSyncer(files)
	var rows []syncRow
	failed := 0
	for _, f := range files {
		if f.Origin == "project" && !f.Trusted {
			fmt.Fprintf(os.Stderr, "Enabling project recipe sources from %s:\n", f.Path)
			for _, s := range f.File.Sources {
				fmt.Fprintf(os.Stderr, "  %-20s %s %s\n", s.Name, s.Kind(), s.Location())
			}
		}
		lock, results := syncer.Sync(cmdContext(cmd), f.File.Sources, f.Lock, opts)
		f.Lock = lock
		for _, r := range results {
			if r.Status == recipes.SyncFailed {
				failed++
			}
			rows = append(rows, syncRow{Source: r.Source, Kind: string(r.Kind), Status: string(r.Status), Pin: r.To, From: r.From, File: f.Origin, Error: r.Error})
		}
		if err := writeLockFile(f); err != nil {
			return err
		}
		if f.Origin == "project" {
			if err := trustProject(f); err != nil {
				return err
			}
		}
	}

	printer := NewPrinter()
	ap := enrichAgent(printer, "sync", "recipe-sources")
	after := recipeFingerprints(cmd)
	changes := diffRecipeFingerprints(before, after)
	if ap != nil && changes != "" {
		ap.Context().Warnings = append(ap.Context().Warnings, changes)
	}
	if err := printer.PrintList(rows); err != nil {
		return err
	}
	if ap == nil && changes != "" {
		output.PrintHint("%s", changes)
	}
	if failed > 0 {
		return fmt.Errorf("%d recipe source(s) failed to sync; their previous pins are kept", failed)
	}
	return nil
}

// recipeFingerprints maps every loaded recipe to a digest of its content, so
// a sync can say which recipes it added, changed or removed.
func recipeFingerprints(cmd *cobra.Command) map[string]string {
	book := loadRecipeBook(cmdContext(cmd), parsedRecipeEnv()).book
	out := map[string]string{}
	for _, r := range book.Sorted() {
		data, err := yaml.Marshal(struct {
			M recipes.Metadata
			S recipes.Spec
		}{r.Metadata, r.Spec})
		if err != nil {
			continue
		}
		out[r.Name()] = recipes.BlobDigest(data) + "|" + r.Source.String()
		// The digest separates a content change from a recipe that merely
		// moved to another source (a new source overriding a built-in with
		// the same text).
	}
	return out
}

func diffRecipeFingerprints(before, after map[string]string) string {
	var added, changed, moved, removed []string
	for n, fp := range after {
		prev, ok := before[n]
		prevDigest, _, _ := strings.Cut(prev, "|")
		digest, _, _ := strings.Cut(fp, "|")
		switch {
		case !ok:
			added = append(added, n)
		case prevDigest != digest:
			changed = append(changed, n)
		case prev != fp:
			moved = append(moved, n)
		}
	}
	for n := range before {
		if _, ok := after[n]; !ok {
			removed = append(removed, n)
		}
	}
	if len(added)+len(changed)+len(moved)+len(removed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Recipes:")
	for _, part := range []struct {
		sign  string
		names []string
	}{{"added", added}, {"changed", changed}, {"same text, now from another source", moved}, {"removed", removed}} {
		if len(part.names) == 0 {
			continue
		}
		sort.Strings(part.names)
		fmt.Fprintf(&b, "\n  %d %s: %s", len(part.names), part.sign, abbreviateNames(part.names, 6))
	}
	return b.String()
}

func abbreviateNames(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", … %d more", len(names)-max)
}

// recipeSourceRow is one line of `dtctl get recipe-sources`.
type recipeSourceRow struct {
	Name     string `json:"name" yaml:"name" table:"NAME"`
	Kind     string `json:"kind" yaml:"kind" table:"KIND"`
	Location string `json:"location,omitempty" yaml:"location,omitempty" table:"LOCATION"`
	Pin      string `json:"pin,omitempty" yaml:"pin,omitempty" table:"PIN"`
	Synced   string `json:"synced,omitempty" yaml:"synced,omitempty" table:"SYNCED"`
	Origin   string `json:"origin" yaml:"origin" table:"ORIGIN"`
	Status   string `json:"status" yaml:"status" table:"STATUS"`
}

var getRecipeSourcesCmd = &cobra.Command{
	Use:     "recipe-sources",
	Aliases: []string{"recipe-source"},
	Short:   "List where recipes come from, and the version each source is pinned to",
	Long: `List where recipes come from: the built-in set, declared sources with the
version 'dtctl recipes sync' pinned, DTCTL_RECIPE_PATH and the user directory.

STATUS says whether a source contributes to 'dtctl run' right now: ok, live
(read from disk on every run), not synced, missing (pinned content is not in
the local store; 'dtctl recipes sync' restores it), untrusted (a project file
nobody synced yet), or off.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rows := recipeSourceRows(parsedRecipeEnv())
		printer := NewPrinter()
		enrichAgent(printer, "get", "recipe-sources")
		return printer.PrintList(rows)
	},
}

func recipeSourceRows(src recipeEnvSource) []recipeSourceRow {
	if runSession != nil {
		rows := []recipeSourceRow{{Name: "builtin", Kind: "builtin", Pin: version.Version, Origin: "dtctl", Status: "ok"}}
		for _, a := range runSession.RecipeApps {
			id, ver, _ := strings.Cut(a, "@")
			rows = append(rows, recipeSourceRow{Name: id, Kind: string(recipes.SourceApp), Location: id, Pin: ver, Origin: "request", Status: "live"})
		}
		return rows
	}
	files := loadRecipeSourceFiles()
	eff, builtin, _ := effectiveRecipeSources(files)
	bstatus := "ok"
	if !builtin {
		bstatus = "off"
	}
	rows := []recipeSourceRow{{Name: "builtin", Kind: "builtin", Pin: version.Version, Origin: "dtctl", Status: bstatus}}
	active := map[*recipeSourceFile]map[string]bool{}
	for _, es := range eff {
		if active[es.File] == nil {
			active[es.File] = map[string]bool{}
		}
		active[es.File][es.Spec.Name] = true
	}
	store := recipes.Store{Dir: recipeStoreDir()}
	envKey := recipeEnvironmentKey(src.cfg)
	for _, f := range files {
		if f.Err != nil {
			rows = append(rows, recipeSourceRow{Name: "-", Kind: "-", Location: f.Path, Origin: f.Origin, Status: "invalid: " + compactErr(f.Err)})
			continue
		}
		for _, s := range f.File.Sources {
			row := recipeSourceRow{Name: s.Name, Kind: string(s.Kind()), Location: s.Location(), Origin: f.Origin}
			entry := f.Lock.Find(s.Name)
			if entry != nil && entry.Spec != s.Fingerprint() {
				entry = nil
			}
			row.Pin = entry.Pin(s.Kind(), envKey)
			switch {
			case !f.Trusted:
				row.Status = "untrusted"
			case !active[f][s.Name]:
				row.Status = "overridden"
			case s.Kind() == recipes.SourceDir:
				row.Status = "live"
			case entry == nil:
				row.Status = "not synced"
			case s.Kind() == recipes.SourceApp:
				pins := entry.Env(envKey)
				switch {
				case envKey == "":
					row.Status = "no context"
				case pins == nil:
					row.Status = "not synced"
				default:
					row.Status = "ok"
					row.Synced = sinceString(pins.Synced)
					for _, b := range pins.Bundles {
						if _, ok := store.Blob(b.Digest); !ok {
							row.Status = "missing"
						}
					}
				}
			default:
				row.Synced = sinceString(entry.Synced)
				row.Status = "ok"
				if !store.HasTree(entry.Digest) {
					row.Status = "missing"
				}
			}
			rows = append(rows, row)
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv(recipePathEnv)) {
		if dir = strings.TrimSpace(dir); dir != "" {
			rows = append(rows, recipeSourceRow{Name: "-", Kind: string(recipes.SourceDir), Location: dir, Origin: recipePathEnv, Status: dirStatus(dir)})
		}
	}
	rows = append(rows, recipeSourceRow{Name: "-", Kind: string(recipes.SourceDir), Location: recipeUserDir(), Origin: "user", Status: dirStatus(recipeUserDir())})
	return rows
}

func dirStatus(dir string) string {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return "absent"
	}
	return "live"
}

func sinceString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func init() {
	rootCmd.AddCommand(recipesCmd)
	recipesCmd.AddCommand(recipesSyncCmd, recipesOutdatedCmd, recipesAddCmd, recipesRemoveCmd)
	getCmd.AddCommand(getRecipeSourcesCmd)

	recipesSyncCmd.Flags().Bool("update", false, "re-resolve sources and move their pins (all, or the ones named)")
	recipesAddCmd.Flags().String("git", "", "GitHub repository: github.com/<owner>/<repo>")
	recipesAddCmd.Flags().String("ref", "", "branch, tag or commit a git source follows (default: the default branch)")
	recipesAddCmd.Flags().String("path", "", "recipe root inside a git or archive source")
	recipesAddCmd.Flags().String("archive", "", "https URL of a .tar.gz")
	recipesAddCmd.Flags().String("sha256", "", "expected sha256 of an archive source")
	recipesAddCmd.Flags().String("app", "", "app ID whose recipe bundles to load from the environment")
	recipesAddCmd.Flags().String("dir", "", "local directory, read live (relative to the sources file)")
	recipesAddCmd.Flags().Bool("project", false, "declare it in the project's .dtctl/recipes.yaml instead of the user file")
	recipesAddCmd.Flags().Bool("no-sync", false, "declare only; sync later")
	recipesRemoveCmd.Flags().Bool("project", false, "remove it from the project's .dtctl/recipes.yaml")

	for _, c := range []*cobra.Command{recipesCmd, recipesSyncCmd, recipesOutdatedCmd, recipesAddCmd, recipesRemoveCmd, getRecipeSourcesCmd} {
		stability.Mark(c, stability.Experimental, recipesSince)
	}
}
