package recipes

import (
	"fmt"
	"strings"
	"time"
)

// Window is a resolved query window: absolute instants, so the reported window
// is exact and every request of an invocation sees the same one.
type Window struct {
	From time.Time
	To   time.Time
}

// FromRFC3339 and ToRFC3339 format the window as the query API's
// defaultTimeframeStart/End expect.
func (w Window) FromRFC3339() string { return w.From.UTC().Format(time.RFC3339) }
func (w Window) ToRFC3339() string   { return w.To.UTC().Format(time.RFC3339) }

// ParseInstant reads a --from/--to value: a duration read as "ago" ("2h",
// "7d") or an RFC3339 timestamp. now is the reference instant for durations.
func ParseInstant(s string, now time.Time) (time.Time, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty time value")
	}
	if strings.EqualFold(v, "now") {
		return now, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	// "-2h" is how DQL and the old flags spell it; accept it as the same thing.
	d, err := ParseDuration(strings.TrimPrefix(v, "-"))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: use a duration ago (2h, 7d) or an RFC3339 timestamp", s)
	}
	return now.Add(-d), nil
}

// ResolveWindow applies a recipe's timeframe policy to --from/--to.
// It returns nil for a `timeframe: none` recipe, which sends no window.
func ResolveWindow(tf Timeframe, from, to string, now time.Time) (*Window, error) {
	if tf.None {
		if from != "" || to != "" {
			return nil, fmt.Errorf("this recipe queries current state and takes no --from/--to")
		}
		return nil, nil
	}
	if tf.Fixed && (from != "" || to != "") {
		return nil, fmt.Errorf("this recipe is a snapshot of current state over a fixed %s window; --from/--to are not supported", FormatDuration(tf.Default))
	}
	w := Window{To: now}
	if to != "" {
		t, err := ParseInstant(to, now)
		if err != nil {
			return nil, fmt.Errorf("--to: %w", err)
		}
		w.To = t
	}
	if from != "" {
		t, err := ParseInstant(from, now)
		if err != nil {
			return nil, fmt.Errorf("--from: %w", err)
		}
		w.From = t
	} else {
		w.From = w.To.Add(-tf.Default)
	}
	if tf.Align == AlignUTCDay {
		w.From = w.From.UTC().Truncate(24 * time.Hour)
		w.To = w.To.UTC().Truncate(24 * time.Hour)
	}
	if !w.From.Before(w.To) {
		if tf.Align == AlignUTCDay {
			return nil, fmt.Errorf("the window %s..%s holds no complete UTC day", w.FromRFC3339(), w.ToRFC3339())
		}
		return nil, fmt.Errorf("--from (%s) must be before --to (%s)", w.FromRFC3339(), w.ToRFC3339())
	}
	if tf.Min > 0 && w.To.Sub(w.From) < tf.Min {
		return nil, fmt.Errorf("this recipe needs at least %s of history; the window is %s (use --from %s or earlier)",
			FormatDuration(tf.Min), FormatDuration(w.To.Sub(w.From)), FormatDuration(tf.Min))
	}
	if tf.Max > 0 && w.To.Sub(w.From) > tf.Max {
		return nil, fmt.Errorf("this recipe allows at most %s (the window is %s); for a longer one, run its query with dtctl query (see --dry-run)",
			FormatDuration(tf.Max), FormatDuration(w.To.Sub(w.From)))
	}
	return &w, nil
}

// ResolveQueryWindow resolves `dtctl query --from/--to`: unlike a recipe there
// is no default length, so --to alone means "the server's default length,
// ending then" is not expressible — it requires --from.
func ResolveQueryWindow(from, to string, now time.Time) (*Window, error) {
	if from == "" && to == "" {
		return nil, nil
	}
	if from == "" {
		return nil, fmt.Errorf("--to requires --from")
	}
	return ResolveWindow(Timeframe{Declared: true}, from, to, now)
}

// timestampExpr is a window bound as handed to a template: it prints as a DQL
// timestamp expression, and timeAdd shifts it.
type timestampExpr time.Time

func (t timestampExpr) String() string {
	return fmt.Sprintf("toTimestamp(%q)", time.Time(t).UTC().Format(time.RFC3339))
}

// timeAdd is the template function that shifts a window bound by a duration
// ("2h", "-1d").
func timeAdd(t timestampExpr, d string) (timestampExpr, error) {
	neg := strings.HasPrefix(d, "-")
	dur, err := ParseDuration(strings.TrimPrefix(d, "-"))
	if err != nil {
		return t, fmt.Errorf("timeAdd: %w", err)
	}
	if neg {
		dur = -dur
	}
	return timestampExpr(time.Time(t).Add(dur)), nil
}
