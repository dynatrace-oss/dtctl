package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

// runCmd is the parent of every recipe. Its children are not registered at
// init: they are content, generated from the loaded recipe book by
// attachRecipeCommands for the invocations that address them (see there).
var runCmd = &cobra.Command{
	Use:   "run <recipe> [arg] [flags]",
	Short: "Run a recipe: a named, parameterized DQL query (find one with 'dtctl get recipes')",
	Long: `Run a recipe: a named, parameterized DQL query that answers one common
question — open problems, a service's failures, pods that restarted — without
writing DQL. Each recipe is a subcommand with its own flags and --help.

A recipe is a pre-filled 'dtctl query': it renders its DQL and runs it through
the same path, so -o, the query limits, --spill, -S/--segment and every output
format work as they do for query. In agent mode the envelope adds
context.query (the rendered DQL, to adapt when the recipe is not quite the
question), context.window and context.scope.

Find a recipe:
  dtctl get recipes                         # the domain index
  dtctl get recipes --search "pods oom"     # rank recipes for a question
  dtctl describe recipe <name>              # params, DQL and how to read the result

Run one:
  dtctl run problems-active
  dtctl run services-failures checkout --from 6h
  dtctl run k8s-pod-restarts --namespace payments --min-restarts 3
  dtctl run logs-error-sources --dry-run    # print the DQL, execute nothing

Recipes come from four layers, strongest first: your own
(~/.config/dtctl/recipes/), your organization's (DTCTL_RECIPE_PATH), the
environment's (bundles that installed Dynatrace apps ship), and dtctl's
built-in set. A recipe replaces one of the same name from a weaker layer.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return listRecipes(cmd, recipeListOptions{})
		}
		return unknownRecipeError(currentRecipeLoad, args[0])
	},
}

// recipeLeafAnnotation marks a generated recipe command, so the stability
// manifest and the catalog can leave content out of the code's contract.
const recipeLeafAnnotation = "dtctl.dev/recipe"

// currentRecipeLoad is the book the attached recipe commands were built from,
// for the parent's error path. Nil when none were attached this invocation.
var currentRecipeLoad *recipeLoad

// currentRecipeEnv is the environment the attached book was loaded for.
var currentRecipeEnv recipeEnvSource

func init() {
	rootCmd.AddCommand(runCmd)
	// Recipes are experimental as a mechanism (the framework flags, the
	// param-to-flag mapping, the envelope keys). The leaves inherit the tier
	// from here, and are content: they are not in the manifest at all.
	stability.Mark(runCmd, stability.Experimental, recipesSince)
	// `run <unknown> --param x` must reach RunE to say the recipe is unknown
	// (after refreshing the app bundles), not fail on the leaf's flag.
	runCmd.FParseErrWhitelist.UnknownFlags = true
}

// attachRecipeCommands builds the `run` subtree from the recipe book when this
// invocation addresses it, and only then: loading touches the user's recipe
// directory and possibly the environment, which no other command should pay
// for, and a broken recipe file must never break an unrelated command.
//
// The subtree is rebuilt per invocation and detached afterwards, so an
// embedded caller never sees one tenant's app recipes in another's request.
// `dtctl commands` never attaches it: the catalog lists `run`, not the
// recipes, which keeps it (and the stability manifest) free of content.
func attachRecipeCommands(args []string) {
	detachRecipeCommands()
	pos, _ := recipeTarget(args)
	if len(pos) == 0 || pos[0] != "run" {
		return
	}
	src := rawRecipeEnv(args)
	load := loadRecipeBook(cmdContext(rootCmd), src)
	currentRecipeLoad = load
	currentRecipeEnv = src
	for _, r := range load.book.Sorted() {
		c := newRecipeCommand(load, r)
		runCmd.AddCommand(c)
		// executeArgs installed these on the tree before the leaves existed.
		setupErrorHandlers(c)
		installScopePreflight(c)
	}
}

// detachRecipeCommands removes the previous invocation's recipe commands and
// forgets their pristine snapshots.
func detachRecipeCommands() {
	currentRecipeLoad = nil
	currentRecipeEnv = recipeEnvSource{}
	for _, c := range runCmd.Commands() {
		if c.Annotations[recipeLeafAnnotation] == "" {
			continue
		}
		runCmd.RemoveCommand(c)
		delete(pristineTree, c)
	}
}

// recipeTarget returns argv's positional words (global flags and their
// values removed) with a leading completion or help marker stripped, and
// whether the invocation only completes or prints help — which reads the
// bundle cache and never the network.
func recipeTarget(args []string) (pos []string, helpOnly bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			helpOnly = true
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			if !strings.Contains(a, "=") && globalFlagTakesValue(a) {
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) > 0 {
		switch pos[0] {
		case cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd, "help":
			return pos[1:], true
		}
	}
	return pos, helpOnly
}

// globalFlagTakesValue reports whether a root persistent flag consumes the
// next word ("--context box"), so the value is not mistaken for a command.
func globalFlagTakesValue(token string) bool {
	var f *pflag.Flag
	if strings.HasPrefix(token, "--") {
		f = rootCmd.PersistentFlags().Lookup(token[2:])
	} else if len(token) == 2 {
		f = rootCmd.PersistentFlags().ShorthandLookup(token[1:])
	}
	return f != nil && f.NoOptDefVal == "" && f.Value.Type() != "bool"
}

// unknownRecipeError answers `run <name>` for a name no layer defines, with
// the closest recipes as runnable suggestions.
func unknownRecipeError(load *recipeLoad, name string) error {
	e := &suggest.CommandError{
		Command:   name,
		Message:   fmt.Sprintf("unknown recipe %q", name),
		UsageHint: "List recipes with 'dtctl get recipes' or search them with 'dtctl get recipes --search <words>'.",
	}
	if load != nil {
		// A recipe that failed to load is not unknown: say why it was
		// dropped, which is what its author needs to fix.
		for _, p := range load.book.Problems {
			if strings.Contains(p.Message, fmt.Sprintf("recipe %q:", name)) {
				e.Message = fmt.Sprintf("recipe %q did not load: %s", name, p.String())
				e.UsageHint = "Fix the file, then check it with 'dtctl verify recipe -f <file>'."
				return e
			}
		}
		for i, m := range load.book.Search(strings.ReplaceAll(name, "-", " "), load.book.Sorted()) {
			if i == 3 {
				break
			}
			e.Suggestions = append(e.Suggestions, suggest.Suggestion{Value: m.Recipe.Name()})
			e.Runnable = append(e.Runnable, "dtctl describe recipe "+m.Recipe.Name())
		}
		for _, n := range load.notes {
			e.UsageHint += "\nNote: " + n
		}
		// The recipe may be one an app on this environment ships but no
		// source enables yet. The cached listing answers that without a call.
		if hint := availableAppsHint(availableRecipeApps(cmdContext(rootCmd), currentRecipeEnv, false), load.enabledApps); hint != "" {
			e.UsageHint += "\n" + hint
		}
	}
	return e
}

// recipeInputError is a usage error in a recipe invocation: a missing or
// invalid param, a scope the recipe does not take, a window it rejects.
type recipeInputError struct{ msg string }

func (e *recipeInputError) Error() string { return e.msg }

func recipeInputErr(format string, args ...any) error {
	return &recipeInputError{msg: fmt.Sprintf(format, args...)}
}

// newRecipeCommand generates the cobra command for one recipe: params become
// flags, the scope dimensions it declares become --namespace and friends, and
// the query execution flags are shared with `dtctl query`.
func newRecipeCommand(load *recipeLoad, r *recipes.Recipe) *cobra.Command {
	use := r.Name()
	pos := r.Spec.Params.Positional()
	if pos != nil {
		if pos.Required {
			use += " <" + pos.Name + ">"
		} else {
			use += " [" + pos.Name + "]"
		}
	}
	short := r.Spec.Summary
	if r.Spec.Deprecated != nil {
		short += " (deprecated)"
	}
	c := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        recipeLongHelp(load.book, r),
		Annotations: map[string]string{recipeLeafAnnotation: r.Source.String()},
		Args:        cobra.NoArgs,
	}
	if pos != nil {
		c.Args = cobra.MaximumNArgs(1)
	}
	for _, p := range r.Spec.Params {
		addParamFlag(c, p)
	}
	for _, d := range r.Spec.Scope {
		dim := load.book.Scopes[d]
		if dim == nil {
			continue
		}
		usage := dim.Description + " (repeatable: values are OR-combined)"
		if dim.KeyValue() {
			usage = dim.Description + ", as key=value (repeatable: values of one key are OR-combined)"
		}
		c.Flags().StringArray(dim.FlagName(), nil, flagUsage(usage))
	}
	tf := r.Spec.Timeframe
	if !tf.None {
		c.Flags().String("from", "", fmt.Sprintf("start of the window: a duration ago (2h, 7d) or an RFC3339 timestamp (default %s)", recipes.FormatDuration(tf.Default)))
		c.Flags().String("to", "", "end of the window: a duration ago or an RFC3339 timestamp (default now)")
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the rendered DQL (stdout) and window (stderr); execute nothing")
	addDQLExecutionFlags(c, true)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return runRecipe(cmd, args, load, r)
	}
	return c
}

func addParamFlag(c *cobra.Command, p *recipes.Param) {
	usage := flagUsage(p.Description)
	if usage == "" {
		usage = p.Name
	}
	switch p.Type {
	case recipes.TypeEnum:
		usage += " (one of: " + strings.Join(p.Values, ", ") + ")"
	case recipes.TypeList:
		usage += " (comma-separated or repeated)"
	}
	if p.Pattern != "" {
		usage += " (must match " + p.Pattern + ")"
	}
	if p.Min != nil || p.Max != nil {
		switch {
		case p.Min != nil && p.Max != nil:
			usage += fmt.Sprintf(" (%d-%d)", *p.Min, *p.Max)
		case p.Min != nil:
			usage += fmt.Sprintf(" (min %d)", *p.Min)
		default:
			usage += fmt.Sprintf(" (max %d)", *p.Max)
		}
	}
	if p.Required {
		if p.Positional {
			usage += " (required; or as the argument)"
		} else {
			usage += " (required)"
		}
	} else if p.Positional {
		usage += " (or as the argument)"
	}
	def := p.DefaultString()
	name := p.FlagName()
	switch p.Type {
	case recipes.TypeInt:
		dv, _ := p.DefaultValue()
		n, _ := dv.(int64)
		c.Flags().Int64(name, n, usage)
	case recipes.TypeBool:
		dv, _ := p.DefaultValue()
		b, _ := dv.(bool)
		c.Flags().Bool(name, b, usage)
	case recipes.TypeList:
		var items []string
		if def != "" {
			items = strings.Split(def, ",")
		}
		c.Flags().StringSlice(name, items, usage)
	default:
		c.Flags().String(name, def, usage)
	}
	if p.Type == recipes.TypeEnum {
		values := append([]string{}, p.Values...)
		_ = c.RegisterFlagCompletionFunc(name, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		})
	}
}

// recipeLongHelp is the recipe's documentation: what it answers, how to read
// its result (also when it is empty), its window, and where it came from.
// flagUsage keeps content-authored text from naming a flag's value: pflag
// reads the first `backticked` word of a usage string as the placeholder.
func flagUsage(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func recipeLongHelp(book *recipes.Book, r *recipes.Recipe) string {
	var b strings.Builder
	if r.Spec.Description != "" {
		b.WriteString(strings.TrimSpace(r.Spec.Description))
	} else {
		b.WriteString(r.Spec.Summary)
	}
	b.WriteString("\n\nReading the result: " + strings.TrimSpace(r.Spec.Means))
	b.WriteString("\nIf it is empty: " + strings.TrimSpace(r.Spec.EmptyMeans))
	b.WriteString("\n\nWindow: " + r.Spec.Timeframe.Describe())
	if len(r.Spec.Requires) > 0 {
		b.WriteString("\nRequires: " + strings.Join(r.Spec.Requires, ", ") + " (capabilities from 'dtctl inventory')")
	}
	if r.Spec.Segments.Off() {
		b.WriteString("\nFilter segments: not applicable (this data must not be narrowed by a segment)")
	}
	b.WriteString("\nSource: " + r.Source.String())
	if r.Source.Location != "" && r.Source.Layer != recipes.LayerBuiltin {
		b.WriteString(" (" + r.Source.Location + ")")
	}
	if r.Spec.Deprecated != nil {
		b.WriteString("\n\nDeprecated: " + deprecationText(r))
	}
	if len(r.Spec.Next) > 0 {
		var names []string
		for _, n := range r.Spec.Next {
			if book.Get(n.Recipe) != nil {
				names = append(names, n.Recipe)
			}
		}
		if len(names) > 0 {
			b.WriteString("\n\nFollow-ups: " + strings.Join(names, ", "))
		}
	}
	b.WriteString("\n\nSee the DQL: dtctl run " + r.Name() + " --dry-run")
	return b.String()
}

func deprecationText(r *recipes.Recipe) string {
	d := r.Spec.Deprecated
	msg := strings.TrimSpace(d.Message)
	if d.ReplacedBy != "" {
		if msg != "" {
			msg += " "
		}
		msg += "Use 'dtctl run " + d.ReplacedBy + "' instead."
	}
	if msg == "" {
		msg = "this recipe will be removed in a future release."
	}
	return msg
}

// recipeInvocation is a parsed recipe command line.
type recipeInvocation struct {
	input    recipes.Input
	carry    recipes.Carry
	segments []string
}

// parseRecipeInvocation reads the params, scope and window from the command
// line. Every error is a usage error naming the flag.
func parseRecipeInvocation(cmd *cobra.Command, args []string, book *recipes.Book, r *recipes.Recipe, now time.Time) (*recipeInvocation, error) {
	inv := &recipeInvocation{input: recipes.Input{Params: map[string]any{}, Scope: map[string][]string{}}}
	fs := cmd.Flags()
	for _, p := range r.Spec.Params {
		name := p.FlagName()
		if !fs.Changed(name) {
			continue
		}
		var raw string
		switch p.Type {
		case recipes.TypeList:
			items, _ := fs.GetStringSlice(name)
			raw = strings.Join(items, ",")
		default:
			raw = fs.Lookup(name).Value.String()
		}
		v, err := p.ParseValue(raw)
		if err != nil {
			return nil, &recipeInputError{msg: err.Error()}
		}
		inv.input.Params[p.Name] = v
	}
	if pos := r.Spec.Params.Positional(); pos != nil && len(args) == 1 {
		if fs.Changed(pos.FlagName()) {
			return nil, recipeInputErr("%s given twice: as the argument and as --%s", pos.Name, pos.FlagName())
		}
		v, err := pos.ParseValue(args[0])
		if err != nil {
			return nil, &recipeInputError{msg: strings.Replace(err.Error(), "--"+pos.FlagName(), "<"+pos.Name+">", 1)}
		}
		inv.input.Params[pos.Name] = v
	}
	for _, d := range r.Spec.Scope {
		dim := book.Scopes[d]
		if dim == nil {
			continue
		}
		vals, _ := fs.GetStringArray(dim.FlagName())
		var clean []string
		for _, v := range vals {
			if v = strings.TrimSpace(v); v != "" {
				clean = append(clean, v)
			}
		}
		if len(clean) > 0 {
			inv.input.Scope[d] = clean
		}
	}
	if len(inv.input.Scope) > 0 {
		inv.carry.Scope = inv.input.Scope
	}
	from, _ := fs.GetString("from")
	to, _ := fs.GetString("to")
	w, err := recipes.ResolveWindow(r.Spec.Timeframe, from, to, now)
	if err != nil {
		return nil, &recipeInputError{msg: err.Error()}
	}
	inv.input.Window = w
	inv.carry.From, inv.carry.To = from, to

	segs, _ := fs.GetStringArray("segment")
	segFile, _ := fs.GetString("segments-file")
	if r.Spec.Segments.Off() && (len(segs) > 0 || segFile != "") {
		return nil, recipeInputErr("recipe %s does not take filter segments: its data (%s) must not be narrowed by one", r.Name(), strings.TrimSuffix(r.Spec.Summary, "."))
	}
	inv.segments = segs
	inv.carry.Segments = segs
	if segFile != "" {
		inv.segments = append(inv.segments, "file:"+segFile)
	}
	return inv, nil
}

// runRecipe renders and executes one recipe.
func runRecipe(cmd *cobra.Command, args []string, load *recipeLoad, r *recipes.Recipe) error {
	if !isSupportedQueryOutputFormat(outputFormat) {
		return fmt.Errorf("unsupported output format %q for run", outputFormat)
	}
	book := load.book
	inv, err := parseRecipeInvocation(cmd, args, book, r, time.Now())
	if err != nil {
		return err
	}
	rendered, err := book.Render(r, inv.input)
	if err != nil {
		return &recipeInputError{msg: err.Error()}
	}

	var warnings []string
	if r.Spec.Deprecated != nil {
		warnings = append(warnings, fmt.Sprintf("recipe %s is deprecated: %s", r.Name(), deprecationText(r)))
	}
	scope := map[string][]string{}
	for k, v := range inv.input.Scope {
		scope[k] = v
	}
	if len(inv.segments) > 0 {
		scope["segments"] = inv.segments
	}

	if dryRun {
		return printRecipeDryRun(cmd, r, rendered, scope, warnings)
	}

	cfg, c, err := SetupClient()
	if err != nil {
		return err
	}
	if verdicts := loadInventoryVerdicts(cfg); verdicts != nil {
		for _, req := range r.Spec.Requires {
			if ev, absent := verdicts.absent(req); absent {
				warnings = append(warnings, fmt.Sprintf("this environment lacks %q according to the inventory from %s ago (%s); the recipe ran anyway — refresh with 'dtctl inventory'", req, roundAge(verdicts.age()), ev))
			}
		}
	}
	if !agentMode {
		for _, w := range warnings {
			output.PrintWarning("%s", w)
		}
	}

	run := dqlRun{Query: rendered.DQL, EmptyHint: strings.TrimSpace(r.Spec.EmptyMeans), SkipEmptyDiagnosis: true, IsEmpty: r.IsEmpty}
	if rendered.Window != nil {
		run.TimeframeStart, run.TimeframeEnd = rendered.Window.FromRFC3339(), rendered.Window.ToRFC3339()
	}
	run.Decorate = func(ctx *output.ResponseContext, result *exec.DQLQueryResponse, records []map[string]interface{}) {
		decorateRecipeContext(ctx, book, r, rendered, inv, scope, warnings, result, records)
	}
	return runDQL(cmd, cfg, c, run)
}

// decorateRecipeContext adds the recipe's view to the query envelope: what
// ran, the rendered query, the effective window and scope, the recipe's own
// reading of an empty result, and its follow-ups as runnable suggestions.
func decorateRecipeContext(ctx *output.ResponseContext, book *recipes.Book, r *recipes.Recipe, rendered *recipes.Rendered,
	inv *recipeInvocation, scope map[string][]string, warnings []string, result *exec.DQLQueryResponse, records []map[string]interface{}) {
	ctx.Verb = "run"
	ctx.Resource = r.Name()
	ctx.Recipe = &output.RecipeRef{Name: r.Name(), Version: r.Metadata.Version, Source: r.Source.String()}
	ctx.Query = rendered.DQL
	if len(scope) > 0 {
		ctx.Scope = scope
	}
	requested := rendered.Window
	if requested == nil {
		requested = rendered.InlineWindow
	}
	if g := result.GetMetadata(); g != nil && g.AnalysisTimeframe != nil && g.AnalysisTimeframe.Start != "" {
		ctx.Window = &output.TimeWindow{From: g.AnalysisTimeframe.Start, To: g.AnalysisTimeframe.End}
		if rendered.Window != nil && windowDiffers(rendered.Window, g.AnalysisTimeframe.Start, g.AnalysisTimeframe.End) {
			ctx.Warnings = append(ctx.Warnings, "the query's own from:/to: overrode --from/--to; context.window is the window that was searched")
		}
	} else if requested != nil {
		ctx.Window = &output.TimeWindow{From: requested.FromRFC3339(), To: requested.ToRFC3339()}
	}
	ctx.Warnings = append(ctx.Warnings, warnings...)

	// records is nil only for a result streamed to disk, which is never empty.
	empty := r.IsEmpty(records)
	partial := recipePartialCause(result)
	switch {
	case empty && partial != "":
		// A scan that stopped early found nothing in the part it read: that
		// is "unknown", and the recipe's reading of an empty result would
		// turn it into a verified absence.
		ctx.EmptyReason = &output.EmptyReason{Code: "recipe_partial", Evidence: partialEvidence(partial)}
	case empty:
		ctx.EmptyReason = &output.EmptyReason{Code: "recipe_empty_means", Evidence: strings.TrimSpace(r.Spec.EmptyMeans)}
	case partial != "":
		ctx.Warnings = append(ctx.Warnings, "partial result: "+partialEvidence(partial)+"; counts are lower bounds and a row missing here may exist")
	}
	if n := finalLimit(rendered.DQL); n > 0 && len(records) == n {
		ctx.HasMore = true
		ctx.Suggestions = append(ctx.Suggestions, fmt.Sprintf("# the recipe stops at %d rows and returned %d: this is the top of a longer list, not a total — count with dtctl query '<context.query>' after replacing the final | limit with | summarize n = count()", n, n))
	}
	next := book.NextCommands(r, rendered.Params, inv.carry, empty && partial == "", records)
	adapt := "adapt the recipe: dtctl query '<context.query>'"
	if requested != nil && rendered.Window != nil {
		adapt += " --from " + requested.FromRFC3339() + " --to " + requested.ToRFC3339()
	}
	ctx.Suggestions = append(append(next, ctx.Suggestions...), adapt)
}

// windowDiffers reports whether the searched window is not the one sent: the
// query's own from:/to: took precedence over the default timeframe.
func windowDiffers(w *recipes.Window, start, end string) bool {
	s, err1 := time.Parse(time.RFC3339, start)
	e, err2 := time.Parse(time.RFC3339, end)
	if err1 != nil || err2 != nil {
		return false
	}
	const slack = 2 * time.Minute
	return absDuration(s.Sub(w.From)) > slack || absDuration(e.Sub(w.To)) > slack
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// printRecipeDryRun prints the rendered query. The DQL goes to stdout and
// the window to stderr, so `dtctl query "$(dtctl run x --dry-run)"` works.
func printRecipeDryRun(cmd *cobra.Command, r *recipes.Recipe, rendered *recipes.Rendered, scope map[string][]string, warnings []string) error {
	w := rendered.Window
	if w == nil {
		w = rendered.InlineWindow
	}
	rep := newDryRunReport(cmd).As("run", r.Name())
	for _, line := range strings.Split(rendered.DQL, "\n") {
		rep.Linef("%s", line)
	}
	rep.Detail("query", "%s", rendered.DQL)
	ctx := &output.ResponseContext{
		Verb:     "run",
		Resource: r.Name(),
		Recipe:   &output.RecipeRef{Name: r.Name(), Version: r.Metadata.Version, Source: r.Source.String()},
		Query:    rendered.DQL,
		Warnings: warnings,
	}
	if w != nil {
		rep.Detail("from", "%s", w.FromRFC3339()).Detail("to", "%s", w.ToRFC3339())
		ctx.Window = &output.TimeWindow{From: w.FromRFC3339(), To: w.ToRFC3339()}
	}
	if len(scope) > 0 {
		ctx.Scope = scope
	}
	ctx.Suggestions = []string{"run it: the same command without --dry-run"}
	rep.WithContext(ctx)
	if !agentMode {
		for _, msg := range warnings {
			output.PrintWarning("%s", msg)
		}
		switch {
		case w == nil:
			fmt.Fprintln(os.Stderr, "# window: none (state query)")
		case rendered.InlineWindow != nil:
			fmt.Fprintf(os.Stderr, "# window: %s .. %s (written into the query)\n", w.FromRFC3339(), w.ToRFC3339())
		default:
			// The window is not in the DQL; name the flags that carry it
			// to a 'dtctl query' of the printed statement.
			fmt.Fprintf(os.Stderr, "# window: %s .. %s (with dtctl query: --from %s --to %s)\n",
				w.FromRFC3339(), w.ToRFC3339(), w.FromRFC3339(), w.ToRFC3339())
		}
		if segs := scope["segments"]; len(segs) > 0 {
			fmt.Fprintf(os.Stderr, "# segments: %s (applied by the server, not in the DQL)\n", strings.Join(segs, ", "))
		}
	}
	return rep.Print()
}

// recipePartialCause is the first reason the query result is incomplete, as
// an exec.Partial* cause, or "".
func recipePartialCause(result *exec.DQLQueryResponse) string {
	if result == nil {
		return ""
	}
	for _, n := range result.GetNotifications() {
		if c := exec.PartialCause(n); c != "" {
			return c
		}
	}
	return ""
}

// partialEvidence names the cut and its remedy.
func partialEvidence(cause string) string {
	switch cause {
	case exec.PartialScanLimit:
		return "the scan stopped at its data limit before reading the whole window — narrow --from/--to or the scope, or raise --default-scan-limit-gbytes"
	case exec.PartialTimeout:
		return "the query timed out before reading the whole window — narrow --from/--to or the scope"
	case exec.PartialResultLimit:
		return "the result was cut at the record limit — aggregate further or raise --max-result-records"
	case exec.PartialConsumption:
		return "the query stopped at the consumption limit — narrow --from/--to or the scope"
	}
	return "the query result is incomplete (" + cause + ")"
}

var finalLimitRe = regexp.MustCompile(`(?s)\|\s*limit\s+(\d+)\s*$`)

// finalLimit is N of a statement's closing | limit N, or 0.
func finalLimit(dql string) int {
	m := finalLimitRe.FindStringSubmatch(strings.TrimSpace(dql))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
