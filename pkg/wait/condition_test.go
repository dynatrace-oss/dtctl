package wait

import (
	"testing"
)

func TestParseCondition(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      Condition
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "simple any",
			input: "any",
			want: Condition{
				Type:     ConditionTypeAny,
				Operator: OpGreater,
				Value:    0,
			},
		},
		{
			name:  "simple none",
			input: "none",
			want: Condition{
				Type:     ConditionTypeNone,
				Operator: OpEqual,
				Value:    0,
			},
		},
		{
			name:  "count equals",
			input: "count=1",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    1,
			},
		},
		{
			name:  "count greater than or equal",
			input: "count-gte=5",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    5,
			},
		},
		{
			name:  "count greater than",
			input: "count-gt=0",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreater,
				Value:    0,
			},
		},
		{
			name:  "count less than or equal",
			input: "count-lte=10",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLessEqual,
				Value:    10,
			},
		},
		{
			name:  "count less than",
			input: "count-lt=100",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLess,
				Value:    100,
			},
		},
		{
			name:  "count with spaces",
			input: " count = 42 ",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    42,
			},
		},
		{
			name:      "empty string",
			input:     "",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:      "invalid format no equals",
			input:     "count",
			wantErr:   true,
			errSubstr: "invalid condition format",
		},
		{
			name:      "invalid value not a number",
			input:     "count=abc",
			wantErr:   true,
			errSubstr: "invalid value",
		},
		{
			name:      "negative value",
			input:     "count=-5",
			wantErr:   true,
			errSubstr: "must be non-negative",
		},
		{
			name:      "unknown condition type",
			input:     "records=5",
			wantErr:   true,
			errSubstr: "unknown condition type",
		},
		{
			name:  "count equals zero",
			input: "count=0",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    0,
			},
		},
		{
			name:  "large count value",
			input: "count-gte=1000000",
			want: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    1000000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCondition(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCondition(%q) expected error containing %q, got nil", tt.input, tt.errSubstr)
					return
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseCondition(%q) error = %v, want error containing %q", tt.input, err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseCondition(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("ParseCondition(%q).Type = %v, want %v", tt.input, got.Type, tt.want.Type)
			}
			if got.Operator != tt.want.Operator {
				t.Errorf("ParseCondition(%q).Operator = %v, want %v", tt.input, got.Operator, tt.want.Operator)
			}
			if got.Value != tt.want.Value {
				t.Errorf("ParseCondition(%q).Value = %v, want %v", tt.input, got.Value, tt.want.Value)
			}
		})
	}
}

func TestConditionEvaluate(t *testing.T) {
	tests := []struct {
		name        string
		condition   Condition
		recordCount int64
		want        bool
	}{
		{
			name: "equal - match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    5,
			},
			recordCount: 5,
			want:        true,
		},
		{
			name: "equal - no match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    5,
			},
			recordCount: 10,
			want:        false,
		},
		{
			name: "greater than or equal - equal",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    5,
			},
			recordCount: 5,
			want:        true,
		},
		{
			name: "greater than or equal - greater",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    5,
			},
			recordCount: 10,
			want:        true,
		},
		{
			name: "greater than or equal - less",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    5,
			},
			recordCount: 3,
			want:        false,
		},
		{
			name: "greater than - match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreater,
				Value:    5,
			},
			recordCount: 10,
			want:        true,
		},
		{
			name: "greater than - equal no match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreater,
				Value:    5,
			},
			recordCount: 5,
			want:        false,
		},
		{
			name: "less than or equal - equal",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLessEqual,
				Value:    10,
			},
			recordCount: 10,
			want:        true,
		},
		{
			name: "less than or equal - less",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLessEqual,
				Value:    10,
			},
			recordCount: 5,
			want:        true,
		},
		{
			name: "less than or equal - greater",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLessEqual,
				Value:    10,
			},
			recordCount: 15,
			want:        false,
		},
		{
			name: "less than - match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLess,
				Value:    10,
			},
			recordCount: 5,
			want:        true,
		},
		{
			name: "less than - equal no match",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLess,
				Value:    10,
			},
			recordCount: 10,
			want:        false,
		},
		{
			name: "any - with records",
			condition: Condition{
				Type:     ConditionTypeAny,
				Operator: OpGreater,
				Value:    0,
			},
			recordCount: 1,
			want:        true,
		},
		{
			name: "any - no records",
			condition: Condition{
				Type:     ConditionTypeAny,
				Operator: OpGreater,
				Value:    0,
			},
			recordCount: 0,
			want:        false,
		},
		{
			name: "none - no records",
			condition: Condition{
				Type:     ConditionTypeNone,
				Operator: OpEqual,
				Value:    0,
			},
			recordCount: 0,
			want:        true,
		},
		{
			name: "none - with records",
			condition: Condition{
				Type:     ConditionTypeNone,
				Operator: OpEqual,
				Value:    0,
			},
			recordCount: 1,
			want:        false,
		},
		{
			name: "zero count with equal",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    0,
			},
			recordCount: 0,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.condition.Evaluate(tt.recordCount)
			if got != tt.want {
				t.Errorf("Condition.Evaluate(%d) = %v, want %v", tt.recordCount, got, tt.want)
			}
		})
	}
}

