package resource

import (
	"errors"
	"strconv"
	"strings"
)

var ErrBadFilter = errors.New("scimply: invalid filter expression")

type FilterExpression interface {
	filterNode()
}

type CompareOp int

const (
	OpEq CompareOp = iota
	OpNe
	OpCo
	OpSw
	OpEw
	OpGt
	OpGe
	OpLt
	OpLe
	OpPr
)

func (op CompareOp) String() string {
	switch op {
	case OpEq:
		return "eq"
	case OpNe:
		return "ne"
	case OpCo:
		return "co"
	case OpSw:
		return "sw"
	case OpEw:
		return "ew"
	case OpGt:
		return "gt"
	case OpGe:
		return "ge"
	case OpLt:
		return "lt"
	case OpLe:
		return "le"
	case OpPr:
		return "pr"
	}
	return "unknown"
}

type LogicalOp int

const (
	LogicalAnd LogicalOp = iota
	LogicalOr
)

type AttrExpression struct {
	Path     AttributePath
	Operator CompareOp
	Value    interface{}
}

func (*AttrExpression) filterNode() {}

type LogicalExpression struct {
	Left  FilterExpression
	Op    LogicalOp
	Right FilterExpression
}

func (*LogicalExpression) filterNode() {}

type NotExpression struct {
	Inner FilterExpression
}

func (*NotExpression) filterNode() {}

type ValuePathExpression struct {
	Path   AttributePath
	Filter FilterExpression
}

func (*ValuePathExpression) filterNode() {}

// ParseFilter parses a SCIM filter string into an AST of FilterExpression nodes.
// It uses a hand-written recursive-descent parser (filterParser) with operator
// precedence: OR has the lowest precedence, then AND, then NOT, then primary
// expressions. After parsing, any unconsumed input means the filter is malformed.
func ParseFilter(s string) (FilterExpression, error) {
	p := &filterParser{input: s, pos: 0}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.pos != len(p.input) {
		return nil, ErrBadFilter
	}
	return expr, nil
}

type filterParser struct {
	input string
	pos   int
}

func (p *filterParser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *filterParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *filterParser) remaining() string {
	return p.input[p.pos:]
}

