package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
)

// Recipe pointers in `dtctl query` envelopes: an agent writing DQL for data a
// recipe covers is told so in the response it reads anyway. Agents left to
// find recipes on their own never did (0 of 168 runs in the first
// evaluation), and the recipes encode exactly what an ad-hoc query tends to
// get wrong — which spans count as failed, that each model call is recorded
// twice, which metric keys drop series.
//
// How loudly depends on how the query went. Next to a result that looks fine
// only a specific match is named; an empty, cut-short or failed query is
// where a verified starting point helps most, so a looser match qualifies.

// recipeHintMode is how the query went.
type recipeHintMode int

const (
	recipeHintOK recipeHintMode = iota
	recipeHintEmpty
	recipeHintFailed
)

// maxRecipeHints bounds the pointers per response.
const maxRecipeHints = 2

// recipeHintsForQuery returns the suggestions naming recipes for query.
func recipeHintsForQuery(cmd *cobra.Command, cfg *config.Config, query string, mode recipeHintMode) []string {
	book := queryRecipeBook(cmd, cfg)
	if book == nil {
		return nil
	}
	return queryRecipeHints(book, query, queryFromFlag(cmd), mode)
}

func queryRecipeBook(cmd *cobra.Command, cfg *config.Config) *recipes.Book {
	load := loadRecipeBook(cmdContext(cmd), recipeEnvSource{cfg: cfg, newClient: NewClientFromConfig})
	if load == nil {
		return nil
	}
	return load.book
}

func queryFromFlag(cmd *cobra.Command) string {
	from, _ := cmd.Flags().GetString("from")
	return from
}

func activeRecipes(book *recipes.Book) []*recipes.Recipe {
	var among []*recipes.Recipe
	for _, r := range book.Sorted() {
		if r.Spec.Deprecated == nil {
			among = append(among, r)
		}
	}
	return among
}

// queryRecipeHints names the recipes that match query, each as a command
// the agent can run as it stands: the params, scope and window the query
// already names are filled in (see recipes.BindQuery). One step to the
// answer, where a pointer to `describe recipe` was two.
func queryRecipeHints(book *recipes.Book, query, from string, mode recipeHintMode) []string {
	var out []string
	for _, m := range book.MatchQuery(query, activeRecipes(book)) {
		if len(out) == maxRecipeHints {
			break
		}
		if mode == recipeHintOK && !m.Strong || mode != recipeHintOK && m.Score < 3 {
			continue
		}
		out = append(out, recipeHint(book, m.Recipe, query, from, mode))
	}
	return out
}

func recipeHint(book *recipes.Book, r *recipes.Recipe, query, from string, mode recipeHintMode) string {
	lead := "a verified recipe for this question"
	switch mode {
	case recipeHintEmpty:
		lead = "a verified recipe for this data, which may know why this came back empty"
	case recipeHintFailed:
		lead = "a verified recipe for this data, a working starting point"
	}
	return fmt.Sprintf("%s  # %s: %s", recipes.HintCommand(r, book.BindQuery(r, query, from)), lead, strings.TrimSuffix(r.Spec.Summary, "."))
}

// maxQueryWarnings bounds the trap warnings per response.
const maxQueryWarnings = 3

// queryRecipeWarnings tests an ad-hoc query against the traps the recipes
// know (their checks) and the DQL trap lints. A warning reaches the agent
// in the response it reads anyway and says how to write the query instead,
// so fixing it costs no extra call.
func queryRecipeWarnings(book *recipes.Book, query, from string) []string {
	var out []string
	for _, hit := range book.QueryChecks(query, activeRecipes(book)) {
		out = append(out, fmt.Sprintf("%s (%s does this)", hit.Warn, recipes.HintCommand(hit.Recipe, book.BindQuery(hit.Recipe, query, from))))
	}
	out = append(out, recipes.LintQuery(query, recipes.EffectiveQueryWindow(query, from))...)
	if len(out) > maxQueryWarnings {
		out = out[:maxQueryWarnings]
	}
	return out
}

// decorateQueryWithRecipes adds recipe pointers and trap warnings to a
// query envelope.
func decorateQueryWithRecipes(cmd *cobra.Command, cfg *config.Config, query string) func(*output.ResponseContext, *exec.DQLQueryResponse, []map[string]interface{}) {
	return func(ctx *output.ResponseContext, result *exec.DQLQueryResponse, records []map[string]interface{}) {
		ctx.Window = queryWindowContext(result, cmd.Flags().Changed("from") || cmd.Flags().Changed("to") || recipes.NamesWindow(query))
		book := queryRecipeBook(cmd, cfg)
		if book == nil {
			return
		}
		from := queryFromFlag(cmd)
		ctx.Warnings = append(ctx.Warnings, queryRecipeWarnings(book, query, from)...)
		mode := recipeHintOK
		if (records != nil && len(records) == 0) || recipePartialCause(result) != "" {
			mode = recipeHintEmpty
		}
		hints := queryRecipeHints(book, query, from, mode)
		switch {
		case len(hints) == 0:
		case ctx.EmptyReason != nil:
			// A diagnosed cause (a misspelt field) is the better lead.
			ctx.Suggestions = append(ctx.Suggestions, hints...)
		default:
			ctx.Suggestions = append(hints, ctx.Suggestions...)
		}
	}
}

// queryWindowContext is the window a query searched, from the response's
// own metadata, with its length spelled out. A query that names no window
// reads the last 2h, and an agent asked about "the last 24h" that forgets
// from: gets a confident count of 2h with nothing in the response saying
// so; the note does (measured: a bizevents count reported as 24h was 2h in
// every run of one evaluation task).
func queryWindowContext(result *exec.DQLQueryResponse, named bool) *output.TimeWindow {
	g := result.GetMetadata()
	if g == nil || g.AnalysisTimeframe == nil || g.AnalysisTimeframe.Start == "" {
		return nil
	}
	w := &output.TimeWindow{From: g.AnalysisTimeframe.Start, To: g.AnalysisTimeframe.End}
	start, err1 := time.Parse(time.RFC3339, w.From)
	end, err2 := time.Parse(time.RFC3339, w.To)
	if err1 != nil || err2 != nil || !end.After(start) {
		return w
	}
	w.Span = spanString(end.Sub(start))
	if !named {
		w.Note = "the query names no window, so it read the default last " + w.Span + "; widen it with fetch ..., from: now()-24h or --from 24h"
	}
	return w
}

// spanString is d in the largest whole unit, days from 2d on: 7d, 24h, 15m.
func spanString(d time.Duration) string {
	d = d.Round(time.Minute)
	switch {
	case d >= 48*time.Hour && d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0 && d > 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

// recipeHintedError carries recipe pointers on a failed query to the error
// envelope (see agentErrorDetail).
type recipeHintedError struct {
	error
	hints []string
}

func (e *recipeHintedError) Unwrap() error { return e.error }

// withRecipeHints wraps a DQL failure with the recipes that read the same data.
func withRecipeHints(cmd *cobra.Command, cfg *config.Config, query string, err error) error {
	var qe *sdkquery.QueryError
	if err == nil || !errors.As(err, &qe) {
		return err
	}
	if hints := recipeHintsForQuery(cmd, cfg, query, recipeHintFailed); len(hints) > 0 {
		return &recipeHintedError{error: err, hints: hints}
	}
	return err
}
