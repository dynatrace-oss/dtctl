package segment

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// DQL → AST (FilterToAST)
// ---------------------------------------------------------------------------

func TestFilterToAST_Simple(t *testing.T) {
	ast, err := FilterToAST(`status = "ERROR"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isFilterAST(ast) {
		t.Fatalf("expected JSON AST, got: %s", ast)
	}

	// Verify the root group structure
	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal AST: %v", err)
	}
	if root.Type != "Group" {
		t.Errorf("expected root type 'Group', got %q", root.Type)
	}
	if root.LogicalOperator != "AND" {
		t.Errorf("expected logical operator 'AND', got %q", root.LogicalOperator)
	}
	if root.Explicit {
		t.Error("expected root to be non-explicit")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}

	// Verify the statement
	var stmt astStatementJSON
	if err := json.Unmarshal(root.Children[0], &stmt); err != nil {
		t.Fatalf("failed to unmarshal statement: %v", err)
	}
	if stmt.Type != "Statement" {
		t.Errorf("expected statement type 'Statement', got %q", stmt.Type)
	}

	// Verify key
	var key astLeafJSON
	if err := json.Unmarshal(stmt.Key, &key); err != nil {
		t.Fatalf("failed to unmarshal key: %v", err)
	}
	if key.Value != "status" {
		t.Errorf("expected key value 'status', got %q", key.Value)
	}
	if key.TextValue != "status" {
		t.Errorf("expected key textValue 'status', got %q", key.TextValue)
	}

	// Verify operator
	var op astLeafJSON
	if err := json.Unmarshal(stmt.Operator, &op); err != nil {
		t.Fatalf("failed to unmarshal operator: %v", err)
	}
	if op.Value != "=" {
		t.Errorf("expected operator '=', got %q", op.Value)
	}

	// Verify value
	var val astLeafJSON
	if err := json.Unmarshal(stmt.Value, &val); err != nil {
		t.Fatalf("failed to unmarshal value: %v", err)
	}
	if val.Value != "ERROR" {
		t.Errorf("expected value 'ERROR', got %q", val.Value)
	}
	if val.TextValue != `"ERROR"` {
		t.Errorf("expected textValue '\"ERROR\"', got %q", val.TextValue)
	}
	if val.IsEscaped == nil || !*val.IsEscaped {
		t.Error("expected isEscaped to be true")
	}
}

func TestFilterToAST_UnquotedValue(t *testing.T) {
	ast, err := FilterToAST(`count = 42`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var stmt astStatementJSON
	if err := json.Unmarshal(root.Children[0], &stmt); err != nil {
		t.Fatalf("failed to unmarshal statement: %v", err)
	}

	var val astLeafJSON
	if err := json.Unmarshal(stmt.Value, &val); err != nil {
		t.Fatalf("failed to unmarshal value: %v", err)
	}
	if val.Value != "42" {
		t.Errorf("expected value '42', got %q", val.Value)
	}
	if val.TextValue != "42" {
		t.Errorf("expected textValue '42', got %q", val.TextValue)
	}
	if val.IsEscaped == nil || *val.IsEscaped {
		t.Error("expected isEscaped to be false for unquoted value")
	}
}

func TestFilterToAST_DottedKey(t *testing.T) {
	ast, err := FilterToAST(`k8s.cluster.name = "alpha"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	var stmt astStatementJSON
	if err := json.Unmarshal(root.Children[0], &stmt); err != nil {
		t.Fatalf("failed to unmarshal statement: %v", err)
	}
	var key astLeafJSON
	if err := json.Unmarshal(stmt.Key, &key); err != nil {
		t.Fatalf("failed to unmarshal key: %v", err)
	}
	if key.Value != "k8s.cluster.name" {
		t.Errorf("expected key 'k8s.cluster.name', got %q", key.Value)
	}
}

func TestFilterToAST_BacktickKey(t *testing.T) {
	ast, err := FilterToAST("`my field` = \"value\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	var stmt astStatementJSON
	if err := json.Unmarshal(root.Children[0], &stmt); err != nil {
		t.Fatalf("failed to unmarshal statement: %v", err)
	}
	var key astLeafJSON
	if err := json.Unmarshal(stmt.Key, &key); err != nil {
		t.Fatalf("failed to unmarshal key: %v", err)
	}
	if key.Value != "my field" {
		t.Errorf("expected key 'my field', got %q", key.Value)
	}
	if key.TextValue != "`my field`" {
		t.Errorf("expected textValue '`my field`', got %q", key.TextValue)
	}
}

func TestFilterToAST_ImplicitAND(t *testing.T) {
	ast, err := FilterToAST(`a = "1" b = "2"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if root.LogicalOperator != "AND" {
		t.Errorf("expected AND, got %q", root.LogicalOperator)
	}
	// Should have 2 Statement children (no separator for implicit AND)
	stmtCount := 0
	for _, child := range root.Children {
		var peek struct {
			Type string `json:"type"`
		}
		json.Unmarshal(child, &peek)
		if peek.Type == "Statement" {
			stmtCount++
		}
	}
	if stmtCount != 2 {
		t.Errorf("expected 2 statements, got %d", stmtCount)
	}
}

func TestFilterToAST_ExplicitAND(t *testing.T) {
	ast, err := FilterToAST(`a = "1" AND b = "2"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if root.LogicalOperator != "AND" {
		t.Errorf("expected AND, got %q", root.LogicalOperator)
	}
	// Should have 3 children: Statement, LogicalOperator("AND"), Statement
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}
	var lo astLogicalOperatorJSON
	if err := json.Unmarshal(root.Children[1], &lo); err != nil {
		t.Fatalf("failed to unmarshal logical operator: %v", err)
	}
	if lo.Value != "AND" {
		t.Errorf("expected AND separator, got %q", lo.Value)
	}
}

func TestFilterToAST_OR(t *testing.T) {
	ast, err := FilterToAST(`a = "1" OR b = "2"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if root.LogicalOperator != "OR" {
		t.Errorf("expected OR, got %q", root.LogicalOperator)
	}
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children (stmt, OR sep, stmt), got %d", len(root.Children))
	}
}

func TestFilterToAST_MixedANDOR(t *testing.T) {
	// a = "1" OR b = "2" c = "3"
	// Due to precedence: OR-group containing [a="1"] OR [AND-group of b="2" c="3"]
	ast, err := FilterToAST(`a = "1" OR b = "2" c = "3"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if root.LogicalOperator != "OR" {
		t.Errorf("expected root OR, got %q", root.LogicalOperator)
	}
	// Children: Statement(a=1), LogicalOperator(OR), Group(AND: b=2, c=3)
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}
	// Third child should be a Group
	var thirdPeek struct {
		Type string `json:"type"`
	}
	json.Unmarshal(root.Children[2], &thirdPeek)
	if thirdPeek.Type != "Group" {
		t.Errorf("expected third child to be Group, got %q", thirdPeek.Type)
	}
}

func TestFilterToAST_Parentheses(t *testing.T) {
	ast, err := FilterToAST(`(a = "1" OR b = "2") AND c = "3"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if root.LogicalOperator != "AND" {
		t.Errorf("expected AND root, got %q", root.LogicalOperator)
	}

	// First child should be an explicit group
	var firstPeek struct {
		Type     string `json:"type"`
		Explicit bool   `json:"explicit"`
	}
	json.Unmarshal(root.Children[0], &firstPeek)
	if firstPeek.Type != "Group" {
		t.Errorf("expected first child to be Group, got %q", firstPeek.Type)
	}
	if !firstPeek.Explicit {
		t.Error("expected first child group to be explicit (parenthesized)")
	}
}

func TestFilterToAST_AllOperators(t *testing.T) {
	operators := []string{"=", "!=", "<", "<=", ">", ">=", "in"}
	for _, op := range operators {
		t.Run(op, func(t *testing.T) {
			input := "key " + op + ` "value"`
			ast, err := FilterToAST(input)
			if err != nil {
				t.Fatalf("unexpected error for operator %q: %v", op, err)
			}

			var root astGroupJSON
			json.Unmarshal([]byte(ast), &root)
			var stmt astStatementJSON
			json.Unmarshal(root.Children[0], &stmt)
			var opNode astLeafJSON
			json.Unmarshal(stmt.Operator, &opNode)
			if opNode.Value != op {
				t.Errorf("expected operator %q, got %q", op, opNode.Value)
			}
		})
	}
}

func TestFilterToAST_NotInOperator(t *testing.T) {
	// "not in" is a supported operator
	ast, err := FilterToAST(`status not in "active"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	json.Unmarshal([]byte(ast), &root)
	var stmt astStatementJSON
	json.Unmarshal(root.Children[0], &stmt)
	var op astLeafJSON
	json.Unmarshal(stmt.Operator, &op)
	if op.Value != "not in" {
		t.Errorf("expected operator 'not in', got %q", op.Value)
	}
}

func TestFilterToAST_InOperator(t *testing.T) {
	ast, err := FilterToAST(`status in "active"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	json.Unmarshal([]byte(ast), &root)
	var stmt astStatementJSON
	json.Unmarshal(root.Children[0], &stmt)
	var op astLeafJSON
	json.Unmarshal(stmt.Operator, &op)
	if op.Value != "in" {
		t.Errorf("expected operator 'in', got %q", op.Value)
	}

	// Verify round-trip: AST → DQL should produce original DQL
	dql, err := FilterFromAST(ast)
	if err != nil {
		t.Fatalf("FilterFromAST error: %v", err)
	}
	if dql != `status in "active"` {
		t.Errorf("round-trip failed, got %q", dql)
	}
}

func TestFilterToAST_Ranges(t *testing.T) {
	// Verify range positions are correct
	// Input: `status = "ERROR"` (16 chars)
	// Positions: status(0-6) =(7-8) "ERROR"(9-16)
	ast, err := FilterToAST(`status = "ERROR"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root astGroupJSON
	json.Unmarshal([]byte(ast), &root)

	// Root range should span entire input
	if root.Range == nil {
		t.Fatal("expected range on root")
	}
	if root.Range.From != 0 || root.Range.To != 16 {
		t.Errorf("expected root range {0, 16}, got {%d, %d}", root.Range.From, root.Range.To)
	}

	var stmt astStatementJSON
	json.Unmarshal(root.Children[0], &stmt)

	var key astLeafJSON
	json.Unmarshal(stmt.Key, &key)
	if key.Range.From != 0 || key.Range.To != 6 {
		t.Errorf("expected key range {0, 6}, got {%d, %d}", key.Range.From, key.Range.To)
	}

	var op astLeafJSON
	json.Unmarshal(stmt.Operator, &op)
	if op.Range.From != 7 || op.Range.To != 8 {
		t.Errorf("expected operator range {7, 8}, got {%d, %d}", op.Range.From, op.Range.To)
	}

	var val astLeafJSON
	json.Unmarshal(stmt.Value, &val)
	if val.Range.From != 9 || val.Range.To != 16 {
		t.Errorf("expected value range {9, 16}, got {%d, %d}", val.Range.From, val.Range.To)
	}
}

func TestFilterToAST_Errors(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		errorContains string
	}{
		{
			name:          "empty string",
			input:         "",
			errorContains: "empty filter expression",
		},
		{
			name:          "only whitespace",
			input:         "   ",
			errorContains: "empty filter expression",
		},
		{
			name:          "unsupported == operator",
			input:         `status == "ERROR"`,
			errorContains: "unsupported filter syntax",
		},
		{
			name:          "unterminated string",
			input:         `status = "ERROR`,
			errorContains: "unterminated quoted string",
		},
		{
			name:          "unterminated backtick key",
			input:         "`my field = \"value\"",
			errorContains: "unterminated backtick",
		},
		{
			name:          "missing value",
			input:         `status =`,
			errorContains: "expected value",
		},
		{
			name:          "unsupported wildcard",
			input:         `status = *`,
			errorContains: "unsupported filter syntax",
		},
		{
			name:          "unsupported variable",
			input:         `status = $var`,
			errorContains: "unsupported filter syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FilterToAST(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			} else if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
			}
		})
	}
}

func TestFilterToAST_AlreadyAST(t *testing.T) {
	astInput := `{"type":"Group","logicalOperator":"AND","explicit":false,"children":[]}`
	result, err := FilterToAST(astInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != astInput {
		t.Errorf("expected passthrough of AST, got different result")
	}
}

// ---------------------------------------------------------------------------
// AST → DQL (FilterFromAST)
// ---------------------------------------------------------------------------

func TestFilterFromAST_SingleStatement(t *testing.T) {
	ast := `{"type":"Group","logicalOperator":"AND","explicit":false,"range":{"from":0,"to":16},"children":[{"type":"Statement","range":{"from":0,"to":16},"key":{"type":"Key","textValue":"status","value":"status","range":{"from":0,"to":6}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":7,"to":8}},"value":{"type":"String","textValue":"\"ERROR\"","value":"ERROR","range":{"from":9,"to":16},"isEscaped":true}}]}`

	result, err := FilterFromAST(ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `status = "ERROR"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFilterFromAST_MultiStatementAND(t *testing.T) {
	// Build an AST with two statements and implicit AND (no separator node)
	ast := `{"type":"Group","logicalOperator":"AND","explicit":false,"range":{"from":0,"to":19},"children":[{"type":"Statement","range":{"from":0,"to":7},"key":{"type":"Key","textValue":"a","value":"a","range":{"from":0,"to":1}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":2,"to":3}},"value":{"type":"String","textValue":"\"1\"","value":"1","range":{"from":4,"to":7},"isEscaped":true}},{"type":"Statement","range":{"from":8,"to":19},"key":{"type":"Key","textValue":"b","value":"b","range":{"from":8,"to":9}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":10,"to":11}},"value":{"type":"String","textValue":"\"2\"","value":"2","range":{"from":12,"to":15},"isEscaped":true}}]}`

	result, err := FilterFromAST(ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two statements joined with space (implicit AND)
	expected := `a = "1" b = "2"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFilterFromAST_ORGroup(t *testing.T) {
	ast := `{"type":"Group","logicalOperator":"OR","explicit":false,"range":{"from":0,"to":19},"children":[{"type":"Statement","range":{"from":0,"to":7},"key":{"type":"Key","textValue":"a","value":"a","range":{"from":0,"to":1}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":2,"to":3}},"value":{"type":"String","textValue":"\"1\"","value":"1","range":{"from":4,"to":7},"isEscaped":true}},{"type":"LogicalOperator","textValue":"OR","value":"OR","range":{"from":8,"to":10}},{"type":"Statement","range":{"from":11,"to":19},"key":{"type":"Key","textValue":"b","value":"b","range":{"from":11,"to":12}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":13,"to":14}},"value":{"type":"String","textValue":"\"2\"","value":"2","range":{"from":15,"to":18},"isEscaped":true}}]}`

	result, err := FilterFromAST(ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `a = "1" OR b = "2"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFilterFromAST_NestedGroups(t *testing.T) {
	// (a = "1" OR b = "2") AND c = "3"
	// The explicit group should produce parentheses
	ast := `{"type":"Group","logicalOperator":"AND","explicit":false,"range":{"from":0,"to":31},"children":[{"type":"Group","logicalOperator":"OR","explicit":true,"range":{"from":0,"to":20},"children":[{"type":"Statement","range":{"from":1,"to":8},"key":{"type":"Key","textValue":"a","value":"a","range":{"from":1,"to":2}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":3,"to":4}},"value":{"type":"String","textValue":"\"1\"","value":"1","range":{"from":5,"to":8},"isEscaped":true}},{"type":"LogicalOperator","textValue":"OR","value":"OR","range":{"from":9,"to":11}},{"type":"Statement","range":{"from":12,"to":19},"key":{"type":"Key","textValue":"b","value":"b","range":{"from":12,"to":13}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":14,"to":15}},"value":{"type":"String","textValue":"\"2\"","value":"2","range":{"from":16,"to":19},"isEscaped":true}}]},{"type":"LogicalOperator","textValue":"AND","value":"AND","range":{"from":21,"to":24}},{"type":"Statement","range":{"from":25,"to":31},"key":{"type":"Key","textValue":"c","value":"c","range":{"from":25,"to":26}},"operator":{"type":"ComparisonOperator","textValue":"=","value":"=","range":{"from":27,"to":28}},"value":{"type":"String","textValue":"\"3\"","value":"3","range":{"from":29,"to":32},"isEscaped":true}}]}`

	result, err := FilterFromAST(ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `(a = "1" OR b = "2") AND c = "3"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFilterFromAST_NotAST(t *testing.T) {
	// Non-AST input should be passed through
	input := `status = "ERROR"`
	result, err := FilterFromAST(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != input {
		t.Errorf("expected passthrough of non-AST, got %q", result)
	}
}

func TestFilterFromAST_Errors(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		errorContains string
	}{
		{
			name:          "empty string",
			input:         "",
			errorContains: "empty filter AST",
		},
		{
			name:          "invalid JSON",
			input:         `{invalid json}`,
			errorContains: "failed to peek AST node type",
		},
		{
			name:          "unknown type",
			input:         `{"type":"Unknown"}`,
			errorContains: "unknown AST node type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FilterFromAST(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			} else if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Round-trip tests: DQL → AST → DQL
// ---------------------------------------------------------------------------

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // expected DQL after round-trip (canonical form)
	}{
		{
			name:     "simple equality",
			input:    `status = "ERROR"`,
			expected: `status = "ERROR"`,
		},
		{
			name:     "dotted key",
			input:    `k8s.cluster.name = "alpha"`,
			expected: `k8s.cluster.name = "alpha"`,
		},
		{
			name:     "unquoted numeric value",
			input:    `count = 42`,
			expected: `count = 42`,
		},
		{
			name:     "backtick key",
			input:    "`my field` = \"value\"",
			expected: "`my field` = \"value\"",
		},
		{
			name:     "implicit AND",
			input:    `a = "1" b = "2"`,
			expected: `a = "1" b = "2"`,
		},
		{
			name:     "explicit AND",
			input:    `a = "1" AND b = "2"`,
			expected: `a = "1" AND b = "2"`,
		},
		{
			name:     "OR expression",
			input:    `a = "1" OR b = "2"`,
			expected: `a = "1" OR b = "2"`,
		},
		{
			name:     "parenthesized OR with AND",
			input:    `(a = "1" OR b = "2") AND c = "3"`,
			expected: `(a = "1" OR b = "2") AND c = "3"`,
		},
		{
			name:     "not-equal operator",
			input:    `status != "OK"`,
			expected: `status != "OK"`,
		},
		{
			name:     "less-than operator",
			input:    `count < 100`,
			expected: `count < 100`,
		},
		{
			name:     "greater-or-equal operator",
			input:    `value >= 3.14`,
			expected: `value >= 3.14`,
		},
		{
			name:     "dt.system.bucket",
			input:    `dt.system.bucket = "custom-logs"`,
			expected: `dt.system.bucket = "custom-logs"`,
		},
		{
			name:     "loglevel OR",
			input:    `loglevel = "ERROR" OR loglevel = "WARN"`,
			expected: `loglevel = "ERROR" OR loglevel = "WARN"`,
		},
		{
			name:     "not in operator",
			input:    `status not in "active"`,
			expected: `status not in "active"`,
		},
		{
			name:     "mixed precedence",
			input:    `a = "1" OR b = "2" c = "3"`,
			expected: `a = "1" OR b = "2" c = "3"`,
		},
		{
			name:     "three AND terms",
			input:    `a = "1" b = "2" c = "3"`,
			expected: `a = "1" b = "2" c = "3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := FilterToAST(tt.input)
			if err != nil {
				t.Fatalf("FilterToAST failed: %v", err)
			}

			dql, err := FilterFromAST(ast)
			if err != nil {
				t.Fatalf("FilterFromAST failed: %v", err)
			}

			if dql != tt.expected {
				t.Errorf("round-trip mismatch:\n  input:    %q\n  ast:      %s\n  output:   %q\n  expected: %q",
					tt.input, ast, dql, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Auto-detection
// ---------------------------------------------------------------------------

func TestIsFilterAST(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`{"type":"Group"}`, true},
		{`{anything}`, true},
		{`status = "ERROR"`, false},
		{`(a = "1" OR b = "2")`, false},
		{``, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isFilterAST(tt.input); got != tt.expected {
				t.Errorf("isFilterAST(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// API spec example: verify the exact format from FILTER_AST_PLAN.md
// ---------------------------------------------------------------------------

func TestFilterToAST_APISpecExample(t *testing.T) {
	// From the plan: k8s.cluster.name = "alpha" should produce a specific AST structure
	input := `k8s.cluster.name = "alpha"`
	ast, err := FilterToAST(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse and verify structure matches the expected API format
	var root astGroupJSON
	if err := json.Unmarshal([]byte(ast), &root); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if root.Type != "Group" {
		t.Errorf("expected type 'Group', got %q", root.Type)
	}
	if root.LogicalOperator != "AND" {
		t.Errorf("expected logicalOperator 'AND', got %q", root.LogicalOperator)
	}
	if root.Explicit {
		t.Error("expected explicit=false")
	}
	if root.Range.From != 0 || root.Range.To != 26 {
		t.Errorf("expected root range {0, 26}, got {%d, %d}", root.Range.From, root.Range.To)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}

	var stmt astStatementJSON
	json.Unmarshal(root.Children[0], &stmt)
	if stmt.Type != "Statement" {
		t.Errorf("expected Statement, got %q", stmt.Type)
	}
	if stmt.Range.From != 0 || stmt.Range.To != 26 {
		t.Errorf("expected statement range {0, 26}, got {%d, %d}", stmt.Range.From, stmt.Range.To)
	}

	var key astLeafJSON
	json.Unmarshal(stmt.Key, &key)
	if key.Type != "Key" || key.TextValue != "k8s.cluster.name" || key.Value != "k8s.cluster.name" {
		t.Errorf("unexpected key: %+v", key)
	}
	if key.Range.From != 0 || key.Range.To != 16 {
		t.Errorf("expected key range {0, 16}, got {%d, %d}", key.Range.From, key.Range.To)
	}

	var op astLeafJSON
	json.Unmarshal(stmt.Operator, &op)
	if op.Type != "ComparisonOperator" || op.Value != "=" {
		t.Errorf("unexpected operator: %+v", op)
	}
	if op.Range.From != 17 || op.Range.To != 18 {
		t.Errorf("expected operator range {17, 18}, got {%d, %d}", op.Range.From, op.Range.To)
	}

	var val astLeafJSON
	json.Unmarshal(stmt.Value, &val)
	if val.Type != "String" || val.Value != "alpha" || val.TextValue != `"alpha"` {
		t.Errorf("unexpected value: %+v", val)
	}
	if val.Range.From != 19 || val.Range.To != 26 {
		t.Errorf("expected value range {19, 26}, got {%d, %d}", val.Range.From, val.Range.To)
	}
	if val.IsEscaped == nil || !*val.IsEscaped {
		t.Error("expected isEscaped=true")
	}
}

// ---------------------------------------------------------------------------
// convertIncludesForAPI / convertIncludesForDisplay
// ---------------------------------------------------------------------------

func TestConvertIncludesForAPI(t *testing.T) {
	input := `{"name":"test","includes":[{"dataObject":"logs","filter":"status = \"ERROR\""}]}`
	result, err := convertIncludesForAPI([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The filter field should now be a JSON AST string
	var parsed struct {
		Includes []struct {
			Filter string `json:"filter"`
		} `json:"includes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(parsed.Includes) != 1 {
		t.Fatalf("expected 1 include, got %d", len(parsed.Includes))
	}
	if !isFilterAST(parsed.Includes[0].Filter) {
		t.Errorf("expected filter to be AST, got: %s", parsed.Includes[0].Filter)
	}
}

func TestConvertIncludesForAPI_AlreadyAST(t *testing.T) {
	astFilter := `{"type":"Group","logicalOperator":"AND","explicit":false,"children":[]}`
	input := `{"name":"test","includes":[{"dataObject":"logs","filter":` + jsonString(astFilter) + `}]}`
	result, err := convertIncludesForAPI([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Includes []struct {
			Filter string `json:"filter"`
		} `json:"includes"`
	}
	json.Unmarshal(result, &parsed)
	// Should pass through unchanged
	if parsed.Includes[0].Filter != astFilter {
		t.Errorf("expected AST passthrough, got: %s", parsed.Includes[0].Filter)
	}
}

func TestConvertIncludesForDisplay(t *testing.T) {
	// Build a segment with AST filters and verify they get converted to DQL
	astFilter, _ := FilterToAST(`status = "ERROR"`)

	seg := &FilterSegment{
		UID:  "test-uid",
		Name: "test",
		Includes: []Include{
			{DataObject: "logs", Filter: astFilter},
		},
	}

	convertIncludesForDisplay(seg)

	if seg.Includes[0].Filter != `status = "ERROR"` {
		t.Errorf("expected DQL filter, got: %s", seg.Includes[0].Filter)
	}
}

func TestConvertIncludesForDisplay_AlreadyDQL(t *testing.T) {
	seg := &FilterSegment{
		UID:  "test-uid",
		Name: "test",
		Includes: []Include{
			{DataObject: "logs", Filter: `status = "ERROR"`},
		},
	}

	convertIncludesForDisplay(seg)

	if seg.Includes[0].Filter != `status = "ERROR"` {
		t.Errorf("expected DQL passthrough, got: %s", seg.Includes[0].Filter)
	}
}

func TestConvertIncludesForAPI_NoIncludes(t *testing.T) {
	input := `{"name":"test"}`
	result, err := convertIncludesForAPI([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should pass through unchanged
	if string(result) != input {
		t.Errorf("expected unchanged output for no includes")
	}
}

// jsonString returns a JSON-encoded string literal.
func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
