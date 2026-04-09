package segment

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FilterToAST converts a DQL filter expression to a JSON AST string.
// If the input is already a JSON AST (starts with '{'), it is returned as-is.
func FilterToAST(dql string) (string, error) {
	dql = strings.TrimSpace(dql)
	if dql == "" {
		return "", fmt.Errorf("empty filter expression")
	}
	if isFilterAST(dql) {
		return dql, nil
	}

	p := newParser(dql)
	node, err := p.parseExpression()
	if err != nil {
		return "", fmt.Errorf("failed to parse filter expression: %w", err)
	}
	if p.pos < len(p.input) {
		return "", fmt.Errorf("unexpected input at position %d: %q", p.pos, p.input[p.pos:])
	}

	// Wrap in an implicit root group.
	root := ensureRootGroup(node, len(dql))

	data, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("failed to serialize filter AST: %w", err)
	}
	return string(data), nil
}

// FilterFromAST converts a JSON AST string back to a DQL filter expression.
// If the input is not a JSON AST (doesn't start with '{'), it is returned as-is.
func FilterFromAST(ast string) (string, error) {
	ast = strings.TrimSpace(ast)
	if ast == "" {
		return "", fmt.Errorf("empty filter AST")
	}
	if !isFilterAST(ast) {
		return ast, nil
	}

	result, err := renderFromRaw([]byte(ast))
	if err != nil {
		return "", fmt.Errorf("failed to render filter AST: %w", err)
	}
	return result, nil
}

// isFilterAST returns true if the filter string is a JSON AST (starts with '{').
// Plain DQL filter expressions never start with '{'.
func isFilterAST(filter string) bool {
	return len(filter) > 0 && filter[0] == '{'
}

// ---------------------------------------------------------------------------
// JSON structures for the AST format (the Dynatrace FilterField syntax tree)
// ---------------------------------------------------------------------------

// astGroupJSON is the JSON shape for Group nodes.
type astGroupJSON struct {
	Type            string            `json:"type"`
	LogicalOperator string            `json:"logicalOperator"`
	Explicit        bool              `json:"explicit"`
	Range           *astRange         `json:"range,omitempty"`
	Children        []json.RawMessage `json:"children"`
}

// astStatementJSON is the JSON shape for Statement nodes.
type astStatementJSON struct {
	Type     string          `json:"type"`
	Range    *astRange       `json:"range,omitempty"`
	Key      json.RawMessage `json:"key"`
	Operator json.RawMessage `json:"operator"`
	Value    json.RawMessage `json:"value"`
}

// astLeafJSON is the JSON shape for leaf nodes (Key, ComparisonOperator, String).
type astLeafJSON struct {
	Type      string    `json:"type"`
	TextValue string    `json:"textValue"`
	Value     string    `json:"value"`
	Range     *astRange `json:"range,omitempty"`
	IsEscaped *bool     `json:"isEscaped,omitempty"`
}

// astLogicalOperatorJSON is the JSON shape for LogicalOperator separator nodes.
type astLogicalOperatorJSON struct {
	Type      string    `json:"type"`
	TextValue string    `json:"textValue"`
	Value     string    `json:"value"`
	Range     *astRange `json:"range,omitempty"`
}

type astRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// ---------------------------------------------------------------------------
// Internal representation used during parsing
// ---------------------------------------------------------------------------

// filterNode is the internal representation used during parsing.
type filterNode struct {
	nodeType        string // "Group", "Statement", "LogicalOperator"
	logicalOperator string // "AND", "OR" (for Group)
	explicit        bool   // true = explicit parenthesized group
	rangeFrom       int
	rangeTo         int
	children        []*filterNode
	// Statement fields
	key      *leafNode
	operator *leafNode
	value    *leafNode
	// LogicalOperator separator
	textValue string
}

type leafNode struct {
	leafType  string // "Key", "ComparisonOperator", "String"
	textValue string
	value     string
	rangeFrom int
	rangeTo   int
	isEscaped *bool
}

func (n *filterNode) rangeStart() int { return n.rangeFrom }
func (n *filterNode) rangeEnd() int   { return n.rangeTo }

func (n *filterNode) logicalOperatorOrDefault() string {
	if n.logicalOperator != "" {
		return n.logicalOperator
	}
	return "AND"
}

