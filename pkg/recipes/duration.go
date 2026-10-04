package recipes

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationPart = regexp.MustCompile(`^(\d+)(w|d|h|m|s)`)

// ParseDuration parses a window length: Go durations extended by days (d) and
// weeks (w), the units a query window is actually written in ("7d", "2w",
// "1d12h"). Negative values and fractions are rejected — a window length is a
// positive count.
func ParseDuration(s string) (time.Duration, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return 0, fmt.Errorf("empty duration")
	}
	rest := in
	var total time.Duration
	for rest != "" {
		m := durationPart.FindStringSubmatch(rest)
		if m == nil {
			return 0, fmt.Errorf("invalid duration %q (use e.g. 30m, 2h, 7d, 2w)", s)
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		var unit time.Duration
		switch m[2] {
		case "w":
			unit = 7 * 24 * time.Hour
		case "d":
			unit = 24 * time.Hour
		case "h":
			unit = time.Hour
		case "m":
			unit = time.Minute
		case "s":
			unit = time.Second
		}
		total += time.Duration(n) * unit
		rest = rest[len(m[0]):]
	}
	if total <= 0 {
		return 0, fmt.Errorf("invalid duration %q: must be positive", s)
	}
	return total, nil
}

// FormatDuration renders a duration in the largest whole units, the way a
// recipe author writes it: 168h → "7d", 90m → "1h30m".
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	var b strings.Builder
	units := []struct {
		suffix string
		size   time.Duration
	}{{"d", 24 * time.Hour}, {"h", time.Hour}, {"m", time.Minute}, {"s", time.Second}}
	for _, u := range units {
		if n := d / u.size; n > 0 {
			fmt.Fprintf(&b, "%d%s", n, u.suffix)
			d -= n * u.size
		}
	}
	return b.String()
}
