package monitoringvalue

import (
	"context"
	"errors"
	"math/big"
)

var (
	ErrDivisionByZero = errors.New("division_by_zero")
	ErrPrecision      = errors.New("precision_unresolved")
	ErrRange          = errors.New("numeric_out_of_range")
	ErrUnderflow      = errors.New("numeric_underflow")
)

// Number is immutable: rational operations stay exact and irrational nodes
// keep a refinable expression. An output is accepted only when both enclosing
// endpoints round to the same coefficient. Intermediate nodes never round.
type Number struct {
	exact *big.Rat
	op    string
	a, b  *Number
}

func Rational(r *big.Rat) (*Number, error) {
	if err := Check(r); err != nil {
		return nil, err
	}
	return &Number{exact: new(big.Rat).Set(r)}, nil
}
func Integer(n int64) *Number { return &Number{exact: big.NewRat(n, 1)} }
func Decimal(s string) (*Number, error) {
	r, err := ParseDecimal(s)
	if err != nil {
		return nil, err
	}
	return Rational(r)
}
func (n *Number) Exact() (*big.Rat, bool) {
	if n.exact == nil {
		return nil, false
	}
	return new(big.Rat).Set(n.exact), true
}
func same(a, b *Number) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.exact != nil && b.exact != nil {
		return a.exact.Cmp(b.exact) == 0
	}
	return a.op != "" && a.op == b.op && same(a.a, b.a) && same(a.b, b.b)
}
func Binary(op string, a, b *Number) (*Number, error) {
	if a == nil || b == nil {
		return nil, errors.New("invalid_numeric_operand")
	}
	if op == "subtract" && same(a, b) {
		return Integer(0), nil
	}
	if op == "multiply" && a.op == "sqrt" && same(a, b) {
		return a.a, nil
	}

	if a.exact != nil && b.exact != nil {
		r := new(big.Rat)
		switch op {
		case "add":
			r.Add(a.exact, b.exact)
		case "subtract":
			r.Sub(a.exact, b.exact)
		case "multiply":
			r.Mul(a.exact, b.exact)
		case "divide":
			if b.exact.Sign() == 0 {
				return nil, ErrDivisionByZero
			}
			r.Quo(a.exact, b.exact)
		case "min":
			r.Set(a.exact)
			if b.exact.Cmp(r) < 0 {
				r.Set(b.exact)
			}
		case "max":
			r.Set(a.exact)
			if b.exact.Cmp(r) > 0 {
				r.Set(b.exact)
			}
		default:
			return nil, errors.New("invalid_numeric_operator")
		}
		return Rational(r)
	}
	switch op {
	case "add", "subtract", "multiply", "divide", "min", "max":
	default:
		return nil, errors.New("invalid_numeric_operator")
	}
	if op == "divide" && b.exact != nil && b.exact.Sign() == 0 {
		return nil, ErrDivisionByZero
	}
	return &Number{op: op, a: a, b: b}, nil
}
func Unary(op string, a *Number) (*Number, error) {
	if a == nil {
		return nil, errors.New("invalid_numeric_operand")
	}
	if op != "abs" && op != "sqrt" {
		return nil, errors.New("invalid_numeric_operator")
	}
	if a.exact != nil {
		if op == "abs" {
			return Rational(new(big.Rat).Abs(a.exact))
		}
		if a.exact.Sign() < 0 {
			return nil, errors.New("invalid_square_root")
		}
		n, d := new(big.Int).Sqrt(a.exact.Num()), new(big.Int).Sqrt(a.exact.Denom())
		if new(big.Int).Mul(n, n).Cmp(a.exact.Num()) == 0 && new(big.Int).Mul(d, d).Cmp(a.exact.Denom()) == 0 {
			return Rational(new(big.Rat).SetFrac(n, d))
		}
	}
	return &Number{op: op, a: a}, nil
}

type interval struct{ lo, hi *big.Rat }

