package recipes

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Matching an ad-hoc query to recipes is how an agent that never asked for one meets one.
// The match is structural: a recipe must share a data source or metric key with the query;
// distinctive shared fields decide strength, common ones count little.

// QueryMatch is a recipe that reads what a query reads.
type QueryMatch struct {
	Recipe *Recipe
	Score  int
	// Strong marks a match specific enough to name next to a working query.
	Strong bool
}

type querySig struct {
	sources map[string]bool
	metrics map[string]bool
	fields  map[string]bool
	// terms are the words of the statement's identifiers and functions, matched against recipe names and summaries.
	terms []string
	// params maps a field the recipe compares with a param to that param (signatures only).
	params map[string]string
}

// dqlWords are the language's own words, which say nothing about the question.
var dqlWords = map[string]bool{
	"fetch": true, "filter": true, "filterout": true, "summarize": true, "fields": true, "fieldsadd": true,
	"fieldskeep": true, "fieldsremove": true, "fieldsrename": true, "sort": true, "limit": true, "desc": true,
	"asc": true, "by": true, "timeseries": true, "maketimeseries": true, "dt": true, "true": true, "false": true,
	"count": true, "sum": true, "avg": true, "min": true, "max": true, "isnotnull": true, "isnull": true,
	"if": true, "else": true, "not": true, "and": true, "or": true, "in": true, "from": true, "to": true,
	"now": true, "toString": true, "tostring": true, "name": true, "id": true, "value": true, "lookup": true,
	"parse": true, "append": true, "join": true, "dedup": true, "expand": true, "data": true, "interval": true,
	"scalar": true, "round": true, "decimals": true, "arraysum": true, "arrayavg": true, "smartscape": true,
	"smartscapenodes": true, "smartscapeedges": true, "takefirst": true, "takelast": true, "countif": true,
	"countdistinct": true, "countdistinctexact": true, "contains": true, "matchesphrase": true, "lower": true,
	"system": true, "bucket": true, "start": true, "end": true, "timestamp": true, "entity": true,
}