func TestConditionString(t *testing.T) {
	tests := []struct {
		name      string
		condition Condition
		want      string
	}{
		{
			name: "any",
			condition: Condition{
				Type:     ConditionTypeAny,
				Operator: OpGreater,
				Value:    0,
			},
			want: "any",
		},
		{
			name: "none",
			condition: Condition{
				Type:     ConditionTypeNone,
				Operator: OpEqual,
				Value:    0,
			},
			want: "none",
		},
		{
			name: "count equals",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpEqual,
				Value:    1,
			},
			want: "count=1",
		},
		{
			name: "count-gte",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreaterEqual,
				Value:    5,
			},
			want: "count-gte=5",
		},
		{
			name: "count-gt",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpGreater,
				Value:    0,
			},
			want: "count-gt=0",
		},
		{
			name: "count-lte",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLessEqual,
				Value:    10,
			},
			want: "count-lte=10",
		},
		{
			name: "count-lt",
			condition: Condition{
				Type:     ConditionTypeCount,
				Operator: OpLess,
				Value:    100,
			},
			want: "count-lt=100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.condition.String()
			if got != tt.want {
				t.Errorf("Condition.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestConditionEvaluate_UnknownOperator(t *testing.T) {
	// Test with an unknown operator - should return false
	c := Condition{
		Type:     ConditionTypeCount,
		Operator: Operator("!="), // Invalid operator
		Value:    5,
	}

	if c.Evaluate(5) {
		t.Error("Evaluate with unknown operator should return false")
	}
	if c.Evaluate(10) {
		t.Error("Evaluate with unknown operator should return false")
	}
}

func TestConditionString_UnknownType(t *testing.T) {
	// Test String() with unknown type
	c := Condition{
		Type:     ConditionType("unknown"),
		Operator: OpEqual,
		Value:    5,
	}

	result := c.String()
	// Should fallback to default format
	if result == "" {
		t.Error("String() should return non-empty for unknown type")
	}
}

func TestConditionString_UnknownOperatorInCount(t *testing.T) {
	// Test String() with count type but unknown operator
	c := Condition{
		Type:     ConditionTypeCount,
		Operator: Operator("!="),
		Value:    5,
	}

	result := c.String()
	// Should fallback to default format showing the operator
	if result == "" {
		t.Error("String() should return non-empty for unknown operator")
	}
	if !contains(result, "!=") {
		t.Errorf("String() should contain operator, got %q", result)
	}
}

func TestParseCondition_Whitespace(t *testing.T) {
	// Test various whitespace scenarios
	tests := []struct {
		input    string
		wantType ConditionType
		wantErr  bool
	}{
		{"  any  ", ConditionTypeAny, false},
		{"\tnone\t", ConditionTypeNone, false},
		{" count = 10 ", ConditionTypeCount, false},
		{"   ", "", true}, // Only whitespace should fail
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCondition(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for whitespace-only input")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", got.Type, tt.wantType)
			}
		})
	}
}
