package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// agentRecipeListCap bounds a filtered listing in agent mode; has_more reports the cut.
const agentRecipeListCap = 25

type recipeListOptions struct {
	domain      string
	search      string
	tags        []string
	all         bool
	noInventory bool
	budget      float64
}

var getRecipesCmd = &cobra.Command{
	Use:     "recipes",
	Aliases: []string{"recipe"},
	Short:   "List recipes: named DQL queries you run with 'dtctl run'",
	Long: `List recipes: named, parameterized DQL queries run as 'dtctl run <recipe>'.

Recipes needing data this environment lacks are hidden, judged by the last
'dtctl inventory' verdicts (cached a day) or else a quick check bounded by
--inventory-budget. --all shows everything.

In agent mode the unfiltered listing is a domain index; --domain, --search or
--tag list the recipes themselves.

Examples:
  dtctl get recipes
  dtctl get recipes --domain k8s
  dtctl get recipes --search "slow endpoints"
  dtctl get recipes --tag rca -o wide

Recipes come from the built-in set and ~/.config/dtctl/recipes.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := recipeListOptions{}
		opts.domain, _ = cmd.Flags().GetString("domain")
		opts.search, _ = cmd.Flags().GetString("search")
		opts.tags, _ = cmd.Flags().GetStringSlice("tag")
		opts.all, _ = cmd.Flags().GetBool("all")
		opts.noInventory, _ = cmd.Flags().GetBool("no-inventory")
		opts.budget, _ = cmd.Flags().GetFloat64("inventory-budget")
		return listRecipes(cmd, opts)
	},
}

// listRecipes prints the recipe listing (also `dtctl run` with no recipe).
func listRecipes(cmd *cobra.Command, opts recipeListOptions) error {
	if opts.budget == 0 {
		opts.budget = 10
	}
	load := loadRecipeBook()
	book := load.book

	if opts.domain != "" && book.Domains[opts.domain] == nil {
		return recipeInputErr("unknown domain %q (domains: %s)", opts.domain, strings.Join(sortedDomainNames(book), ", "))
	}
	var candidates []*recipes.Recipe
	for _, r := range book.Sorted() {
		if opts.domain != "" && r.Domain() != opts.domain {
			continue
		}
		if !hasAllTags(r, opts.tags) {
			continue
		}
		candidates = append(candidates, r)
	}

	var warnings []string
	for _, p := range book.Problems {
		warnings = append(warnings, "skipped: "+p.String())
	}
	warnings = append(warnings, load.notes...)

	filter := &output.InventoryFilter{}
	hidden := map[string]bool{}
	hiddenFor := map[string]string{} // recipe → the absent capability
	if opts.noInventory {
		filter.Unfiltered = "--no-inventory"
	} else {
		// recipeVerdicts handles a nil config.
		cfg, err := LoadConfig()
		if err != nil {
			cfg = nil
		}
		verdicts, note := recipeVerdicts(cmd, cfg, book, candidates, opts.budget)
		if verdicts == nil {
			filter.Unfiltered = note
		} else {
			filter.Age = roundAge(verdicts.age()).String()
			for _, r := range candidates {
				for _, req := range r.Spec.Requires {
					if _, absent := verdicts.absent(req); absent {
						hidden[r.Name()] = true
						hiddenFor[r.Name()] = req
						break
					}
				}
			}
			filter.Hidden = len(hidden)
		}
	}

	visible := candidates
	if !opts.all && len(hidden) > 0 {
		visible = nil
		for _, r := range candidates {
			if !hidden[r.Name()] {
				visible = append(visible, r)
			}
		}
	}
	outranked := false
	if opts.search != "" {
		ranked := book.Search(opts.search, visible)
		visible = visible[:0:0]
		weak := true
		for _, m := range ranked {
			visible = append(visible, m.Recipe)
			weak = weak && m.Weak
		}
		switch {
		case len(ranked) == 0:
			warnings = append(warnings, fmt.Sprintf("no recipe matches %q: write the query with dtctl query (dtctl get recipes --domain <d> lists a domain's recipes, whose DQL is a starting point)", opts.search))
		case weak:
			warnings = append(warnings, fmt.Sprintf("no recipe is about %q: the ones listed only mention it in their description; check their summary before running one, or write the query with dtctl query", opts.search))
		}
		if !opts.all && len(hidden) > 0 {
			if w := hiddenBetterMatches(book, opts.search, candidates, hidden, hiddenFor); w != "" {
				warnings = append(warnings, w)
				outranked = true
			}
		}
	}

	filtered := opts.domain != "" || opts.search != "" || len(opts.tags) > 0
	printer := NewPrinter()
	ap := enrichAgent(printer, "get", "recipes")
	if ap == nil {
		for _, w := range warnings {
			output.PrintWarning("%s", w)
		}
		items := make([]recipes.ListItem, 0, len(visible))
		for _, r := range visible {
			items = append(items, recipes.Item(r))
		}
		if err := printer.PrintList(items); err != nil {
			return err
		}
		if outputFormat == "table" || outputFormat == "wide" || outputFormat == "" {
			printRecipeListFooter(filter, len(hidden), opts, len(items))
		}
		return nil
	}

	ctx := ap.Context()
	ctx.Inventory = filter
	ctx.Warnings = append(ctx.Warnings, warnings...)
	if !filtered {
		ap.SetSuggestions([]string{
			"dtctl get recipes --search \"<words of your question>\"  -- rank recipes for a question",
			"dtctl get recipes --domain <domain>  -- list one domain",
			"dtctl describe recipe <name>  -- params, DQL, how to read the result",
			"no recipe fits? write DQL: dtctl query '<dql>' (start from a recipe's DQL: dtctl run <name> --dry-run)",
		})
		return printer.PrintList(book.DomainIndex(candidates, hidden))
	}
	items := make([]recipes.ListItem, 0, len(visible))
	for _, r := range visible {
		items = append(items, recipes.Item(r))
	}
	total := len(items)
	if total > agentRecipeListCap {
		items = items[:agentRecipeListCap]
		ap.SetHasMore(true)
		ap.SetTotal(total)
	}
	suggestions := []string{"dtctl describe recipe <name>  -- params, DQL, how to read the result"}
	if len(items) > 0 && !outranked {
		suggestions = append(suggestions, strings.TrimSpace("dtctl run "+items[0].Name+" "+items[0].Args))
	}
	if total > agentRecipeListCap {
		suggestions = append(suggestions, "narrow it: add --search, --domain or --tag")
	}
	if len(items) == 0 {
		suggestions = append(suggestions, "no recipe matched: write the DQL yourself with dtctl query '<dql>'")
	}
	ap.SetSuggestions(suggestions)
	return printer.PrintList(items)
}

// hiddenBetterMatches names hidden recipes that outrank every listed match.
func hiddenBetterMatches(book *recipes.Book, query string, candidates []*recipes.Recipe, hidden map[string]bool, hiddenFor map[string]string) string {
	var names []string
	for _, m := range book.Search(query, candidates) {
		if !hidden[m.Recipe.Name()] {
			break
		}
		if len(names) < 3 {
			names = append(names, fmt.Sprintf("%s (needs %s)", m.Recipe.Name(), hiddenFor[m.Recipe.Name()]))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "the best matches are hidden because the inventory found their data absent here: " +
		strings.Join(names, ", ") + "; the listed recipes answer a different question (--all shows the hidden ones)"
}

func printRecipeListFooter(filter *output.InventoryFilter, hidden int, opts recipeListOptions, shown int) {
	switch {
	case hidden > 0 && !opts.all:
		fmt.Fprintf(os.Stderr, "\n%d recipe(s) hidden: they need data this environment lacks (inventory from %s ago). --all shows them.\n", hidden, filter.Age)
	case filter.Unfiltered != "" && filter.Unfiltered != "--no-inventory":
		fmt.Fprintf(os.Stderr, "\nNot filtered by inventory: %s.\n", filter.Unfiltered)
	}
	if shown > 0 {
		fmt.Fprintln(os.Stderr, "Run one with 'dtctl run <name>'; see its DQL and params with 'dtctl describe recipe <name>'.")
	}
}

func hasAllTags(r *recipes.Recipe, tags []string) bool {
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" && !r.HasTag(t) {
			return false
		}
	}
	return true
}

func sortedDomainNames(b *recipes.Book) []string {
	out := make([]string, 0, len(b.Domains))
	for name := range b.Domains {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// recipeVerdicts returns the verdicts that filter a listing: cached, else a quick
// structural pass. Nil comes with the reason the listing stays unfiltered, never an error.
func recipeVerdicts(cmd *cobra.Command, cfg *config.Config, book *recipes.Book, candidates []*recipes.Recipe, budget float64) (*inventoryVerdicts, string) {
	needed := map[string]bool{}
	for _, r := range candidates {
		for _, req := range r.Spec.Requires {
			needed[req] = true
		}
	}
	if len(needed) == 0 {
		return nil, "no listed recipe requires a capability"
	}
	if cfg == nil {
		return nil, "no environment configured"
	}
	if v := loadInventoryVerdicts(cfg); v != nil {
		return v, ""
	}
	if budget <= 0 {
		return nil, "no cached inventory (run 'dtctl inventory')"
	}
	defs := recipeCapabilityDefs(book)
	structural := map[string]*inventory.CapabilityDef{}
	for name := range needed {
		// Probe-shaped capabilities are left to `dtctl inventory`; unknown, never hidden.
		if d := defs[name]; d != nil && d.Probe == "" {
			structural[name] = d
		}
	}
	if len(structural) == 0 {
		return nil, "the required capabilities need a full inventory (run 'dtctl inventory')"
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return nil, "no client: " + compactErr(err)
	}
	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Duration(budget*float64(time.Second))+2*time.Second)
	defer cancel()
	runner := &inventoryRunner{executor: NewDQLExecutorFromConfig(cfg, c), scanLimitGB: 25}
	inv, err := inventory.Discover(ctx, runner, structural, inventory.DiscoverOptions{
		ContextName:   cfg.CurrentContext,
		BudgetQueries: 10,
		BudgetSeconds: budget,
	})
	if err != nil {
		return nil, "inventory check failed: " + compactErr(err)
	}
	v := verdictsFromInventory(inv, true)
	if cached := loadInventoryVerdicts(cfg); cached != nil {
		v.merge(cached)
	}
	saveInventoryVerdicts(cfg, v)
	return v, ""
}

// recipeCapabilityDefs is the capability set recipes are judged against:
// built-in definitions plus those app bundles ship.
func recipeCapabilityDefs(book *recipes.Book) map[string]*inventory.CapabilityDef {
	defs := inventory.BuiltinDefinitions()
	for name, d := range book.Capabilities {
		defs[name] = d
	}
	return defs
}

// roundAge rounds an age for display: minutes under a day, hours above.
func roundAge(d time.Duration) time.Duration {
	if d < time.Minute {
		return d.Round(time.Second)
	}
	if d < 24*time.Hour {
		return d.Round(time.Minute)
	}
	return d.Round(time.Hour)
}

// --- describe recipe -------------------------------------------------------

var describeRecipeCmd = &cobra.Command{
	Use:     "recipe <name>",
	Aliases: []string{"recipes"},
	Short:   "Show a recipe: params, window, scope, DQL, and how to read its result",
	Long: `Show a recipe: params, scope flags, window, the DQL it runs (defaults rendered,
required params as placeholders), how to read its result, and follow-up recipes.

Examples:
  dtctl describe recipe k8s-pod-restarts
  dtctl describe recipe services-failures -o yaml
`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeRecipeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		load := loadRecipeBook()
		r := load.book.Get(args[0])
		if r == nil {
			return unknownRecipeError(load, args[0])
		}
		d := load.book.Describe(r, time.Now())

		printer := NewPrinter()
		if ap := enrichAgent(printer, "describe", "recipe"); ap != nil {
			ap.Context().Recipe = &output.RecipeRef{Name: r.Name(), Version: r.Metadata.Version, Source: r.Source.String()}
			ap.SetSuggestions([]string{
				"run it: " + d.Usage,
				"see the DQL for your arguments: " + d.Usage + " --dry-run",
			})
			return printer.Print(d)
		}
		if outputFormat != "" && outputFormat != "table" && outputFormat != "wide" {
			return printer.Print(d)
		}
		printRecipeDescription(d)
		return nil
	},
}

func printRecipeDescription(d recipes.Description) {
	w := os.Stdout
	field := func(label, value string) {
		if value != "" {
			fmt.Fprintf(w, "%-12s %s\n", label+":", value)
		}
	}
	field("Name", d.Name)
	field("Summary", d.Summary)
	field("Domain", d.Domain)
	field("Version", fmt.Sprint(d.Version))
	src := d.Source
	if d.Location != "" {
		src += " (" + d.Location + ")"
	}
	field("Source", src)
	if len(d.Shadows) > 0 {
		field("Replaces", strings.Join(d.Shadows, ", "))
	}
	field("Tags", strings.Join(d.Tags, ", "))
	field("Usage", d.Usage)
	field("Window", d.Window)
	if !d.Segments {
		field("Segments", "not applicable")
	}
	field("Requires", strings.Join(d.Requires, ", "))
	field("Deprecated", d.Deprecated)
	if d.Description != "" {
		fmt.Fprintf(w, "\n%s\n", d.Description)
	}
	if len(d.Params) > 0 {
		fmt.Fprintln(w, "\nParams:")
		for _, p := range d.Params {
			var attrs []string
			attrs = append(attrs, p.Type)
			if p.Required {
				attrs = append(attrs, "required")
			}
			if p.Positional {
				attrs = append(attrs, "or as the argument")
			}
			if p.Default != "" {
				attrs = append(attrs, "default "+p.Default)
			}
			if len(p.Values) > 0 {
				attrs = append(attrs, "one of "+strings.Join(p.Values, "|"))
			}
			fmt.Fprintf(w, "  %-22s %s (%s)\n", p.Flag, p.Description, strings.Join(attrs, ", "))
		}
	}
	if len(d.Scope) > 0 {
		fmt.Fprintln(w, "\nScope (repeatable; values OR-combined, flags AND-combined):")
		for _, s := range d.Scope {
			fmt.Fprintf(w, "  %-22s %s (%s)\n", s.Flag, s.Description, s.Field)
		}
	}
	fmt.Fprintf(w, "\nReading the result:\n  %s\n", d.Means)
	fmt.Fprintf(w, "If it is empty:\n  %s\n", d.EmptyMeans)
	if len(d.Next) > 0 {
		fmt.Fprintf(w, "\nFollow-ups: %s\n", strings.Join(d.Next, ", "))
	}
	if d.DQL != "" {
		fmt.Fprintf(w, "\nDQL (defaults; placeholders for required params):\n")
		for _, line := range strings.Split(d.DQL, "\n") {
			fmt.Fprintln(w, "  "+line)
		}
	} else if d.RenderError != "" {
		fmt.Fprintf(w, "\nDQL: cannot render: %s\n", d.RenderError)
	}
}

// completeRecipeNames completes recipe names from the cache-only book.
func completeRecipeNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	load := loadRecipeBook()
	var out []string
	for _, r := range load.book.Sorted() {
		if strings.HasPrefix(r.Name(), toComplete) {
			out = append(out, r.Name()+"\t"+r.Spec.Summary)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// --- verify recipe ---------------------------------------------------------

var verifyRecipeCmd = &cobra.Command{
	Use:   "recipe [name...]",
	Short: "Check recipes: schema, template, cross-references, and DQL syntax",
	Long: `Check recipes without running them.

Each recipe is checked offline (schema, params, templates, rules, targets), then
its rendered DQL is verified by the environment's parser; --offline skips that.
-f checks a file holding a Recipe or a RecipeBundle.

Examples:
  dtctl verify recipe services-failures
  dtctl verify recipe --all
  dtctl verify recipe -f ./my-recipe.yaml
  dtctl verify recipe -f ./bundle.yaml --offline

Exit codes: 0 all valid, 1 any invalid, 2/3 as for 'verify query'.
`,
	ValidArgsFunction: completeRecipeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isSupportedVerifyOutputFormat(outputFormat) {
			return fmt.Errorf("unsupported output format %q for verify recipe (supported: json, yaml, toon)", outputFormat)
		}
		all, _ := cmd.Flags().GetBool("all")
		files, _ := cmd.Flags().GetStringArray("file")
		offline, _ := cmd.Flags().GetBool("offline")
		if !all && len(files) == 0 && len(args) == 0 {
			return recipeInputErr("name a recipe, pass -f <file>, or use --all")
		}
		if all && (len(files) > 0 || len(args) > 0) {
			return recipeInputErr("--all checks every loaded recipe; it does not combine with names or -f")
		}

		loader, _ := recipeLoader()
		book := loader.Load()
		var results []recipes.VerifyItem
		var targets []*recipes.Recipe
		lintFor := map[string]bool{}

		for _, f := range files {
			data, err := readFileFlag("file", f)
			if err != nil {
				return err
			}
			rs, problems, err := checkRecipeFile(loader, book, data, sourceName(f))
			if err != nil {
				results = append(results, recipes.VerifyItem{Name: sourceName(f), Status: "invalid", Message: err.Error()})
				continue
			}
			for _, p := range problems {
				results = append(results, recipes.VerifyItem{Name: sourceName(f), Status: "invalid", Message: p.Message})
			}
			for _, r := range rs.recipes {
				targets = append(targets, r)
				lintFor[r.Name()] = true
			}
			if len(rs.recipes) > 0 {
				book = rs.book
			}
		}
		if all {
			targets = book.Sorted()
			for _, p := range book.Problems {
				results = append(results, recipes.VerifyItem{Name: p.Source.String(), Status: "invalid", Message: p.String()})
			}
			for _, r := range targets {
				lintFor[r.Name()] = true
			}
		}
		for _, name := range args {
			r := book.Get(name)
			if r == nil {
				results = append(results, recipes.VerifyItem{Name: name, Status: "invalid", Message: "no such recipe"})
				continue
			}
			targets = append(targets, r)
			lintFor[name] = true
		}

		caps := map[string]bool{}
		for name := range recipeCapabilityDefs(book) {
			caps[name] = true
		}
		lint := map[string][]string{}
		for _, issue := range book.Lint(caps) {
			if lintFor[issue.Recipe] {
				lint[issue.Recipe] = append(lint[issue.Recipe], issue.Message)
			}
		}

		var executor *exec.DQLExecutor
		if !offline && len(targets) > 0 {
			_, c, err := SetupClient()
			if err != nil {
				return err
			}
			executor = exec.NewDQLExecutor(c)
		}
		exitCode := 0
		for _, r := range targets {
			item := recipes.VerifyItem{Name: r.Name(), Source: r.Source.String(), Status: "valid"}
			if msgs := lint[r.Name()]; len(msgs) > 0 {
				item.Status, item.Message = "invalid", strings.Join(msgs, "; ")
				results = append(results, item)
				continue
			}
			rendered, err := book.Example(r, time.Now())
			if err != nil {
				item.Status, item.Message = "invalid", err.Error()
				results = append(results, item)
				continue
			}
			if executor == nil {
				item.Status = "valid (offline)"
				results = append(results, item)
				continue
			}
			res, err := executor.VerifyQueryWithContext(cmdContext(cmd), rendered.DQL, exec.DQLVerifyOptions{})
			if code := getVerifyExitCode(res, err, false); code != 0 {
				item.Status = "invalid"
				if err != nil {
					item.Status, item.Message = "error", compactErr(err)
					if code > exitCode {
						exitCode = code
					}
				} else {
					item.Message = verifyNotificationText(res)
				}
			} else if res != nil {
				item.Message = verifyNotificationText(res)
			}
			results = append(results, item)
		}
		for _, it := range results {
			if it.Status == "invalid" && exitCode == 0 {
				exitCode = 1
			}
		}

		printer := NewPrinter()
		if ap := enrichAgent(printer, "verify", "recipe"); ap != nil && exitCode == 1 {
			ap.SetSuggestions([]string{"see a recipe's rendered DQL: dtctl describe recipe <name>"})
		}
		if err := printer.PrintList(results); err != nil {
			return err
		}
		if exitCode != 0 {
			return &silentExitError{code: exitCode, reason: "recipe verification failed"}
		}
		return nil
	},
}

func verifyNotificationText(res *exec.DQLVerifyResponse) string {
	if res == nil {
		return ""
	}
	var msgs []string
	for _, n := range res.Notifications {
		msgs = append(msgs, strings.ToLower(n.Severity)+": "+n.Message)
	}
	if !res.Valid && len(msgs) == 0 {
		return "the DQL parser rejected the query"
	}
	return strings.Join(msgs, "; ")
}

type checkedFile struct {
	book    *recipes.Book
	recipes []*recipes.Recipe
}

// checkRecipeFile lints a Recipe (user layer) or RecipeBundle (environment layer)
// against everything else loaded.
func checkRecipeFile(base recipes.Loader, book *recipes.Book, data []byte, location string) (*checkedFile, []recipes.Problem, error) {
	if _, err := recipes.ParseBundle(data); err == nil {
		src := recipes.Source{Layer: recipes.LayerEnvironment, Location: location, AppID: "local-file"}
		l := base
		l.Bundles = []recipes.BundleDoc{{Content: data, Source: src}}
		book := l.Load()
		return fileResult(book, location), fileProblems(book, location), nil
	}
	r, err := recipes.ParseRecipe(data)
	if err != nil {
		return nil, nil, err
	}
	r.Source = recipes.Source{Layer: recipes.LayerUser, Location: location}
	if err := book.Validate(r); err != nil {
		return &checkedFile{book: book}, []recipes.Problem{{Source: r.Source, Message: fmt.Sprintf("recipe %q: %v", r.Name(), err)}}, nil
	}
	if prev := book.Get(r.Name()); prev != nil && prev.Source.Location != location {
		r.Shadows = append(append([]recipes.Source{}, prev.Shadows...), prev.Source)
	}
	book.Recipes[r.Name()] = r
	return &checkedFile{book: book, recipes: []*recipes.Recipe{r}}, nil, nil
}

func fileResult(book *recipes.Book, location string) *checkedFile {
	out := &checkedFile{book: book}
	for _, r := range book.Sorted() {
		if r.Source.Location == location {
			out.recipes = append(out.recipes, r)
		}
	}
	return out
}

func fileProblems(book *recipes.Book, location string) []recipes.Problem {
	var out []recipes.Problem
	for _, p := range book.Problems {
		if p.Source.Location == location {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	getRecipesCmd.Flags().String("domain", "", "list one domain's recipes")
	getRecipesCmd.Flags().String("search", "", "rank recipes by how well they match these words")
	getRecipesCmd.Flags().StringSlice("tag", nil, "only recipes with every one of these tags")
	getRecipesCmd.Flags().Bool("all", false, "include recipes inventory hid (they need data this environment lacks)")
	getRecipesCmd.Flags().Bool("no-inventory", false, "do not filter by inventory verdicts (and run no check)")
	getRecipesCmd.Flags().Float64("inventory-budget", 10, "seconds the structural inventory check may take when no verdicts are cached (0 skips it)")
	_ = getRecipesCmd.RegisterFlagCompletionFunc("domain", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		load := loadRecipeBook()
		return sortedDomainNames(load.book), cobra.ShellCompDirectiveNoFileComp
	})

	verifyRecipeCmd.Flags().StringArrayP("file", "f", nil, "a Recipe or RecipeBundle file to check (repeatable; - for stdin)")
	verifyRecipeCmd.Flags().Bool("all", false, "check every loaded recipe")
	verifyRecipeCmd.Flags().Bool("offline", false, "skip the DQL parser check (no environment needed)")

	addDevelopmentCommand(getCmd, getRecipesCmd, recipesFeature)
	addDevelopmentCommand(describeCmd, describeRecipeCmd, recipesFeature)
	addDevelopmentCommand(verifyCmd, verifyRecipeCmd, recipesFeature)
}