var sigWord = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_.]*`)

var (
	sigFetch      = regexp.MustCompile(`(?m)(?:^|\[|\|)\s*fetch\s+([A-Za-z_][\w.]*)`)
	sigSmartscape = regexp.MustCompile(`smartscape(?:Nodes|Edges)\s*\(?\s*"?([A-Z_][A-Z0-9_]*)?`)
	sigTimeseries = regexp.MustCompile(`(?m)(?:^|\[|\|)\s*timeseries\b`)
	sigMetricArg  = regexp.MustCompile(`\b(?:avg|sum|min|max|count|percentile|median|start|end)\(\s*([a-z][\w.:-]*\.[\w.:-]+)`)
	sigField      = regexp.MustCompile(`\b[a-z_][a-z0-9_]*(?:\.[a-z0-9_]+)+\b`)
	sigString     = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

// commonFields name the shape of almost every record, not the question.
var commonFields = map[string]bool{
	"dt.system.bucket": true, "dt.system.sampling_ratio": true, "dt.security_context": true,
}

func signatureOf(dql string) querySig {
	s := querySig{sources: map[string]bool{}, metrics: map[string]bool{}, fields: map[string]bool{}}
	dql = sigString.ReplaceAllString(stripDQLComments(dql), `""`)
	fetched := map[string]bool{}
	for _, m := range sigFetch.FindAllStringSubmatch(dql, -1) {
		s.sources["fetch "+m[1]] = true
		fetched[m[1]] = true
	}
	for _, m := range sigSmartscape.FindAllStringSubmatch(dql, -1) {
		s.sources["smartscape "+m[1]] = true
	}
	if sigTimeseries.MatchString(dql) {
		for _, m := range sigMetricArg.FindAllStringSubmatch(dql, -1) {
			s.metrics[m[1]] = true
			s.sources["metric "+m[1]] = true
		}
	}
	seen := map[string]bool{}
	for _, w := range sigWord.FindAllString(dql, -1) {
		for _, t := range tokenize(strings.NewReplacer(".", " ", "_", " ").Replace(w)) {
			if !dqlWords[t] && len(t) > 2 && !seen[t] {
				seen[t] = true
				s.terms = append(s.terms, t)
			}
		}
	}
	for _, f := range sigField.FindAllString(dql, -1) {
		if !s.metrics[f] && !fetched[f] && !commonFields[f] {
			s.fields[f] = true
		}
	}
	return s
}

// querySigs caches the recipes' signatures for one book.
type querySigs struct {
	sigs map[string]querySig
	df   map[string]int
}

func (b *Book) recipeSigs() *querySigs {
	b.sigsOnce.Do(func() {
		qs := &querySigs{sigs: map[string]querySig{}, df: map[string]int{}}
		now := time.Now()
		for _, r := range b.Sorted() {
			rendered, err := b.Example(r, now)
			if err != nil {
				continue
			}
			sig := signatureOf(rendered.DQL)
			sig.params = paramFields(rendered.DQL)
			qs.sigs[r.Name()] = sig
			for f := range sig.fields {
				qs.df[f]++
			}
		}
		b.sigs = qs
	})
	return b.sigs
}

// MatchQuery ranks the recipes among that read what dql reads, best first.
// Only recipes sharing a data source with the query are considered.
func (b *Book) MatchQuery(dql string, among []*Recipe) []QueryMatch {
	q := signatureOf(dql)
	if len(q.sources) == 0 {
		return nil
	}
	qs := b.recipeSigs()
	n := len(qs.sigs)
	var out []QueryMatch
	for _, r := range among {
		sig, ok := qs.sigs[r.Name()]
		if !ok {
			continue
		}
		shared := 0
		for src := range q.sources {
			if sig.sources[src] {
				shared++
			}
		}
		score, strong := 1, false
		if shared == 0 {
			// A source no recipe reads can still match by name words
			// (interaction_to_next_paint ~ frontends-web-vitals), never strongly.
			words := 0
			name := tokenize(strings.ReplaceAll(r.Name(), "-", " "))
			for _, t := range q.terms {
				if anyTermMatch(t, name) {
					words++
				}
			}
			if words >= 2 {
				out = append(out, QueryMatch{Recipe: r, Score: words})
			}
			continue
		}
		if b.BindQuery(r, dql, "").Subject {
			// The query looks up what the recipe takes (a problem ID, a service).
			score += 3
			strong = true
		}
		for m := range q.metrics {
			if sig.metrics[m] {
				score += 3
				strong = true
			}
		}
		distinctive := 0
		for f := range q.fields {
			if !sig.fields[f] {
				continue
			}
			switch df := qs.df[f]; {
			case df <= 3:
				score += 2
				distinctive++
			case df*4 <= n:
				score++
			}
		}
		// Query words against the recipe's question ("failed" meets services-failures);
		// each recipe word counts once.
		words := 0
		name := tokenize(strings.ReplaceAll(r.Name(), "-", " "))
		about := append(lower(r.Metadata.Tags), tokenize(r.Spec.Summary)...)
		hit := map[string]bool{}
		for _, t := range q.terms {
			if w := firstTermMatch(t, name); w != "" {
				if !hit[w] {
					hit[w] = true
					words += 2
				}
			} else if w := firstTermMatch(t, about); w != "" && !hit[w] {
				hit[w] = true
				words++
			}
		}
		score += words
		if distinctive >= 2 || (distinctive >= 1 && strong) || words >= 4 {
			strong = true
		}
		out = append(out, QueryMatch{Recipe: r, Score: score, Strong: strong})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Recipe.Name() < out[j].Recipe.Name()
	})
	return out
}

func firstTermMatch(t string, words []string) string {
	for _, w := range words {
		if termMatches(t, w) {
			return w
		}
	}
	return ""
}

func anyTermMatch(t string, words []string) bool {
	for _, w := range words {
		if termMatches(t, w) {
			return true
		}
	}
	return false
}
