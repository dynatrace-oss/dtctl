package recipes

import (
	"sort"
	"strings"
	"unicode"
)

// Match is a recipe with its search score. Weak marks a match that names no
// query term in its name, tags or summary — it only mentions one in passing,
// so it is not "a recipe about" the query.
type Match struct {
	Recipe *Recipe
	Score  int
	Weak   bool
}

// Search ranks recipes by keyword overlap with the query: name, tags and
// summary weigh most, the description and the domain's description less. It
// is a local ranking — no embeddings, no network — and good enough because
// summaries are written as the question the recipe answers.
//
// A term most recipes contain ("service", "count") ranks little, and the
// tail of matches scoring under half the best is dropped: a search answers
// "which recipe is for this", and twenty loosely related names answer it
// worse than three. Matches all weak, or none, means no recipe is about it.
func (b *Book) Search(query string, among []*Recipe) []Match {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	docs := make([][]searchField, len(among))
	df := make([]int, len(terms))
	for i, r := range among {
		fields := []searchField{
			{tokenize(strings.ReplaceAll(r.Name(), "-", " ")), 4},
			{lower(r.Metadata.Tags), 3},
			{tokenize(r.Spec.Summary), 3},
			{tokenize(r.Spec.Description), 1},
			{tokenize(r.Spec.Means), 1},
		}
		if d := b.Domains[r.Domain()]; d != nil {
			fields = append(fields, searchField{tokenize(d.Description), 1})
		}
		docs[i] = fields
		for j, t := range terms {
			if bestWeight(t, fields) > 0 {
				df[j]++
			}
		}
	}
	var out []Match
	for i, r := range among {
		score, hit, strong := 0, 0, 0
		for j, t := range terms {
			best := bestWeight(t, docs[i])
			if best == 0 {
				continue
			}
			hit++
			if best >= 3 {
				strong++
			}
			// A term in more than a third of the recipes tells them apart
			// little: half weight.
			if df[j]*3 > len(among) {
				score += best
			} else {
				score += 2 * best
			}
		}
		if hit == 0 {
			continue
		}
		// Recipes naming every term up front rank above any partial match.
		if strong == len(terms) {
			score += 100
		}
		out = append(out, Match{Recipe: r, Score: score, Weak: strong == 0})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Recipe.Name() < out[j].Recipe.Name()
	})
	if len(out) > 0 {
		top := out[0].Score
		keep := out[:0]
		for _, m := range out {
			if m.Score*2 >= top {
				keep = append(keep, m)
			}
		}
		out = keep
	}
	if len(out) > maxSearchResults {
		out = out[:maxSearchResults]
	}
	return out
}

// maxSearchResults bounds a search: past it, a list stops answering "which
// recipe" and becomes the catalog again.
const maxSearchResults = 8

// searchField is one weighted text of a recipe.
type searchField struct {
	words  []string
	weight int
}

func bestWeight(term string, fields []searchField) int {
	best := 0
	for _, f := range fields {
		for _, w := range f.words {
			if termMatches(term, w) && f.weight > best {
				best = f.weight
			}
		}
	}
	return best
}

// termMatches compares with a crude stem: "restart" matches "restarts",
// "failing" matches "failures" through the shared 4+ letter prefix "fail".
func termMatches(term, word string) bool {
	if term == word {
		return true
	}
	n := commonPrefix(term, word)
	short, long := len(term), len(word)
	if short > long {
		short, long = long, short
	}
	// Plurals of short words: "pod"/"pods", "log"/"logs".
	if n == short && n >= 3 && long-short <= 2 {
		return true
	}
	return n >= 4 && (n == len(term) || n == len(word) || n >= len(term)-3)
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "on": true, "for": true,
	"and": true, "or": true, "is": true, "are": true, "to": true, "with": true, "by": true,
	"which": true, "what": true, "my": true, "me": true, "show": true, "list": true, "all": true,
	"how": true, "many": true, "do": true, "does": true, "that": true, "this": true,
}

func tokenize(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := words[:0]
	for _, w := range words {
		if !stopwords[w] {
			out = append(out, w)
		}
	}
	return out
}

func lower(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// HasTag reports whether the recipe carries the tag (case-insensitive).
func (r *Recipe) HasTag(tag string) bool {
	for _, t := range r.Metadata.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