// ensureRootGroup wraps the parsed node in a root group if needed.
func ensureRootGroup(node *filterNode, inputLen int) *filterNode {
	if node.nodeType == "Group" {
		node.explicit = false
		node.rangeFrom = 0
		node.rangeTo = inputLen
		return node
	}
	return &filterNode{
		nodeType:        "Group",
		logicalOperator: "AND",
		explicit:        false,
		rangeFrom:       0,
		rangeTo:         inputLen,
		children:        []*filterNode{node},
	}
}

// ---------------------------------------------------------------------------
// JSON marshaling: filterNode → API JSON
// ---------------------------------------------------------------------------

// MarshalJSON implements json.Marshaler for filterNode.
func (n *filterNode) MarshalJSON() ([]byte, error) {
	switch n.nodeType {
	case "Group":
		g := astGroupJSON{
			Type:            "Group",
			LogicalOperator: n.logicalOperator,
			Explicit:        n.explicit,
			Range:           &astRange{From: n.rangeFrom, To: n.rangeTo},
		}
		children := make([]json.RawMessage, 0, len(n.children))
		for _, child := range n.children {
			data, err := json.Marshal(child)
			if err != nil {
				return nil, err
			}
			children = append(children, data)
		}
		g.Children = children
		return json.Marshal(g)

	case "Statement":
		keyData, err := marshalLeaf(n.key)
		if err != nil {
			return nil, err
		}
		opData, err := marshalLeaf(n.operator)
		if err != nil {
			return nil, err
		}
		valData, err := marshalLeaf(n.value)
		if err != nil {
			return nil, err
		}
		s := astStatementJSON{
			Type:     "Statement",
			Range:    &astRange{From: n.rangeFrom, To: n.rangeTo},
			Key:      keyData,
			Operator: opData,
			Value:    valData,
		}
		return json.Marshal(s)

	case "LogicalOperator":
		lo := astLogicalOperatorJSON{
			Type:      "LogicalOperator",
			TextValue: n.textValue,
			Value:     n.textValue,
			Range:     &astRange{From: n.rangeFrom, To: n.rangeTo},
		}
		return json.Marshal(lo)

	default:
		return nil, fmt.Errorf("unknown node type: %s", n.nodeType)
	}
}

func marshalLeaf(l *leafNode) (json.RawMessage, error) {
	leaf := astLeafJSON{
		Type:      l.leafType,
		TextValue: l.textValue,
		Value:     l.value,
		Range:     &astRange{From: l.rangeFrom, To: l.rangeTo},
		IsEscaped: l.isEscaped,
	}
	return json.Marshal(leaf)
}

// ---------------------------------------------------------------------------
// Parser: DQL filter string → filterNode tree
// ---------------------------------------------------------------------------

type parser struct {
	input string
	pos   int
}

func newParser(input string) *parser {
	return &parser{input: input, pos: 0}
}

func (p *parser) parseExpression() (*filterNode, error) {
	return p.parseOr()
}

// parseOr: term (OR term)*
func (p *parser) parseOr() (*filterNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	type orTerm struct {
		sepPos int
		node   *filterNode
	}
	var extra []orTerm

	for {
		p.skipSpaces()
		orPos := p.pos
		if !p.matchKeyword("OR") {
			break
		}
		p.skipSpaces()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		extra = append(extra, orTerm{sepPos: orPos, node: right})
	}

	if len(extra) == 0 {
		return left, nil
	}

	children := make([]*filterNode, 0, 1+len(extra)*2)
	children = append(children, left)
	for _, t := range extra {
		children = append(children, &filterNode{
			nodeType:  "LogicalOperator",
			textValue: "OR",
			rangeFrom: t.sepPos,
			rangeTo:   t.sepPos + 2,
		})
		children = append(children, t.node)
	}

	return &filterNode{
		nodeType:        "Group",
		logicalOperator: "OR",
		explicit:        false,
		rangeFrom:       children[0].rangeStart(),
		rangeTo:         children[len(children)-1].rangeEnd(),
		children:        children,
	}, nil
}