func (p *filterParser) parseOr() (FilterExpression, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if !p.matchKeyword("or") {
			break
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpression{Left: left, Op: LogicalOr, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseAnd() (FilterExpression, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if !p.matchKeyword("and") {
			break
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpression{Left: left, Op: LogicalAnd, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseNot() (FilterExpression, error) {
	p.skipWS()
	if p.matchKeyword("not") {
		p.skipWS()
		if p.peek() != '(' {
			return nil, ErrBadFilter
		}
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.peek() != ')' {
			return nil, ErrBadFilter
		}
		p.pos++
		return &NotExpression{Inner: inner}, nil
	}
	return p.parsePrimary()
}

func (p *filterParser) parsePrimary() (FilterExpression, error) {
	p.skipWS()
	if p.peek() == '(' {
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.peek() != ')' {
			return nil, ErrBadFilter
		}
		p.pos++
		return inner, nil
	}
	return p.parseAttrOrValuePath()
}

// parseAttrOrValuePath parses one of two constructs that both start with an
// attribute path:
//   - Value-path filter:  attrName[subFilter]  → ValuePathExpression
//   - Attribute comparison: attrName op value  → AttrExpression
//
// The '[' lookahead disambiguates the two cases after the attribute path is read.
// For the "pr" (present) operator no comparison value follows, so the function
// returns early without calling parseCompValue.
func (p *filterParser) parseAttrOrValuePath() (FilterExpression, error) {
	attrPath, err := p.parseAttrPath()
	if err != nil {
		return nil, err
	}

	p.skipWS()

	if p.peek() == '[' {
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.peek() != ']' {
			return nil, ErrBadFilter
		}
		p.pos++
		return &ValuePathExpression{Path: attrPath, Filter: inner}, nil
	}

	op, err := p.parseCompareOp()
	if err != nil {
		return nil, err
	}

	if op == OpPr {
		return &AttrExpression{Path: attrPath, Operator: OpPr, Value: nil}, nil
	}

	p.skipWS()
	val, err := p.parseCompValue()
	if err != nil {
		return nil, err
	}

	return &AttrExpression{Path: attrPath, Operator: op, Value: val}, nil
}

func (p *filterParser) parseAttrPath() (AttributePath, error) {
	p.skipWS()
	start := p.pos

	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '[' || ch == ']' || ch == '(' || ch == ')' {
			break
		}
		p.pos++
	}

	raw := p.input[start:p.pos]
	if raw == "" {
		return AttributePath{}, ErrBadFilter
	}

	attr, err := ParsePath(raw)
	if err != nil {
		return AttributePath{}, ErrBadFilter
	}
	return attr, nil
}

func (p *filterParser) parseCompareOp() (CompareOp, error) {
	p.skipWS()
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '(' || ch == ')' || ch == '[' || ch == ']' {
			break
		}
		p.pos++
	}
	tok := strings.ToLower(p.input[start:p.pos])
	switch tok {
	case "eq":
		return OpEq, nil
	case "ne":
		return OpNe, nil
	case "co":
		return OpCo, nil
	case "sw":
		return OpSw, nil
	case "ew":
		return OpEw, nil
	case "gt":
		return OpGt, nil
	case "ge":
		return OpGe, nil
	case "lt":
		return OpLt, nil
	case "le":
		return OpLe, nil
	case "pr":
		return OpPr, nil
	default:
		return 0, ErrBadFilter
	}
}

func (p *filterParser) parseCompValue() (interface{}, error) {
	p.skipWS()
	if p.pos >= len(p.input) {
		return nil, ErrBadFilter
	}
	ch := p.input[p.pos]

	switch {
	case ch == '"':
		return p.parseString()
	case p.remaining()[:min(4, len(p.remaining()))] == "null":
		if isWordBoundary(p.input, p.pos+4) {
			p.pos += 4
			return nil, nil
		}
		return nil, ErrBadFilter
	case p.remaining()[:min(4, len(p.remaining()))] == "true":
		if isWordBoundary(p.input, p.pos+4) {
			p.pos += 4
			return true, nil
		}
		return nil, ErrBadFilter
	case len(p.remaining()) >= 5 && p.remaining()[:5] == "false":
		if isWordBoundary(p.input, p.pos+5) {
			p.pos += 5
			return false, nil
		}
		return nil, ErrBadFilter
	case ch == '-' || (ch >= '0' && ch <= '9'):
		return p.parseNumber()
	default:
		return nil, ErrBadFilter
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isWordBoundary(s string, pos int) bool {
	if pos >= len(s) {
		return true
	}
	ch := s[pos]
	return ch == ' ' || ch == '\t' || ch == ')' || ch == ']'
}

func (p *filterParser) parseString() (string, error) {
	if p.peek() != '"' {
		return "", ErrBadFilter
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' {
			p.pos += 2
			continue
		}
		if ch == '"' {
			p.pos++
			raw := p.input[start:p.pos]
			s, err := strconv.Unquote(raw)
			if err != nil {
				return "", ErrBadFilter
			}
			return s, nil
		}
		p.pos++
	}
	return "", ErrBadFilter
}

// parseNumber consumes a JSON-style number literal (integer, decimal, or
// scientific notation) from the current position. It manually scans each
// part — optional leading minus, integer digits, optional fractional part,
// optional exponent — before delegating to strconv.ParseFloat for the actual
// conversion. All numbers are returned as float64 to simplify downstream
// comparison logic.
func (p *filterParser) parseNumber() (interface{}, error) {
	start := p.pos
	if p.pos < len(p.input) && p.input[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch >= '0' && ch <= '9' {
			p.pos++
		} else {
			break
		}
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.input) && (p.input[p.pos] == 'e' || p.input[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
			p.pos++
		}
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
	}
	numStr := p.input[start:p.pos]
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, ErrBadFilter
	}
	return f, nil
}

// matchKeyword attempts to consume the keyword kw (case-insensitive) at the
// current position. It saves the position first so it can roll back on failure.
// The word-boundary check prevents matching "and" inside "android": after the
// candidate text, the next character must be whitespace, a bracket, or end-of-input.
// If the match succeeds, pos is advanced past the keyword and any trailing whitespace.
func (p *filterParser) matchKeyword(kw string) bool {
	saved := p.pos
	p.skipWS()
	if p.pos+len(kw) > len(p.input) {
		p.pos = saved
		return false
	}
	candidate := p.input[p.pos : p.pos+len(kw)]
	if !strings.EqualFold(candidate, kw) {
		p.pos = saved
		return false
	}
	afterPos := p.pos + len(kw)
	if !isWordBoundary(p.input, afterPos) && afterPos < len(p.input) {
		ch := p.input[afterPos]
		if isAlpha(rune(ch)) || isDigit(rune(ch)) || ch == '_' || ch == '-' {
			p.pos = saved
			return false
		}
	}
	p.pos += len(kw)
	p.skipWS()
	return true
}
