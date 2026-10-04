package cmd

import (
	"errors"
	"fmt"

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
	load := loadRecipeBook(cmdContext(cmd), recipeEnvSource{cfg: cfg, newClient: NewClientFromConfig})
	if load == nil || load.book == nil {
		return nil
	}
	book := load.book
	var among []*recipes.Recipe
	for _, r := range book.Sorted() {
		if r.Spec.Deprecated == nil {
			among = append(among, r)
		}
	}
	var out []string
	for _, m := range book.MatchQuery(query, among) {
		if len(out) == maxRecipeHints {
			break
		}
		if mode == recipeHintOK && !m.Strong || mode != recipeHintOK && m.Score < 3 {
			continue
		}
		out = append(out, recipeHint(m.Recipe, mode))
	}
	return out
}

func recipeHint(r *recipes.Recipe, mode recipeHintMode) string {
	lead := "recipe for this data"
	switch mode {
	case recipeHintEmpty:
		lead = "a verified recipe for this data (it may know why this came back empty)"
	case recipeHintFailed:
		lead = "a verified recipe for this data, a working starting point"
	}
	return fmt.Sprintf("dtctl run %s  -- %s: %s; its DQL and how to read it: dtctl describe recipe %s",
		r.Name(), lead, r.Spec.Summary, r.Name())
}

// decorateQueryWithRecipes adds recipe pointers to a query envelope.
func decorateQueryWithRecipes(cmd *cobra.Command, cfg *config.Config, query string) func(*output.ResponseContext, *exec.DQLQueryResponse, []map[string]interface{}) {
	return func(ctx *output.ResponseContext, result *exec.DQLQueryResponse, records []map[string]interface{}) {
		mode := recipeHintOK
		if (records != nil && len(records) == 0) || recipePartialCause(result) != "" {
			mode = recipeHintEmpty
		}
		hints := recipeHintsForQuery(cmd, cfg, query, mode)
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
