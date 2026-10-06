package recipes

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// A recipe knows a trap; `dtctl query` tests the agent's own statement against
// every recipe's checks and warns when one fires.

// Check is one trap, as regular expressions over the query (comments
// removed): it fires when every Match matches and no Unless does.
type Check struct {
	Match  []string `json:"match" yaml:"match"`
	Unless []string `json:"unless,omitempty" yaml:"unless,omitempty"`
	// Warn says what goes wrong and how to write it instead, in a sentence or two.
	Warn string `json:"warn" yaml:"warn"`
	// Example is a query the check must fire on (the builtin test runs it).
	Example string `json:"example" yaml:"example"`
}

func (c *Check) compile() (match, unless []*regexp.Regexp, err error) {
	for _, p := range c.Match {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, nil, err
		}
		match = append(match, re)
	}
	for _, p := range c.Unless {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, nil, err
		}
		unless = append(unless, re)
	}
	return match, unless, nil
}

// Fires reports whether the check fires on dql.
func (c *Check) Fires(dql string) bool {
	match, unless, err := c.compile()
	if err != nil || len(match) == 0 {
		return false
	}
	dql = stripDQLComments(dql)
	for _, re := range match {
		if !re.MatchString(dql) {
			return false
		}
	}
	for _, re := range unless {
		if re.MatchString(dql) {
			return false
		}
	}
	return true
}

func validateCheck(i int, c *Check) []error {
	var errs []error
	if len(c.Match) == 0 {
		errs = append(errs, fmt.Errorf("spec.checks[%d]: match needs at least one pattern", i))
	}
	if _, _, err := c.compile(); err != nil {
		errs = append(errs, fmt.Errorf("spec.checks[%d]: %v", i, err))
	}
	if strings.TrimSpace(c.Warn) == "" {
		errs = append(errs, fmt.Errorf("spec.checks[%d]: warn is required", i))
	}
	if strings.TrimSpace(c.Example) == "" {
		errs = append(errs, fmt.Errorf("spec.checks[%d]: example is required (a query the check fires on)", i))
	}
	return errs
}

// lintChecks: a check fires on its example and not on the recipe's own
// statement, which is the trap done right.
func (b *Book) lintChecks(r *Recipe) []string {
	var out []string
	for i := range r.Spec.Checks {
		c := &r.Spec.Checks[i]
		if !c.Fires(c.Example) {
			out = append(out, fmt.Sprintf("checks[%d] does not fire on its example", i))
		}
		if ex, err := b.Example(r, time.Now()); err == nil && c.Fires(ex.DQL) {
			out = append(out, fmt.Sprintf("checks[%d] fires on the recipe's own DQL", i))
		}
	}
	return out
}

// CheckHit is a check that fired on a query.
type CheckHit struct {
	Recipe *Recipe
	Warn   string
}

// QueryChecks returns the checks of among that fire on dql, at most one per
// recipe, in among's order.
func (b *Book) QueryChecks(dql string, among []*Recipe) []CheckHit {
	var out []CheckHit
	for _, r := range among {
		for i := range r.Spec.Checks {
			if c := &r.Spec.Checks[i]; c.Fires(dql) {
				out = append(out, CheckHit{Recipe: r, Warn: strings.TrimSpace(c.Warn)})
				break
			}
		}
	}
	return out
}
