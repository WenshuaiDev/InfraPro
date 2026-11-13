// Package observation defines lossless monitoring input contracts. It never
// converts source values through floating point or display precision.
package observation

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"math/big"
	"regexp"
	"strconv"
)

const DecimalScale = 12

var numberPattern = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)

// Coefficient parses decimal/scientific notation exactly. Extra nonzero digits
// are errors, never rounded. Input and exponent limits bound allocation.
func Coefficient(text string, scale int) (*big.Int, error) {
	if len(text) > 1024 || scale < 0 || scale > DecimalScale {
		return nil, errors.New("numeric_input_limit")
	}
	m := numberPattern.FindStringSubmatch(text)
	if m == nil {
		return nil, errors.New("invalid_number")
	}
	exponent := 0
	if m[4] != "" {
		var err error
		exponent, err = strconv.Atoi(m[4])
		if err != nil || exponent < -1024 || exponent > 1024 {
			return nil, errors.New("numeric_exponent_limit")
		}
	}
	c, _ := new(big.Int).SetString(m[2]+m[3], 10)
	shift := exponent - len(m[3]) + scale
	if shift >= 0 {
		c.Mul(c, pow10(shift))
	} else {
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(c, pow10(-shift), r)
		if r.Sign() != 0 {
			return nil, errors.New("numeric_precision_exceeded")
		}
		c = q
	}
	if m[1] == "-" {
		c.Neg(c)
	}
	return c, nil
}

func Integer(text string) (int64, error) {
	c, err := Coefficient(text, 0)
	if err != nil {
		return 0, err
	}
	if !c.IsInt64() {
		return 0, errors.New("integer_out_of_range")
	}
	return c.Int64(), nil
}

func Decimal(text string) (*big.Int, error) {
	c, err := Coefficient(text, DecimalScale)
	if err != nil {
		return nil, err
	}
	if new(big.Int).Abs(c).Cmp(pow10(38)) >= 0 {
		return nil, errors.New("decimal_out_of_range")
	}
	return c, nil
}

func Format(c *big.Int, scale int) string { return monitoringvalue.Format(c, scale) }

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// Quality evaluates exact persisted metric bounds. An unknown nonempty source
// code requires an explicit mapping; bad is never upgraded to suspect.
func Quality(source string, value, minimum, maximum *big.Int) (string, bool, error) {
	if source == "" {
		source = "unmarked"
	}
	switch source {
	case "good", "unmarked", "suspect", "bad":
	default:
		return "", false, errors.New("quality_mapping_required")
	}
	outside := value != nil && ((minimum != nil && value.Cmp(minimum) < 0) || (maximum != nil && value.Cmp(maximum) > 0))
	if outside && source != "bad" {
		return "suspect", true, nil
	}
	return source, outside, nil
}
