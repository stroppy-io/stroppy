// Package dsqgen is a lean port of the TPC-DS query generator (dsqgen). Unlike
// the C tool it does NOT reproduce dsqgen's exact RNG stream; it parses the
// query templates' `define` headers and produces VALID, scale-correct
// substitution values with its own seeded RNG, drawing from the same
// distributions and rowcounts the data uses so that generated query parameters
// hit real rows. Query generation is independent of data generation: the
// templates only need parameter domains (ranges / distributions / rowcount at
// scale), not the data's internal correlations.
//
// This file is the template parser: it turns a `.tpl` header into an ordered
// list of parameter definitions, each with a small expression AST. The query
// body (everything after the defines) is kept verbatim for later [PLACEHOLDER]
// injection.
package dsqgen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExprKind tags a node in a define's right-hand-side expression.
type ExprKind int

const (
	ExprCall   ExprKind = iota // fn(args...)        e.g. random(1, 100, uniform)
	ExprRef                    // [NAME]            reference to another define
	ExprInt                    // integer literal
	ExprStr                    // "string" literal
	ExprWord                   // bareword          e.g. uniform, sales, fips_county
	ExprTuple                  // {expr, expr, ...}  text() weighted entry
	ExprBinary                 // a OP b            OP in Word: + - * /  (+ also concatenates date strings)
)

// Expr is one node of a define's right-hand side. Only the fields relevant to
// Kind are populated.
type Expr struct {
	Kind ExprKind
	Func string // ExprCall: function name
	Word string // ExprWord/ExprRef: identifier (ExprRef: base name without .K suffix)
	Str  string // ExprStr
	Int  int64  // ExprInt
	Idx  int    // ExprRef: 1-based element of a multi-value (ulist) define; 0 = first
	Args []Expr // ExprCall/ExprTuple operands; ExprBinary: exactly two
}

// Define is one `define NAME = RHS;` directive.
type Define struct {
	Name string
	RHS  Expr
	Deps []string // names of [REF]s anywhere in RHS, for dependency ordering
}

// Template is a parsed query template: its ordered defines and the verbatim
// SQL body with [PLACEHOLDER] markers still in place.
type Template struct {
	Name    string
	Defines []Define
	Body    string
}

var defineRe = regexp.MustCompile(`(?s)\bdefine\s+([A-Za-z_]\w*)\s*=\s*(.*?);`)

// ParseTemplate parses a single template. name is the logical template name
// (e.g. "query1"); src is the full .tpl contents.
func ParseTemplate(name, src string) (*Template, error) {
	t := &Template{Name: name}

	locs := defineRe.FindAllStringSubmatchIndex(src, -1)
	bodyStart := 0
	for _, loc := range locs {
		dn := src[loc[2]:loc[3]]
		rhs := strings.TrimSpace(src[loc[4]:loc[5]])
		// The _LIMIT define is a bare int consumed by the dialect macros; keep it
		// like any other so the body's [_LIMITx] placeholders resolve.
		e, err := parseExpr(rhs)
		if err != nil {
			return nil, fmt.Errorf("%s: define %s: %w", name, dn, err)
		}
		t.Defines = append(t.Defines, Define{Name: dn, RHS: e, Deps: collectRefs(e, nil)})
		if loc[1] > bodyStart {
			bodyStart = loc[1]
		}
	}
	t.Body = strings.TrimLeft(src[bodyStart:], "\r\n")

	return t, nil
}

func collectRefs(e Expr, acc []string) []string {
	if e.Kind == ExprRef {
		acc = append(acc, e.Word)
	}
	for _, a := range e.Args {
		acc = collectRefs(a, acc)
	}
	return acc
}

// --- recursive-descent parser over the small define RHS grammar ---
//
//	expr    := add
//	add     := mul (('+'|'-') mul)*
//	mul     := primary (('*'|'/') primary)*
//	primary := INT | STRING | '[' NAME ']' | NAME | NAME '(' args ')' | '{' args '}'
//	args    := expr (',' expr)*

type parser struct {
	s   string
	pos int
}

func parseExpr(s string) (Expr, error) {
	p := &parser{s: s}
	e, err := p.parseAdd()
	if err != nil {
		return Expr{}, err
	}
	p.skipWS()
	if p.pos != len(p.s) {
		return Expr{}, fmt.Errorf("trailing input %q", p.s[p.pos:])
	}
	return e, nil
}