// parseAnd: factor ((AND | implicit-AND) factor)*
func (p *parser) parseAnd() (*filterNode, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}

	type andTerm struct {
		sep  *filterNode // nil for implicit AND
		node *filterNode
	}
	var extra []andTerm

	for {
		p.skipSpaces()
		if p.pos >= len(p.input) || p.peek() == ')' || p.peekKeyword("OR") {
			break
		}

		andPos := p.pos
		if p.matchKeyword("AND") {
			p.skipSpaces()
			right, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			extra = append(extra, andTerm{
				sep: &filterNode{
					nodeType:  "LogicalOperator",
					textValue: "AND",
					rangeFrom: andPos,
					rangeTo:   andPos + 3,
				},
				node: right,
			})
			continue
		}

		// implicit AND
		right, err := p.parseFactor()
		if err != nil {
			break
		}
		extra = append(extra, andTerm{sep: nil, node: right})
	}

	if len(extra) == 0 {
		return left, nil
	}

	children := make([]*filterNode, 0)
	children = append(children, left)
	for _, t := range extra {
		if t.sep != nil {
			children = append(children, t.sep)
		}
		children = append(children, t.node)
	}

	return &filterNode{
		nodeType:        "Group",
		logicalOperator: "AND",
		explicit:        false,
		rangeFrom:       children[0].rangeStart(),
		rangeTo:         children[len(children)-1].rangeEnd(),
		children:        children,
	}, nil
}

// parseFactor: "(" expression ")" | statement
func (p *parser) parseFactor() (*filterNode, error) {
	p.skipSpaces()
	if p.pos < len(p.input) && p.input[p.pos] == '(' {
		openPos := p.pos
		p.pos++ // consume '('
		p.skipSpaces()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, fmt.Errorf("expected ')' at position %d", p.pos)
		}
		p.pos++ // consume ')'

		group := &filterNode{
			nodeType:        "Group",
			logicalOperator: expr.logicalOperatorOrDefault(),
			explicit:        true,
			rangeFrom:       openPos,
			rangeTo:         p.pos,
		}
		if expr.nodeType == "Group" {
			group.children = expr.children
			group.logicalOperator = expr.logicalOperator
		} else {
			group.children = []*filterNode{expr}
		}
		return group, nil
	}
	return p.parseStatement()
}

// parseStatement: key operator value
func (p *parser) parseStatement() (*filterNode, error) {
	p.skipSpaces()
	startPos := p.pos

	key, err := p.parseKey()
	if err != nil {
		return nil, fmt.Errorf("expected key at position %d: %w", p.pos, err)
	}

	p.skipSpaces()
	op, err := p.parseOperator()
	if err != nil {
		return nil, fmt.Errorf("expected operator at position %d: %w", p.pos, err)
	}

	p.skipSpaces()
	val, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("expected value at position %d: %w", p.pos, err)
	}

	return &filterNode{
		nodeType:  "Statement",
		rangeFrom: startPos,
		rangeTo:   val.rangeTo,
		key:       key,
		operator:  op,
		value:     val,
	}, nil
}

func (p *parser) parseKey() (*leafNode, error) {
	p.skipSpaces()
	start := p.pos

	if p.pos < len(p.input) && p.input[p.pos] == '`' {
		// Backtick-escaped key
		p.pos++ // consume opening backtick
		end := strings.IndexByte(p.input[p.pos:], '`')
		if end == -1 {
			return nil, fmt.Errorf("unterminated backtick-escaped key")
		}
		keyValue := p.input[p.pos : p.pos+end]
		p.pos += end + 1 // consume content + closing backtick
		return &leafNode{
			leafType:  "Key",
			textValue: p.input[start:p.pos],
			value:     keyValue,
			rangeFrom: start,
			rangeTo:   p.pos,
		}, nil
	}

	// Unquoted key: alphanumeric, dots, hyphens, underscores
	for p.pos < len(p.input) && isKeyChar(p.input[p.pos]) {
		p.pos++
	}

	if p.pos == start {
		if p.pos < len(p.input) {
			return nil, fmt.Errorf("unexpected character %q", p.input[p.pos])
		}
		return nil, fmt.Errorf("unexpected end of input")
	}

	text := p.input[start:p.pos]
	return &leafNode{
		leafType:  "Key",
		textValue: text,
		value:     text,
		rangeFrom: start,
		rangeTo:   p.pos,
	}, nil
}

