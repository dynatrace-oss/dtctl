package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
)

// Recipe pointers in `dtctl query` envelopes. Next to a fine-looking result only a
// specific match is named; an empty, cut-short or failed query admits a looser one.

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
	if !recipesEnabled() {
		return nil
	}
	load := loadRecipeBook()
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

// queryRecipeHints names recipes matching query as runnable commands, with the
// params, scope and window the query names filled in (see recipes.BindQuery).
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

// queryRecipeWarnings tests a query against the recipes' checks and the DQL trap lints.
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
