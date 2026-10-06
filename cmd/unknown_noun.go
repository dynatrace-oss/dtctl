package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/recipes"
)

// dataNouns are the data domains agents promote to top-level commands (evals:
// `dtctl problems`, `dtctl dql`, `dtctl synthetic`, `dtctl services`), each
// with a DQL starting point that reads it.
var dataNouns = map[string]string{
	"dql":             `dtctl query 'fetch logs, from:now()-1h | limit 10'`,
	"sql":             `dtctl query 'fetch logs, from:now()-1h | limit 10'`,
	"log":             `dtctl query 'fetch logs, from:now()-1h | limit 10'`,
	"event":           `dtctl query 'fetch events, from:now()-24h | summarize count(), by:{event.kind}'`,
	"events":          `dtctl query 'fetch events, from:now()-24h | summarize count(), by:{event.kind}'`,
	"bizevents":       `dtctl query 'fetch bizevents, from:now()-24h | summarize count(), by:{event.type}'`,
	"businessevents":  `dtctl query 'fetch bizevents, from:now()-24h | summarize count(), by:{event.type}'`,
	"business-events": `dtctl query 'fetch bizevents, from:now()-24h | summarize count(), by:{event.type}'`,
	"span":            `dtctl query 'fetch spans, from:now()-1h | limit 10'`,
	"spans":           `dtctl query 'fetch spans, from:now()-1h | limit 10'`,
	"trace":           `dtctl query 'fetch spans, from:now()-1h | limit 10'`,
	"traces":          `dtctl query 'fetch spans, from:now()-1h | limit 10'`,
	"metric":          `dtctl query 'fetch metric.series, from:now()-1h | limit 10'`,
	"metrics":         `dtctl query 'fetch metric.series, from:now()-1h | limit 10'`,
	"synthetic":       `dtctl query 'fetch dt.synthetic.events, from:now()-24h | limit 10'`,
	"synthetics":      `dtctl query 'fetch dt.synthetic.events, from:now()-24h | limit 10'`,
	"synmon":          `dtctl query 'fetch dt.synthetic.events, from:now()-24h | limit 10'`,
	"monitor":         `dtctl query 'fetch dt.synthetic.events, from:now()-24h | limit 10'`,
	"monitors":        `dtctl query 'fetch dt.synthetic.events, from:now()-24h | limit 10'`,
	"security":        `dtctl query 'fetch security.events, from:now()-24h | limit 10'`,
	"vulnerability":   `dtctl query 'fetch security.events, from:now()-24h | limit 10'`,
	"vulnerabilities": `dtctl query 'fetch security.events, from:now()-24h | limit 10'`,
	"problem":         `dtctl query 'fetch dt.davis.problems, from:now()-24h | limit 10'`,
	"problems":        `dtctl query 'fetch dt.davis.problems, from:now()-24h | limit 10'`,
	"service":         `dtctl query 'smartscapeNodes "SERVICE" | limit 10'`,
	"services":        `dtctl query 'smartscapeNodes "SERVICE" | limit 10'`,
	"host":            `dtctl query 'smartscapeNodes "HOST" | limit 10'`,
	"hosts":           `dtctl query 'smartscapeNodes "HOST" | limit 10'`,
}

// nounAdvice answers `dtctl <noun>` with the commands that do read that noun:
// a get resource it names, the recipes about it, and a DQL starting point.
// Edit distance answers these with an unrelated command (`slo` → "did you mean
// ctx?"), and an agent tries it. Nil when nothing reads the noun.
func nounAdvice(name string) []string {
	n := strings.ToLower(name)
	var out []string
	named := getResourcesNamed(n)
	// A guessed compound ("slo-status", "workflow-runs") names its resource
	// in its first word.
	if head, _, found := strings.Cut(n, "-"); found && len(named) == 0 {
		named = getResourcesNamed(head)
	}
	for _, r := range named {
		out = append(out, "dtctl get "+r)
	}
	// A resource the noun names is the answer; recipe search would add
	// prefix matches ("slo" → slow endpoints). Search runs only for a noun a
	// recipe is named or tagged with, so a typo ("quer") keeps its did-you-mean.
	if load := loadRecipeBook(cmdContext(rootCmd), parsedRecipeEnv()); len(out) == 0 && load.book != nil && recipeNoun(load.book, n) {
		for _, m := range load.book.Search(n, load.book.Sorted()) {
			if m.Weak || len(out) >= 3 {
				break
			}
			out = append(out, recipes.HintCommand(m.Recipe, recipes.QueryBinding{})+"  # "+m.Recipe.Spec.Summary)
		}
	}
	if q, ok := dataNouns[n]; ok {
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, "dtctl commands  # the full catalog")
}

// getResourcesNamed returns the `dtctl get` resources the noun names, in
// either number, and the ones it prefixes ("workflow" → workflows,
// workflow-executions).
func getResourcesNamed(n string) []string {
	var get *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "get" {
			get = c
		}
	}
	if get == nil {
		return nil
	}
	forms := []string{n, n + "s", strings.TrimSuffix(n, "s")}
	stem := strings.TrimSuffix(n, "s") + "-"
	names := func(c *cobra.Command) []string { return append([]string{c.Name()}, c.Aliases...) }
	var exact, prefixed []string
	for _, c := range get.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}
		switch {
		case slices.ContainsFunc(names(c), func(a string) bool { return slices.Contains(forms, a) }):
			exact = append(exact, c.Name())
		case strings.HasPrefix(c.Name(), stem):
			prefixed = append(prefixed, c.Name())
		}
	}
	exact = append(exact, prefixed...)
	return exact[:min(len(exact), 2)]
}

// recipeNoun reports whether a recipe's name or tags carry the noun as a word.
func recipeNoun(book *recipes.Book, n string) bool {
	singular := strings.TrimSuffix(n, "s")
	for _, r := range book.Sorted() {
		words := append(strings.Split(r.Name(), "-"), r.Metadata.Tags...)
		for _, w := range words {
			if w = strings.ToLower(w); w == n || w == singular || strings.TrimSuffix(w, "s") == singular {
				return true
			}
		}
	}
	return false
}