func (p *parser) parseOperator() (*leafNode, error) {
	p.skipSpaces()
	start := p.pos
	remaining := p.input[p.pos:]

	// Check operators from longest to shortest to avoid prefix conflicts
	for _, op := range []string{"not in", "!=", "<=", ">=", "<", ">", "=", "in"} {
		if strings.HasPrefix(remaining, op) {
			// "not in" and "in" require whitespace or end after them
			if op == "not in" || op == "in" {
				afterOp := p.pos + len(op)
				if afterOp < len(p.input) && !isSpace(p.input[afterOp]) {
					continue
				}
			}
			// Reject "==" — single "=" must not be followed by another "="
			if op == "=" && p.pos+1 < len(p.input) && p.input[p.pos+1] == '=' {
				return nil, fmt.Errorf("unsupported filter syntax %q; the Segments API uses single '=' for equality. Provide the filter as a JSON AST string if you need advanced syntax", "==")
			}
			p.pos += len(op)
			return &leafNode{
				leafType:  "ComparisonOperator",
				textValue: op,
				value:     op,
				rangeFrom: start,
				rangeTo:   p.pos,
			}, nil
		}
	}

	// Check unsupported operators for better error messages
	for _, unsupported := range []string{"~", "!~"} {
		if strings.HasPrefix(remaining, unsupported) {
			return nil, fmt.Errorf("unsupported filter syntax %q; provide the filter as a JSON AST string instead", unsupported)
		}
	}

	maxLen := len(remaining)
	if maxLen > 10 {
		maxLen = 10
	}
	return nil, fmt.Errorf("unknown operator at %q", remaining[:maxLen])
}

func (p *parser) parseValue() (*leafNode, error) {
	p.skipSpaces()
	start := p.pos

	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("unexpected end of input")
	}

	ch := p.input[p.pos]

	// Check for unsupported features
	if ch == '*' {
		return nil, fmt.Errorf("unsupported filter syntax \"*\" (wildcard/exists); provide the filter as a JSON AST string instead")
	}
	if ch == '$' {
		return nil, fmt.Errorf("unsupported filter syntax \"$\" (variable reference); provide the filter as a JSON AST string instead")
	}
	if ch == '(' {
		return nil, fmt.Errorf("unsupported filter syntax \"(\" in value position (list syntax); provide the filter as a JSON AST string instead")
	}

	if ch == '"' {
		return p.parseQuotedString()
	}
	return p.parseUnquotedValue(start)
}

func (p *parser) parseQuotedString() (*leafNode, error) {
	start := p.pos
	p.pos++ // consume opening quote

	var sb strings.Builder
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' && p.pos+1 < len(p.input) {
			next := p.input[p.pos+1]
			switch next {
			case '"', '\\':
				sb.WriteByte(next)
				p.pos += 2
			default:
				sb.WriteByte(ch)
				p.pos++
			}
			continue
		}
		if ch == '"' {
			p.pos++ // consume closing quote
			escaped := true
			return &leafNode{
				leafType:  "String",
				textValue: p.input[start:p.pos],
				value:     sb.String(),
				rangeFrom: start,
				rangeTo:   p.pos,
				isEscaped: &escaped,
			}, nil
		}
		sb.WriteByte(ch)
		p.pos++
	}
	return nil, fmt.Errorf("unterminated quoted string starting at position %d", start)
}

func (p *parser) parseUnquotedValue(start int) (*leafNode, error) {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if isSpace(ch) || ch == ')' {
			break
		}
		p.pos++
	}

	if p.pos == start {
		return nil, fmt.Errorf("empty value")
	}

	text := p.input[start:p.pos]
	escaped := false
	return &leafNode{
		leafType:  "String",
		textValue: text,
		value:     text,
		rangeFrom: start,
		rangeTo:   p.pos,
		isEscaped: &escaped,
	}, nil
}

// ---------------------------------------------------------------------------
// Parser helpers
// ---------------------------------------------------------------------------

