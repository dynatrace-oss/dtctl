package wait

import (
	"math"
	"time"
)

// BackoffConfig configures the exponential backoff strategy
type BackoffConfig struct {
	MinInterval  time.Duration // Minimum interval between retries
	MaxInterval  time.Duration // Maximum interval between retries
	Multiplier   float64       // Exponential backoff multiplier
	InitialDelay time.Duration // Delay before first attempt
}

// DefaultBackoffConfig returns the default backoff configuration
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		MinInterval:  1 * time.Second,
		MaxInterval:  10 * time.Second,
		Multiplier:   2.0,
		InitialDelay: 0,
	}
}

// CalculateNextInterval calculates the next retry interval using exponential backoff
// The formula is: interval = min(min_interval * (multiplier ^ attempt), max_interval)
// Attempt is zero-indexed (first retry is attempt 0)
func CalculateNextInterval(attempt int, config BackoffConfig) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate exponential backoff
	base := float64(config.MinInterval)
	exponent := math.Pow(config.Multiplier, float64(attempt))
	interval := time.Duration(base * exponent)

	// Cap at max interval and ensure we don't go below min interval
	return min(max(interval, config.MinInterval), config.MaxInterval)
}

// Validate validates the backoff configuration
func (c BackoffConfig) Validate() error {
	if c.MinInterval <= 0 {
		return &ValidationError{Field: "min-interval", Message: "must be greater than 0"}
	}
	if c.MaxInterval <= 0 {
		return &ValidationError{Field: "max-interval", Message: "must be greater than 0"}
	}
	if c.MinInterval > c.MaxInterval {
		return &ValidationError{Field: "min-interval", Message: "must be less than or equal to max-interval"}
	}
	if c.Multiplier <= 1.0 {
		return &ValidationError{Field: "backoff-multiplier", Message: "must be greater than 1.0"}
	}
	if c.InitialDelay < 0 {
		return &ValidationError{Field: "initial-delay", Message: "must be non-negative"}
	}
	return nil
}

// ValidationError represents a validation error for backoff configuration
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
