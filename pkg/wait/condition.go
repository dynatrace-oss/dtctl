package wait

import (
	"fmt"
	"strconv"
	"strings"
)

// Condition represents a success condition for waiting
type Condition struct {
	Type     ConditionType
	Operator Operator
	Value    int64
}

// ConditionType defines the type of condition
type ConditionType string

const (
	ConditionTypeCount ConditionType = "count"
	ConditionTypeAny   ConditionType = "any"
	ConditionTypeNone  ConditionType = "none"
)

// Operator defines the comparison operator
type Operator string

const (
	OpEqual        Operator = "=="
	OpGreaterEqual Operator = ">="
	OpGreater      Operator = ">"
	OpLessEqual    Operator = "<="
	OpLess         Operator = "<"
)

// ParseCondition parses a condition string in kubectl-like format
// Supported formats:
//   - count=N         (exactly N records)
//   - count-gte=N     (at least N records, >=)
//   - count-gt=N      (more than N records, >)
//   - count-lte=N     (at most N records, <=)
//   - count-lt=N      (fewer than N records, <)
//   - any             (any records, count > 0)
//   - none            (no records, count == 0)
func ParseCondition(s string) (Condition, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Condition{}, fmt.Errorf("condition cannot be empty")
	}

	// Handle simple conditions
	if s == "any" {
		return Condition{
			Type:     ConditionTypeAny,
			Operator: OpGreater,
			Value:    0,
		}, nil
	}

	if s == "none" {
		return Condition{
			Type:     ConditionTypeNone,
			Operator: OpEqual,
			Value:    0,
		}, nil
	}

	// Parse count-based conditions
	// Format: count=N, count-gte=N, count-gt=N, count-lte=N, count-lt=N
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return Condition{}, fmt.Errorf("invalid condition format: %q (expected format: count=N, count-gte=N, any, none)", s)
	}

	key := strings.TrimSpace(parts[0])
	valueStr := strings.TrimSpace(parts[1])

	// Parse the value
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return Condition{}, fmt.Errorf("invalid value %q: must be an integer", valueStr)
	}

	if value < 0 {
		return Condition{}, fmt.Errorf("invalid value %d: must be non-negative", value)
	}

	// Parse the key to determine operator
	var operator Operator
	switch key {
	case "count":
		operator = OpEqual
	case "count-gte":
		operator = OpGreaterEqual
	case "count-gt":
		operator = OpGreater
	case "count-lte":
		operator = OpLessEqual
	case "count-lt":
		operator = OpLess
	default:
		return Condition{}, fmt.Errorf("unknown condition type: %q (supported: count, count-gte, count-gt, count-lte, count-lt, any, none)", key)
	}

	return Condition{
		Type:     ConditionTypeCount,
		Operator: operator,
		Value:    value,
	}, nil
}

// Evaluate evaluates the condition against a record count
func (c Condition) Evaluate(recordCount int64) bool {
	switch c.Operator {
	case OpEqual:
		return recordCount == c.Value
	case OpGreaterEqual:
		return recordCount >= c.Value
	case OpGreater:
		return recordCount > c.Value
	case OpLessEqual:
		return recordCount <= c.Value
	case OpLess:
		return recordCount < c.Value
	default:
		return false
	}
}

// String returns a human-readable representation of the condition
func (c Condition) String() string {
	switch c.Type {
	case ConditionTypeAny:
		return "any"
	case ConditionTypeNone:
		return "none"
	case ConditionTypeCount:
		// Map operator back to kubectl-like format
		switch c.Operator {
		case OpEqual:
			return fmt.Sprintf("count=%d", c.Value)
		case OpGreaterEqual:
			return fmt.Sprintf("count-gte=%d", c.Value)
		case OpGreater:
			return fmt.Sprintf("count-gt=%d", c.Value)
		case OpLessEqual:
			return fmt.Sprintf("count-lte=%d", c.Value)
		case OpLess:
			return fmt.Sprintf("count-lt=%d", c.Value)
		default:
			return fmt.Sprintf("count%s%d", c.Operator, c.Value)
		}
	default:
		return fmt.Sprintf("%s%s%d", c.Type, c.Operator, c.Value)
	}
}