func point(r *big.Rat) interval { return interval{r, r} }
func boundsSqrt(r *big.Rat, digits int) (interval, error) {
	if r.Sign() < 0 {
		return interval{}, errors.New("invalid_square_root")
	}
	// Add fractional digits for very small magnitudes, preserving at least the
	// requested number of significant decimal digits without float logarithms.
	extra := (r.Denom().BitLen()-r.Num().BitLen())/6 + 1
	if extra < 0 {
		extra = 0
	}
	digits += extra
	if digits*7 > MaxBits {
		return interval{}, ErrPrecision
	}
	p := Power10(digits)
	num := new(big.Int).Mul(r.Num(), new(big.Int).Mul(p, p))
	q := new(big.Int).Sqrt(new(big.Int).Quo(num, r.Denom()))
	lo := new(big.Rat).SetFrac(q, p)
	if new(big.Rat).Mul(lo, lo).Cmp(r) == 0 {
		return point(lo), nil
	}
	hi := new(big.Rat).SetFrac(new(big.Int).Add(q, big.NewInt(1)), p)
	return interval{lo, hi}, nil
}
func minRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}
func maxRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) > 0 {
		return a
	}
	return b
}
func (n *Number) bounds(ctx context.Context, digits int, cache map[*Number]interval) (interval, error) {
	if err := ctx.Err(); err != nil {
		return interval{}, err
	}
	if v, ok := cache[n]; ok {
		return v, nil
	}
	if n.exact != nil {
		return point(n.exact), nil
	}
	a, err := n.a.bounds(ctx, digits, cache)
	if err != nil {
		return interval{}, err
	}
	var v interval
	if n.op == "sqrt" {
		if a.lo.Sign() < 0 {
			return v, ErrPrecision
		}
		low, e := boundsSqrt(a.lo, digits)
		if e != nil {
			return v, e
		}
		high, e := boundsSqrt(a.hi, digits)
		if e != nil {
			return v, e
		}
		v = interval{low.lo, high.hi}
	} else if n.op == "abs" {
		if a.lo.Sign() >= 0 {
			v = a
		} else if a.hi.Sign() <= 0 {
			v = interval{new(big.Rat).Neg(a.hi), new(big.Rat).Neg(a.lo)}
		} else {
			v = interval{new(big.Rat), maxRat(new(big.Rat).Neg(a.lo), a.hi)}
		}
	} else {
		b, e := n.b.bounds(ctx, digits, cache)
		if e != nil {
			return v, e
		}
		switch n.op {
		case "add":
			v = interval{new(big.Rat).Add(a.lo, b.lo), new(big.Rat).Add(a.hi, b.hi)}
		case "subtract":
			v = interval{new(big.Rat).Sub(a.lo, b.hi), new(big.Rat).Sub(a.hi, b.lo)}
		case "min":
			v = interval{minRat(a.lo, b.lo), minRat(a.hi, b.hi)}
		case "max":
			v = interval{maxRat(a.lo, b.lo), maxRat(a.hi, b.hi)}
		case "multiply", "divide":
			if n.op == "divide" {
				if b.lo.Sign() == 0 && b.hi.Sign() == 0 {
					return v, ErrDivisionByZero
				}
				if b.lo.Sign() <= 0 && b.hi.Sign() >= 0 {
					return v, ErrPrecision
				}
				b = interval{new(big.Rat).Inv(b.hi), new(big.Rat).Inv(b.lo)}
			}
			for _, x := range []*big.Rat{a.lo, a.hi} {
				for _, y := range []*big.Rat{b.lo, b.hi} {
					p := new(big.Rat).Mul(x, y)
					if v.lo == nil {
						v = point(p)
					} else {
						v.lo = minRat(v.lo, p)
						v.hi = maxRat(v.hi, p)
					}
				}
			}
		}
	}
	if err := Check(v.lo); err != nil {
		return v, err
	}
	if err := Check(v.hi); err != nil {
		return v, err
	}
	cache[n] = v
	return v, nil
}

type Encoded struct {
	Coefficient *big.Int
	Value       string
	Scale       int
	Rounded     bool
}

func encode(c *big.Int, scale int, rounded, nonzero bool) (Encoded, error) {
	if new(big.Int).Abs(c).Cmp(Power10(76)) >= 0 {
		return Encoded{}, ErrRange
	}
	if c.Sign() == 0 && nonzero {
		return Encoded{}, ErrUnderflow
	}
	return Encoded{c, Format(c, scale), scale, rounded}, nil
}
func (n *Number) Encode(ctx context.Context, scale int) (Encoded, error) {
	if scale < 0 || scale > 60 {
		return Encoded{}, errors.New("invalid_numeric_contract")
	}
	if err := ctx.Err(); err != nil {
		return Encoded{}, err
	}
	if n.exact != nil {
		c := RoundHalfEven(n.exact, scale)
		rounded := new(big.Rat).SetFrac(c, Power10(scale)).Cmp(n.exact) != 0
		return encode(c, scale, rounded, n.exact.Sign() != 0)
	}
	if n.op == "sqrt" && n.a.exact != nil {
		c, err := RoundSqrt(n.a.exact, scale)
		if err != nil {
			return Encoded{}, err
		}
		r := new(big.Rat).SetFrac(c, Power10(scale))
		rounded := new(big.Rat).Mul(r, r).Cmp(n.a.exact) != 0
		return encode(c, scale, rounded, n.a.exact.Sign() != 0)
	}
	for digits := 96; digits <= 1536; digits *= 2 {
		b, err := n.bounds(ctx, digits, map[*Number]interval{})
		if errors.Is(err, ErrPrecision) {
			continue
		}
		if err != nil {
			return Encoded{}, err
		}
		lo, hi := RoundHalfEven(b.lo, scale), RoundHalfEven(b.hi, scale)
		if lo.Cmp(hi) != 0 {
			continue
		}
		nonzero := b.lo.Sign() > 0 || b.hi.Sign() < 0
		if lo.Sign() == 0 && !nonzero && b.lo.Cmp(b.hi) != 0 {
			continue
		}
		exact := b.lo.Cmp(b.hi) == 0 && new(big.Rat).SetFrac(lo, Power10(scale)).Cmp(b.lo) == 0
		return encode(lo, scale, !exact, nonzero)
	}
	return Encoded{}, ErrPrecision
}

// Compare orders exact values or disjoint proven bounds. Unresolved equality
// fails explicitly; it never lets an approximate comparison reorder quantiles.
func (n *Number) Compare(ctx context.Context, other *Number) (int, error) {
	if same(n, other) {
		return 0, nil
	}
	if n.exact != nil && other.exact != nil {
		return n.exact.Cmp(other.exact), nil
	}
	for digits := 96; digits <= 1536; digits *= 2 {
		cache := map[*Number]interval{}
		a, e := n.bounds(ctx, digits, cache)
		if e != nil {
			return 0, e
		}
		b, e := other.bounds(ctx, digits, cache)
		if e != nil {
			return 0, e
		}
		if a.hi.Cmp(b.lo) < 0 {
			return -1, nil
		}
		if a.lo.Cmp(b.hi) > 0 {
			return 1, nil
		}
	}
	return 0, ErrPrecision
}
