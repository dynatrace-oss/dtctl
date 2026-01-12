package wait

import (
	"testing"
	"time"
)

func TestCalculateNextInterval(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		config  BackoffConfig
		want    time.Duration
	}{
		{
			name:    "attempt 0 with default config",
			attempt: 0,
			config:  DefaultBackoffConfig(),
			want:    1 * time.Second,
		},
		{
			name:    "attempt 1 with default config",
			attempt: 1,
			config:  DefaultBackoffConfig(),
			want:    2 * time.Second,
		},
		{
			name:    "attempt 2 with default config",
			attempt: 2,
			config:  DefaultBackoffConfig(),
			want:    4 * time.Second,
		},
		{
			name:    "attempt 3 with default config",
			attempt: 3,
			config:  DefaultBackoffConfig(),
			want:    8 * time.Second,
		},
		{
			name:    "attempt 4 with default config - capped at max",
			attempt: 4,
			config:  DefaultBackoffConfig(),
			want:    10 * time.Second, // 16s capped to 10s
		},
		{
			name:    "attempt 10 with default config - still capped",
			attempt: 10,
			config:  DefaultBackoffConfig(),
			want:    10 * time.Second,
		},
		{
			name:    "custom multiplier 1.5",
			attempt: 3,
			config: BackoffConfig{
				MinInterval: 1 * time.Second,
				MaxInterval: 30 * time.Second,
				Multiplier:  1.5,
			},
			want: 3375 * time.Millisecond, // 1 * 1.5^3 = 3.375s
		},
		{
			name:    "custom min interval",
			attempt: 0,
			config: BackoffConfig{
				MinInterval: 5 * time.Second,
				MaxInterval: 60 * time.Second,
				Multiplier:  2.0,
			},
			want: 5 * time.Second,
		},
		{
			name:    "custom max interval - immediate cap",
			attempt: 5,
			config: BackoffConfig{
				MinInterval: 1 * time.Second,
				MaxInterval: 10 * time.Second,
				Multiplier:  2.0,
			},
			want: 10 * time.Second, // 32s capped to 10s
		},
		{
			name:    "negative attempt treated as zero",
			attempt: -1,
			config:  DefaultBackoffConfig(),
			want:    1 * time.Second,
		},
		{
			name:    "fast backoff for CI/CD",
			attempt: 2,
			config: BackoffConfig{
				MinInterval: 500 * time.Millisecond,
				MaxInterval: 15 * time.Second,
				Multiplier:  1.5,
			},
			want: 1125 * time.Millisecond, // 0.5 * 1.5^2 = 1.125s
		},
		{
			name:    "conservative backoff",
			attempt: 3,
			config: BackoffConfig{
				MinInterval: 10 * time.Second,
				MaxInterval: 2 * time.Minute,
				Multiplier:  2.0,
			},
			want: 80 * time.Second, // 10 * 2^3 = 80s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateNextInterval(tt.attempt, tt.config)
			if got != tt.want {
				t.Errorf("CalculateNextInterval(%d, %+v) = %v, want %v", tt.attempt, tt.config, got, tt.want)
			}
		})
	}
}

func TestBackoffConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    BackoffConfig
		wantErr   bool
		errField  string
		errSubstr string
	}{
		{
			name:    "valid default config",
			config:  DefaultBackoffConfig(),
			wantErr: false,
		},
		{
			name: "valid custom config",
			config: BackoffConfig{
				MinInterval:  500 * time.Millisecond,
				MaxInterval:  1 * time.Minute,
				Multiplier:   1.5,
				InitialDelay: 10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "zero min interval",
			config: BackoffConfig{
				MinInterval: 0,
				MaxInterval: 30 * time.Second,
				Multiplier:  2.0,
			},
			wantErr:   true,
			errField:  "min-interval",
			errSubstr: "greater than 0",
		},
		{
			name: "negative min interval",
			config: BackoffConfig{
				MinInterval: -1 * time.Second,
				MaxInterval: 30 * time.Second,
				Multiplier:  2.0,
			},
			wantErr:   true,
			errField:  "min-interval",
			errSubstr: "greater than 0",
		},
		{
			name: "zero max interval",
			config: BackoffConfig{
				MinInterval: 1 * time.Second,
				MaxInterval: 0,
				Multiplier:  2.0,
			},
			wantErr:   true,
			errField:  "max-interval",
			errSubstr: "greater than 0",
		},
		{
			name: "min greater than max",
			config: BackoffConfig{
				MinInterval: 60 * time.Second,
				MaxInterval: 30 * time.Second,
				Multiplier:  2.0,
			},
			wantErr:   true,
			errField:  "min-interval",
			errSubstr: "less than or equal to max-interval",
		},
		{
			name: "multiplier too low",
			config: BackoffConfig{
				MinInterval: 1 * time.Second,
				MaxInterval: 30 * time.Second,
				Multiplier:  1.0,
			},
			wantErr:   true,
			errField:  "backoff-multiplier",
			errSubstr: "greater than 1.0",
		},
		{
			name: "multiplier zero",
			config: BackoffConfig{
				MinInterval: 1 * time.Second,
				MaxInterval: 30 * time.Second,
				Multiplier:  0,
			},
			wantErr:   true,
			errField:  "backoff-multiplier",
			errSubstr: "greater than 1.0",
		},
		{
			name: "negative initial delay",
			config: BackoffConfig{
				MinInterval:  1 * time.Second,
				MaxInterval:  30 * time.Second,
				Multiplier:   2.0,
				InitialDelay: -5 * time.Second,
			},
			wantErr:   true,
			errField:  "initial-delay",
			errSubstr: "non-negative",
		},
		{
			name: "min equals max is valid",
			config: BackoffConfig{
				MinInterval: 10 * time.Second,
				MaxInterval: 10 * time.Second,
				Multiplier:  2.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("BackoffConfig.Validate() expected error, got nil")
					return
				}
				validationErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("BackoffConfig.Validate() expected ValidationError, got %T", err)
					return
				}
				if tt.errField != "" && validationErr.Field != tt.errField {
					t.Errorf("BackoffConfig.Validate() error field = %q, want %q", validationErr.Field, tt.errField)
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("BackoffConfig.Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("BackoffConfig.Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultBackoffConfig(t *testing.T) {
	config := DefaultBackoffConfig()

	if config.MinInterval != 1*time.Second {
		t.Errorf("DefaultBackoffConfig().MinInterval = %v, want %v", config.MinInterval, 1*time.Second)
	}
	if config.MaxInterval != 10*time.Second {
		t.Errorf("DefaultBackoffConfig().MaxInterval = %v, want %v", config.MaxInterval, 10*time.Second)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("DefaultBackoffConfig().Multiplier = %v, want %v", config.Multiplier, 2.0)
	}
	if config.InitialDelay != 0 {
		t.Errorf("DefaultBackoffConfig().InitialDelay = %v, want %v", config.InitialDelay, 0)
	}

	// Ensure default config is valid
	if err := config.Validate(); err != nil {
		t.Errorf("DefaultBackoffConfig().Validate() = %v, want nil", err)
	}
}

func TestBackoffSequence(t *testing.T) {
	// Test the backoff sequence matches the documented example
	config := DefaultBackoffConfig()
	expected := []time.Duration{
		1 * time.Second,  // attempt 0
		2 * time.Second,  // attempt 1
		4 * time.Second,  // attempt 2
		8 * time.Second,  // attempt 3
		10 * time.Second, // attempt 4 (capped at 10s)
		10 * time.Second, // attempt 5 (capped)
		10 * time.Second, // attempt 6 (capped)
	}

	for i, want := range expected {
		got := CalculateNextInterval(i, config)
		if got != want {
			t.Errorf("Attempt %d: got %v, want %v", i, got, want)
		}
	}
}
