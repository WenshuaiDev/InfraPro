package observation

import (
	"errors"
	"math/big"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// Moments can be combined without losing small differences between large
// values. Coefficients all use DecimalScale, including integer observations.
type Moments struct{ N, S1, S2 big.Int }

func (m *Moments) Add(c *big.Int) {
	m.N.Add(&m.N, big.NewInt(1))
	m.S1.Add(&m.S1, c)
	m.S2.Add(&m.S2, new(big.Int).Mul(c, c))
}

func (m *Moments) Merge(other *Moments) {
	m.N.Add(&m.N, &other.N)
	m.S1.Add(&m.S1, &other.S1)
	m.S2.Add(&m.S2, &other.S2)
}

// Results rounds exact rational states using half-even. The square root is
// rounded by an exact integer midpoint comparison, so even a rounding boundary
// needs no floating-point approximation (stronger than a 96-digit float).
func (m *Moments) Results(digits int) (mean, populationStddev *string, err error) {
	if digits < 0 || digits > 18 || m.N.Sign() < 0 {
		return nil, nil, errors.New("invalid_statistics_state")
	}
	if m.N.Sign() == 0 {
		return nil, nil, nil
	}
	num := new(big.Int).Mul(&m.N, &m.S2)
	num.Sub(num, new(big.Int).Mul(&m.S1, &m.S1))
	if num.Sign() < 0 {
		return nil, nil, errors.New("invalid_variance_state")
	}
	meanDen := new(big.Int).Mul(&m.N, pow10(DecimalScale))
	mn := monitoringvalue.RoundHalfEven(new(big.Rat).SetFrac(&m.S1, meanDen), digits)
	den := new(big.Int).Mul(&m.N, &m.N)
	den.Mul(den, pow10(2*DecimalScale))
	q, err := monitoringvalue.RoundSqrt(new(big.Rat).SetFrac(num, den), digits)
	if err != nil {
		return nil, nil, err
	}

	a, b := Format(mn, digits), Format(q, digits)
	return &a, &b, nil
}
