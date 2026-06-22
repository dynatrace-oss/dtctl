package exec

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SpillMode is the tri-state core spill control (D4).
type SpillMode string

const (
	// SpillNever forces rows inline regardless of size (the bare-command
	// default; the explicit "give me rows anyway" override).
	SpillNever SpillMode = "never"
	// SpillAuto spills above the threshold and stays inline below (the agent
	// default).
	SpillAuto SpillMode = "auto"
	// SpillAlways always spills (bare --spill, and implied by --spill-to).
	SpillAlways SpillMode = "always"
)

// DefaultSpillThresholdBytes is the default serialised-output size above which a
// result spills (D5). 50 KB ≈ a screenful, well under any model's budget and
// above an ordinary capped result.
const DefaultSpillThresholdBytes int64 = 50 * 1024

// SpillOptions are the fully-resolved spill settings threaded into a query
// execution. Resolution (flag → env → context-config → global-config → default,
// D15) happens in the command layer; by the time it reaches here every field is
// concrete.
type SpillOptions struct {
	Mode SpillMode
	// ToPath is an explicit caller-chosen destination (--spill-to). When set the
	// file is written exactly there, the format is inferred from the extension,
	// and the location opts out of the managed privacy guarantees (D25).
	ToPath string
	// Dir is a preferred managed base dir (config spill.dir / DTCTL_SPILL_DIR).
	// Empty means use the OS user cache dir default (D7). A non-empty Dir is a
	// user-chosen location and also opts out of managed privacy (D25).
	Dir string
	// Format is the spill file format (json|csv for PR2; ndjson/parquet once the
	// dedicated writers land). Ignored when ToPath sets it via extension.
	Format    string
	Threshold int64
	TTL       time.Duration
}

// Enabled reports whether spilling can occur at all (mode is not never).
func (o SpillOptions) Enabled() bool {
	return o.Mode == SpillAuto || o.Mode == SpillAlways
}

// ParseByteSize parses a human byte size such as "50KB", "50k", "1MB", "1.5MiB"
// or a bare byte count "51200". All unit suffixes — KB/MB/GB, KiB/MiB/GiB, and
// bare K/M/G — are treated as 1024-based (binary). This keeps the threshold
// consistent with the byte counts the agent envelope reports (e.g. "50KB" ==
// 51200 bytes, matching threshold_bytes in the spill manifest).
func ParseByteSize(s string) (int64, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return 0, fmt.Errorf("empty size")
	}
	up := strings.ToUpper(in)

	type unit struct {
		suffix string
		mult   float64
	}
	// Order matters: check longer suffixes first.
	units := []unit{
		{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30},
		{"KB", 1 << 10}, {"MB", 1 << 20}, {"GB", 1 << 30},
		{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(up, u.suffix) {
			numPart := strings.TrimSpace(up[:len(up)-len(u.suffix)])
			f, err := strconv.ParseFloat(numPart, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", s, err)
			}
			if f < 0 {
				return 0, fmt.Errorf("invalid size %q: must not be negative", s)
			}
			return int64(f * u.mult), nil
		}
	}
	// Bare number = bytes.
	n, err := strconv.ParseInt(up, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid size %q: must not be negative", s)
	}
	return n, nil
}
