package ecu

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Expr is a compiled arithmetic formula such as "x*40", "5.2+x*0.05" or
// "(x-128)/256". Variables are looked up by name at evaluation time.
// Supported: numbers (decimal or 0x hex), + - * / %, & | ^ << >>,
// parentheses, unary minus and the functions abs, min, max, round,
// floor, ceil, sqrt.
type Expr struct {
	src  string
	root node
}

type node interface {
	eval(vars map[string]float64) (float64, error)
}

type numNode float64
type varNode string
type unaryNode struct{ x node }
type binNode struct {
	op   string
	l, r node
}
type callNode struct {
	fn   string
	args []node
}

func (n numNode) eval(map[string]float64) (float64, error) { return float64(n), nil }

func (n varNode) eval(vars map[string]float64) (float64, error) {
	v, ok := vars[string(n)]
	if !ok {
		return 0, fmt.Errorf("unknown variable %q", string(n))
	}
	return v, nil
}

func (n unaryNode) eval(vars map[string]float64) (float64, error) {
	v, err := n.x.eval(vars)
	return -v, err
}

func (n binNode) eval(vars map[string]float64) (float64, error) {
	l, err := n.l.eval(vars)
	if err != nil {
		return 0, err
	}
	r, err := n.r.eval(vars)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case "+":
		return l + r, nil
	case "-":
		return l - r, nil
	case "*":
		return l * r, nil
	case "/":
		if r == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return l / r, nil
	case "%":
		if r == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return math.Mod(l, r), nil
	case "&":
		return float64(int64(l) & int64(r)), nil
	case "|":
		return float64(int64(l) | int64(r)), nil
	case "^":
		return float64(int64(l) ^ int64(r)), nil
	case "<<":
		return float64(int64(l) << uint(r)), nil
	case ">>":
		return float64(int64(l) >> uint(r)), nil
	}
	return 0, fmt.Errorf("unknown operator %s", n.op)
}

func (n callNode) eval(vars map[string]float64) (float64, error) {
	args := make([]float64, len(n.args))
	for i, a := range n.args {
		v, err := a.eval(vars)
		if err != nil {
			return 0, err
		}
		args[i] = v
	}
	need := func(k int) error {
		if len(args) != k {
			return fmt.Errorf("%s needs %d argument(s)", n.fn, k)
		}
		return nil
	}
	switch n.fn {
	case "abs", "round", "floor", "ceil", "sqrt":
		if err := need(1); err != nil {
			return 0, err
		}
		switch n.fn {
		case "abs":
			return math.Abs(args[0]), nil
		case "round":
			return math.Round(args[0]), nil
		case "floor":
			return math.Floor(args[0]), nil
		case "ceil":
			return math.Ceil(args[0]), nil
		default:
			return math.Sqrt(args[0]), nil
		}
	case "min", "max":
		if len(args) == 0 {
			return 0, fmt.Errorf("%s needs arguments", n.fn)
		}
		v := args[0]
		for _, a := range args[1:] {
			if n.fn == "min" {
				v = math.Min(v, a)
			} else {
				v = math.Max(v, a)
			}
		}
		return v, nil
	}
	return 0, fmt.Errorf("unknown function %s", n.fn)
}

// Compile parses a formula.
func Compile(src string) (*Expr, error) {
	p := &parser{src: src}
	p.next()
	root, err := p.parseExpr(0)
	if err != nil {
		return nil, fmt.Errorf("formula %q: %w", src, err)
	}
	if p.tok != "" {
		return nil, fmt.Errorf("formula %q: unexpected %q", src, p.tok)
	}
	return &Expr{src: src, root: root}, nil
}

// MustCompile panics on error (for constants).
func MustCompile(src string) *Expr {
	e, err := Compile(src)
	if err != nil {
		panic(err)
	}
	return e
}

// Eval evaluates the formula.
func (e *Expr) Eval(vars map[string]float64) (float64, error) {
	if e == nil {
		return vars["x"], nil
	}
	return e.root.eval(vars)
}

// String returns the source text.
func (e *Expr) String() string {
	if e == nil {
		return "x"
	}
	return e.src
}

type parser struct {
	src string
	pos int
	tok string
}

func (p *parser) next() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
	if p.pos >= len(p.src) {
		p.tok = ""
		return
	}
	c := p.src[p.pos]
	switch {
	case c >= '0' && c <= '9' || c == '.':
		start := p.pos
		if c == '0' && p.pos+1 < len(p.src) && (p.src[p.pos+1] == 'x' || p.src[p.pos+1] == 'X') {
			p.pos += 2
			for p.pos < len(p.src) && strings.ContainsRune("0123456789abcdefABCDEF", rune(p.src[p.pos])) {
				p.pos++
			}
		} else {
			for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.' ||
				p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
				p.pos++
			}
		}
		p.tok = p.src[start:p.pos]
	case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		start := p.pos
		for p.pos < len(p.src) && (p.src[p.pos] == '_' || p.src[p.pos] >= 'a' && p.src[p.pos] <= 'z' ||
			p.src[p.pos] >= 'A' && p.src[p.pos] <= 'Z' || p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
			p.pos++
		}
		p.tok = p.src[start:p.pos]
	case c == '<' || c == '>':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == c {
			p.tok = p.src[p.pos : p.pos+2]
			p.pos += 2
			return
		}
		p.tok = string(c)
		p.pos++
	default:
		p.tok = string(c)
		p.pos++
	}
}

var precedence = map[string]int{"|": 1, "^": 2, "&": 3, "<<": 4, ">>": 4, "+": 5, "-": 5, "*": 6, "/": 6, "%": 6}

func (p *parser) parseExpr(minPrec int) (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		prec, ok := precedence[p.tok]
		if !ok || prec < minPrec {
			return left, nil
		}
		op := p.tok
		p.next()
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = binNode{op: op, l: left, r: right}
	}
}

func (p *parser) parseUnary() (node, error) {
	if p.tok == "-" {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{x}, nil
	}
	if p.tok == "+" {
		p.next()
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	tok := p.tok
	switch {
	case tok == "":
		return nil, fmt.Errorf("unexpected end")
	case tok == "(":
		p.next()
		n, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.tok != ")" {
			return nil, fmt.Errorf("missing )")
		}
		p.next()
		return n, nil
	case tok[0] >= '0' && tok[0] <= '9' || tok[0] == '.':
		var v float64
		var err error
		if strings.HasPrefix(tok, "0x") || strings.HasPrefix(tok, "0X") {
			var u uint64
			u, err = strconv.ParseUint(tok[2:], 16, 64)
			v = float64(u)
		} else {
			v, err = strconv.ParseFloat(tok, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("bad number %q", tok)
		}
		p.next()
		return numNode(v), nil
	case tok[0] == '_' || tok[0] >= 'a' && tok[0] <= 'z' || tok[0] >= 'A' && tok[0] <= 'Z':
		p.next()
		if p.tok == "(" {
			p.next()
			var args []node
			for p.tok != ")" {
				a, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if p.tok == "," {
					p.next()
				} else if p.tok != ")" {
					return nil, fmt.Errorf("expected , or ) in call to %s", tok)
				}
			}
			p.next()
			return callNode{fn: strings.ToLower(tok), args: args}, nil
		}
		return varNode(tok), nil
	}
	return nil, fmt.Errorf("unexpected %q", tok)
}
