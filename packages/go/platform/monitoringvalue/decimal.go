// Package monitoringvalue contains the lossless numeric contract shared by raw
// monitoring statistics and derived calculation. No formal value uses float64.
package monitoringvalue

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

const MaxBits = 16384

var ErrBudget = errors.New("numeric_budget_exceeded")
var decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

func Power10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// ParseDecimal is the rule/parameter protocol, not the raw import protocol:
// scientific notation and non-finite values are deliberately not admitted.
func ParseDecimal(s string) (*big.Rat, error) {
	if len(s) > 1024 || !decimalPattern.MatchString(s) {
		return nil, errors.New("invalid_decimal")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("invalid_decimal")
	}
	return r, Check(r)
}
func Check(r *big.Rat) error {
	if r == nil || r.Num().BitLen() > MaxBits || r.Denom().BitLen() > MaxBits {
		return ErrBudget
	}
	return nil
}
func Format(c *big.Int, scale int) string {
	s := new(big.Int).Abs(c).String()
	if scale > 0 {
		if len(s) <= scale {
			s = strings.Repeat("0", scale+1-len(s)) + s
		}
		s = s[:len(s)-scale] + "." + s[len(s)-scale:]
	}
	if c.Sign() < 0 {
		s = "-" + s
	}
	return s
}
func CanonicalDecimal(s string) (string, error) {
	r, err := ParseDecimal(s)
	if err != nil {
		return "", err
	}
	scale := 0
	if p := strings.IndexByte(s, '.'); p >= 0 {
		scale = len(s) - p - 1
	}
	text := Format(RoundHalfEven(r, scale), scale)
	if scale > 0 {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text, nil
}

// RoundHalfEven returns a coefficient at scale, using an exact midpoint test.
func RoundHalfEven(r *big.Rat, scale int) *big.Int {
	n := new(big.Int).Mul(r.Num(), Power10(scale))
	negative := n.Sign() < 0
	n.Abs(n)
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n, r.Denom(), rem)
	cmp := new(big.Int).Lsh(rem, 1).Cmp(r.Denom())
	if cmp > 0 || cmp == 0 && q.Bit(0) == 1 {
		q.Add(q, big.NewInt(1))
	}
	if negative {
		q.Neg(q)
	}
	return q
}

// RoundSqrt rounds sqrt(r) exactly, even when the result lies at a midpoint.
func RoundSqrt(r *big.Rat, scale int) (*big.Int, error) {
	if r.Sign() < 0 {
		return nil, errors.New("invalid_square_root")
	}
	n := new(big.Int).Mul(r.Num(), Power10(2*scale))
	d := r.Denom()
	q := new(big.Int).Sqrt(new(big.Int).Quo(n, d))
	boundary := new(big.Int).Add(new(big.Int).Lsh(new(big.Int).Set(q), 1), big.NewInt(1))
	boundary.Mul(boundary, boundary).Mul(boundary, d)
	cmp := new(big.Int).Lsh(n, 2).Cmp(boundary)
	if cmp > 0 || cmp == 0 && q.Bit(0) == 1 {
		q.Add(q, big.NewInt(1))
	}
	return q, nil
}

// Moments keeps exact rational moments across different published scales.
// Merge is associative; quantiles are intentionally not part of this state.
type Moments struct {
	N            uint64
	Sum, Squares big.Rat
}

func (m *Moments) Add(r *big.Rat) error {
	if m.N == ^uint64(0) {
		return ErrBudget
	}
	m.N++
	m.Sum.Add(&m.Sum, r)
	m.Squares.Add(&m.Squares, new(big.Rat).Mul(r, r))
	if err := Check(&m.Sum); err != nil {
		return err
	}
	return Check(&m.Squares)
}
func (m *Moments) Remove(r *big.Rat) error {
	if m.N == 0 {
		return errors.New("invalid_statistics_state")
	}
	m.N--
	m.Sum.Sub(&m.Sum, r)
	m.Squares.Sub(&m.Squares, new(big.Rat).Mul(r, r))
	return nil
}
func (m *Moments) Merge(other *Moments) error {
	if ^uint64(0)-m.N < other.N {
		return ErrBudget
	}
	m.N += other.N
	m.Sum.Add(&m.Sum, &other.Sum)
	m.Squares.Add(&m.Squares, &other.Squares)
	if err := Check(&m.Sum); err != nil {
		return err
	}
	return Check(&m.Squares)
}
func (m *Moments) Mean() *big.Rat {
	if m.N == 0 {
		return nil
	}
	return new(big.Rat).Quo(&m.Sum, new(big.Rat).SetInt(new(big.Int).SetUint64(m.N)))
}
func (m *Moments) Variance() (*big.Rat, error) {
	if m.N == 0 {
		return nil, nil
	}
	n := new(big.Rat).SetInt(new(big.Int).SetUint64(m.N))
	v := new(big.Rat).Sub(new(big.Rat).Mul(n, &m.Squares), new(big.Rat).Mul(&m.Sum, &m.Sum))
	if v.Sign() < 0 {
		return nil, errors.New("invalid_variance_state")
	}
	v.Quo(v, new(big.Rat).Mul(n, n))
	return v, Check(v)
}
