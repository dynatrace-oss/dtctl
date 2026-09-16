package engine

import (
	"context"
	"time"
)

// Limits controls resource budgets for a single engine execution.
type Limits struct {
	MaxQueued      int
	MaxDuration    time.Duration
	MaxOutputBytes int64
	MaxFileBytes   int64
}

// DefaultLimits returns conservative defaults for production use.
func DefaultLimits() Limits {
	return Limits{
		MaxQueued:      4,
		MaxDuration:    5 * time.Minute,
		MaxOutputBytes: 32 * 1024 * 1024, // 32 MiB
		MaxFileBytes:   16 * 1024 * 1024, // 16 MiB
	}
}

// ExecuteWithLimits runs one dtctl invocation with explicit resource budgets.
// The limits govern queue depth, duration (via context timeout on the command
// tree), and stdout/stderr size (via capped buffers).
//
// Note: the queue counter (engineQueued) and execution slot (engineSlot) are
// process-global. When two callers use different Limits, MaxQueued is evaluated
// against the shared counter — effectively the stricter of the two limits wins.
// If you embed multiple independent engines in one process, use separate
// processes or accept this shared-budget behaviour.
func ExecuteWithLimits(ctx context.Context, req Request, limits Limits) (*Result, error) {
	return executeInner(ctx, req, limits)
}