func (p *parser) skipSpaces() {
	for p.pos < len(p.input) && isSpace(p.input[p.pos]) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

// matchKeyword matches a keyword followed by non-key-char or EOF, consuming it.
func (p *parser) matchKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.input) {
		return false
	}
	if p.input[p.pos:p.pos+len(kw)] != kw {
		return false
	}
	afterPos := p.pos + len(kw)
	if afterPos < len(p.input) && isKeyChar(p.input[afterPos]) {
		return false
	}
	p.pos += len(kw)
	return true
}

// peekKeyword checks if the next token is a keyword without consuming it.
func (p *parser) peekKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.input) {
		return false
	}
	if p.input[p.pos:p.pos+len(kw)] != kw {
		return false
	}
	afterPos := p.pos + len(kw)
	if afterPos < len(p.input) && isKeyChar(p.input[afterPos]) {
		return false
	}
	return true
}

func isKeyChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '.' || ch == '-' || ch == '_'
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// ---------------------------------------------------------------------------
// Renderer: JSON AST → DQL filter string
// ---------------------------------------------------------------------------

// renderFromRaw renders a raw JSON AST back to a DQL filter string.
func renderFromRaw(data []byte) (string, error) {
	var peek struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return "", fmt.Errorf("failed to peek AST node type: %w", err)
	}

	switch peek.Type {
	case "Group":
		return renderGroup(data)
	case "Statement":
		return renderStatement(data)
	default:
		return "", fmt.Errorf("unknown AST node type: %q", peek.Type)
	}
}

func renderGroup(data []byte) (string, error) {
	var g astGroupJSON
	if err := json.Unmarshal(data, &g); err != nil {
		return "", fmt.Errorf("failed to unmarshal Group: %w", err)
	}

	var parts []string
	for _, childRaw := range g.Children {
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(childRaw, &peek); err != nil {
			return "", err
		}

		switch peek.Type {
		case "LogicalOperator":
			var lo astLogicalOperatorJSON
			if err := json.Unmarshal(childRaw, &lo); err != nil {
				return "", err
			}
			parts = append(parts, lo.Value)
		case "Group":
			sub, err := renderGroup(childRaw)
			if err != nil {
				return "", err
			}
			var subGroup astGroupJSON
			if err := json.Unmarshal(childRaw, &subGroup); err != nil {
				return "", err
			}
			if subGroup.Explicit {
				sub = "(" + sub + ")"
			}
			parts = append(parts, sub)
		case "Statement":
			s, err := renderStatement(childRaw)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		default:
			return "", fmt.Errorf("unknown child node type in group: %q", peek.Type)
		}
	}

	return strings.Join(parts, " "), nil
}

func renderStatement(data []byte) (string, error) {
	var s astStatementJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return "", fmt.Errorf("failed to unmarshal Statement: %w", err)
	}

	var key astLeafJSON
	if err := json.Unmarshal(s.Key, &key); err != nil {
		return "", fmt.Errorf("failed to unmarshal key: %w", err)
	}

	var op astLeafJSON
	if err := json.Unmarshal(s.Operator, &op); err != nil {
		return "", fmt.Errorf("failed to unmarshal operator: %w", err)
	}

	var val astLeafJSON
	if err := json.Unmarshal(s.Value, &val); err != nil {
		return "", fmt.Errorf("failed to unmarshal value: %w", err)
	}

	keyText := renderKeyText(key)
	valText := renderValueText(val)

	return fmt.Sprintf("%s %s %s", keyText, op.Value, valText), nil
}

func renderKeyText(key astLeafJSON) string {
	if len(key.TextValue) > 0 && key.TextValue[0] == '`' {
		return key.TextValue
	}
	if needsBacktickEscape(key.Value) {
		return "`" + key.Value + "`"
	}
	return key.Value
}

func renderValueText(val astLeafJSON) string {
	if val.Type == "String" {
		if val.IsEscaped != nil && *val.IsEscaped {
			return `"` + escapeStringValue(val.Value) + `"`
		}
		if isNumericString(val.Value) {
			return val.Value
		}
		return `"` + escapeStringValue(val.Value) + `"`
	}
	return val.Value
}

func escapeStringValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func needsBacktickEscape(key string) bool {
	if key == "" {
		return true
	}
	for i := 0; i < len(key); i++ {
		if !isKeyChar(key[i]) {
			return true
		}
	}
	return false
}

func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