func (p *parser) skipWS() {
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		break
	}
}

func (p *parser) parseAdd() (Expr, error) {
	left, err := p.parseMul()
	if err != nil {
		return Expr{}, err
	}
	for {
		p.skipWS()
		if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
			op := string(p.s[p.pos])
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return Expr{}, err
			}
			left = Expr{Kind: ExprBinary, Word: op, Args: []Expr{left, right}}
			continue
		}
		break
	}
	return left, nil
}

func (p *parser) parseMul() (Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return Expr{}, err
	}
	for {
		p.skipWS()
		if p.pos < len(p.s) && (p.s[p.pos] == '*' || p.s[p.pos] == '/') {
			op := string(p.s[p.pos])
			p.pos++
			right, err := p.parsePrimary()
			if err != nil {
				return Expr{}, err
			}
			left = Expr{Kind: ExprBinary, Word: op, Args: []Expr{left, right}}
			continue
		}
		break
	}
	return left, nil
}

func (p *parser) parsePrimary() (Expr, error) {
	p.skipWS()
	if p.pos >= len(p.s) {
		return Expr{}, fmt.Errorf("unexpected end of expression")
	}
	c := p.s[p.pos]
	switch {
	case c == '"':
		return p.parseString()
	case c == '[':
		return p.parseRef()
	case c == '{':
		return p.parseTuple()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseInt()
	case isIdentStart(c):
		return p.parseWordOrCall()
	default:
		return Expr{}, fmt.Errorf("unexpected char %q", string(c))
	}
}

func (p *parser) parseString() (Expr, error) {
	p.pos++ // opening quote
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] != '"' {
		p.pos++
	}
	if p.pos >= len(p.s) {
		return Expr{}, fmt.Errorf("unterminated string")
	}
	str := p.s[start:p.pos]
	p.pos++ // closing quote
	return Expr{Kind: ExprStr, Str: str}, nil
}

func (p *parser) parseRef() (Expr, error) {
	p.pos++ // '['
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] != ']' {
		p.pos++
	}
	if p.pos >= len(p.s) {
		return Expr{}, fmt.Errorf("unterminated [ref]")
	}
	name := strings.TrimSpace(p.s[start:p.pos])
	p.pos++ // ']'
	idx := 0
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		if k, err := strconv.Atoi(name[dot+1:]); err == nil {
			idx = k
			name = name[:dot]
		}
	}
	return Expr{Kind: ExprRef, Word: name, Idx: idx}, nil
}

func (p *parser) parseTuple() (Expr, error) {
	p.pos++ // '{'
	args, err := p.parseArgs('}')
	if err != nil {
		return Expr{}, err
	}
	return Expr{Kind: ExprTuple, Args: args}, nil
}

func (p *parser) parseInt() (Expr, error) {
	start := p.pos
	if p.s[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	v, err := strconv.ParseInt(p.s[start:p.pos], 10, 64)
	if err != nil {
		return Expr{}, fmt.Errorf("bad int %q: %w", p.s[start:p.pos], err)
	}
	return Expr{Kind: ExprInt, Int: v}, nil
}

func (p *parser) parseWordOrCall() (Expr, error) {
	start := p.pos
	for p.pos < len(p.s) && isIdentPart(p.s[p.pos]) {
		p.pos++
	}
	word := p.s[start:p.pos]
	p.skipWS()
	if p.pos < len(p.s) && p.s[p.pos] == '(' {
		p.pos++ // '('
		args, err := p.parseArgs(')')
		if err != nil {
			return Expr{}, err
		}
		return Expr{Kind: ExprCall, Func: word, Args: args}, nil
	}
	return Expr{Kind: ExprWord, Word: word}, nil
}

func (p *parser) parseArgs(close byte) ([]Expr, error) {
	var args []Expr
	p.skipWS()
	if p.pos < len(p.s) && p.s[p.pos] == close {
		p.pos++
		return args, nil
	}
	for {
		a, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		p.skipWS()
		if p.pos >= len(p.s) {
			return nil, fmt.Errorf("missing closing %q", string(close))
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
			continue
		case close:
			p.pos++
			return args, nil
		default:
			return nil, fmt.Errorf("expected ',' or %q, got %q", string(close), string(p.s[p.pos]))
		}
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}
