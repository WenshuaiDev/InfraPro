package monitoringvalue

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
)

func number(t *testing.T, s string) *Number {
	t.Helper()
	n, e := Decimal(s)
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func TestOutputContract(t *testing.T) {
	for _, tc := range []struct {
		in    string
		scale int
		want  string
		err   error
	}{
		{"2.5", 0, "2", nil}, {"3.5", 0, "4", nil}, {"-2.5", 0, "-2", nil}, {"-3.5", 0, "-4", nil},
		{"-0.0000", 4, "0.0000", nil}, {"0.00000000000000000000000025", 36, "0.000000000000000000000000250000000000", nil},
		{"0.00000000000000000000000025", 18, "", ErrUnderflow}, {strings.Repeat("9", 76), 0, strings.Repeat("9", 76), nil},
		{strings.Repeat("9", 76) + ".5", 0, "", ErrRange},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := number(t, tc.in).Encode(t.Context(), tc.scale)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error %v want %v", err, tc.err)
			}
			if err == nil && got.Value != tc.want {
				t.Fatalf("%s != %s", got.Value, tc.want)
			}
		})
	}
}
func TestTinyVarianceAndPartitionMerge(t *testing.T) {
	var all, left, right Moments
	for i, s := range []string{"10", "10.000000000001"} {
		r, e := ParseDecimal(s)
		if e != nil {
			t.Fatal(e)
		}
		if e = all.Add(r); e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			_ = left.Add(r)
		} else {
			_ = right.Add(r)
		}
	}
	if e := left.Merge(&right); e != nil {
		t.Fatal(e)
	}
	variance, e := all.Variance()
	if e != nil {
		t.Fatal(e)
	}
	other, e := left.Variance()
	if e != nil || variance.Cmp(other) != 0 {
		t.Fatal("partition changed result")
	}
	want, _ := ParseDecimal("0.00000000000000000000000025")
	if variance.Cmp(want) != 0 {
		t.Fatal(variance)
	}
	root, e := RoundSqrt(variance, 36)
	if e != nil || Format(root, 36) != "0.000000000000500000000000000000000000" {
		t.Fatal(root, e)
	}
}
func TestIrrationalIntermediateAndExactMidpoints(t *testing.T) {
	root, e := Unary("sqrt", number(t, "2"))
	if e != nil {
		t.Fatal(e)
	}
	squared, e := Binary("multiply", root, root)
	if e != nil {
		t.Fatal(e)
	}
	got, e := squared.Encode(t.Context(), 60)
	if e != nil || got.Value != "2."+strings.Repeat("0", 60) {
		t.Fatal(got, e)
	}
	zero, e := Binary("subtract", root, root)
	if e != nil {
		t.Fatal(e)
	}
	got, e = zero.Encode(t.Context(), 18)
	if e != nil || got.Value != "0.000000000000000000" {
		t.Fatal(got, e)
	}
	for _, tc := range []struct{ square, want string }{{"6.25", "2"}, {"12.25", "4"}, {"0.00000000000000000000000025", "0.0000000000005"}} {
		scale := 0
		if strings.Contains(tc.want, ".") {
			scale = 13
		}
		n, e := Unary("sqrt", number(t, tc.square))
		if e != nil {
			t.Fatal(e)
		}
		got, e := n.Encode(t.Context(), scale)
		if e != nil || got.Value != tc.want {
			t.Fatal(tc, got, e)
		}
	}
	// This denominator is tiny but provably nonzero, not an approximate zero.
	tiny := new(big.Rat).SetFrac(big.NewInt(2), Power10(1000))
	n, e := Rational(tiny)
	if e != nil {
		t.Fatal(e)
	}
	n, e = Unary("sqrt", n)
	if e != nil {
		t.Fatal(e)
	}
	_, e = n.Encode(t.Context(), 60)
	if !errors.Is(e, ErrUnderflow) {
		t.Fatal(e)
	}
}
func TestRulesRejectImportNotationAndBoundWork(t *testing.T) {
	for _, s := range []string{"1e3", "NaN", "Infinity", "1/2", ".1", "1.", "", "1_000"} {
		if _, e := ParseDecimal(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
	_, e := Rational(new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), MaxBits)))
	if !errors.Is(e, ErrBudget) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, e = number(t, "1").Encode(ctx, 18)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
