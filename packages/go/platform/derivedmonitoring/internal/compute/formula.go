package compute

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type expression struct {
	op, token string
	args      []*expression
	quantity  Quantity
}
type parser struct {
	text       string
	pos, count int
	limits     Limits
	vars       map[string]Quantity
}

func (p *parser) space() {
	for p.pos < len(p.text) && unicode.IsSpace(rune(p.text[p.pos])) {
		p.pos++
	}
}
func (p *parser) take(c byte) bool {
	p.space()
	if p.pos < len(p.text) && p.text[p.pos] == c {
		p.pos++
		return true
	}
	return false
}
func (p *parser) fail() error { return fmt.Errorf("unsupported expression at byte %d", p.pos) }
func (p *parser) make(op, token string, args ...*expression) (*expression, error) {
	p.count++
	if p.count > p.limits.ExpressionNodes {
		return nil, fmt.Errorf("expression budget exceeded")
	}
	e := &expression{op: op, token: token, args: args}
	var err error
	switch op {
	case "constant":
		e.quantity = scalar()
		_, err = monitoringvalue.ParseDecimal(token)
	case "variable":
		var ok bool
		e.quantity, ok = p.vars[token]
		if !ok {
			err = fmt.Errorf("undeclared variable %s", token)
		}
	case "abs":
		e.quantity, err = quantityUnary(op, args[0].quantity, Params{})
	default:
		e.quantity = args[0].quantity
		for _, a := range args[1:] {
			e.quantity, err = quantityBinary(op, e.quantity, a.quantity)
			if err != nil {
				break
			}
		}
	}
	return e, err
}
func (p *parser) sum(depth int) (*expression, error) {
	a, err := p.product(depth)
	if err != nil {
		return nil, err
	}
	for {
		op := ""
		if p.take('+') {
			op = "add"
		} else if p.take('-') {
			op = "subtract"
		} else {
			return a, nil
		}
		b, e := p.product(depth)
		if e != nil {
			return nil, e
		}
		a, err = p.make(op, "", a, b)
		if err != nil {
			return nil, err
		}
	}
}
func (p *parser) product(depth int) (*expression, error) {
	a, err := p.atom(depth)
	if err != nil {
		return nil, err
	}
	for {
		op := ""
		if p.take('*') {
			op = "multiply"
		} else if p.take('/') {
			op = "divide"
		} else {
			return a, nil
		}
		b, e := p.atom(depth)
		if e != nil {
			return nil, e
		}
		a, err = p.make(op, "", a, b)
		if err != nil {
			return nil, err
		}
	}
}
func (p *parser) atom(depth int) (*expression, error) {
	if depth > p.limits.ExpressionDepth {
		return nil, fmt.Errorf("expression depth exceeded")
	}
	if p.take('+') {
		return p.atom(depth + 1)
	}
	if p.take('-') {
		a, err := p.atom(depth + 1)
		if err != nil {
			return nil, err
		}
		c, err := p.make("constant", "-1")
		if err != nil {
			return nil, err
		}
		return p.make("multiply", "", c, a)
	}
	if p.take('(') {
		a, err := p.sum(depth + 1)
		if err != nil {
			return nil, err
		}
		if !p.take(')') {
			return nil, p.fail()
		}
		return a, nil
	}
	p.space()
	start := p.pos
	if start == len(p.text) {
		return nil, p.fail()
	}
	if p.text[start] >= '0' && p.text[start] <= '9' {
		for p.pos < len(p.text) && ((p.text[p.pos] >= '0' && p.text[p.pos] <= '9') || p.text[p.pos] == '.') {
			p.pos++
		}
		return p.make("constant", p.text[start:p.pos])
	}
	for p.pos < len(p.text) {
		c := p.text[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (p.pos > start && c >= '0' && c <= '9') {
			p.pos++
		} else {
			break
		}
	}
	if start == p.pos {
		return nil, p.fail()
	}
	name := p.text[start:p.pos]
	if !p.take('(') {
		return p.make("variable", name)
	}
	if name != "abs" && name != "min" && name != "max" {
		return nil, p.fail()
	}
	args := []*expression{}
	for {
		a, err := p.sum(depth + 1)
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if len(args) > 16 {
			return nil, p.fail()
		}
		if p.take(')') {
			break
		}
		if !p.take(',') {
			return nil, p.fail()
		}
	}
	if name == "abs" && len(args) != 1 || name != "abs" && len(args) < 2 {
		return nil, p.fail()
	}
	return p.make(name, "", args...)
}
func expressionDepth(e *expression) int {
	d := 0
	for _, a := range e.args {
		if n := expressionDepth(a); n > d {
			d = n
		}
	}
	return d + 1
}
func parseExpression(text string, vars map[string]Quantity, limits Limits) (*expression, error) {
	if len(text) == 0 || len(text) > limits.ExpressionLength {
		return nil, fmt.Errorf("expression length exceeds limit")
	}
	p := parser{text: text, limits: limits, vars: vars}
	e, err := p.sum(1)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.pos != len(text) || expressionDepth(e) > limits.ExpressionDepth {
		return nil, p.fail()
	}
	return e, nil
}
func (e *expression) evaluate(vars map[string]*monitoringvalue.Number) (*monitoringvalue.Number, error) {
	if e.op == "constant" {
		return monitoringvalue.Decimal(e.token)
	}
	if e.op == "variable" {
		v, ok := vars[e.token]
		if !ok {
			return nil, fmt.Errorf("missing variable %s", e.token)
		}
		return v, nil
	}
	a, err := e.args[0].evaluate(vars)
	if err != nil {
		return nil, err
	}
	if e.op == "abs" {
		return monitoringvalue.Unary("abs", a)
	}
	for _, child := range e.args[1:] {
		b, err := child.evaluate(vars)
		if err != nil {
			return nil, err
		}
		a, err = monitoringvalue.Binary(e.op, a, b)
		if err != nil {
			return nil, err
		}
	}
	return a, nil
}
func (e *expression) canonical() string {
	if e.op == "constant" {
		s, _ := monitoringvalue.CanonicalDecimal(e.token)
		return s
	}
	if e.op == "variable" {
		return e.token
	}
	parts := []string{e.op}
	for _, a := range e.args {
		parts = append(parts, a.canonical())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func (e *expression) render() string {
	if e.op == "constant" {
		s, _ := monitoringvalue.CanonicalDecimal(e.token)
		return s
	}
	if e.op == "variable" {
		return e.token
	}
	parts := make([]string, len(e.args))
	for i, a := range e.args {
		parts[i] = a.render()
	}
	switch e.op {
	case "abs", "min", "max":
		return e.op + "(" + strings.Join(parts, ",") + ")"
	}
	symbol := map[string]string{"add": "+", "subtract": "-", "multiply": "*", "divide": "/"}[e.op]
	return "(" + strings.Join(parts, symbol) + ")"
}
