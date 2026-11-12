package observation

import (
	"math/big"
	"testing"
	"time"
)

func TestExactValueBoundaries(t *testing.T) {
	for _, input := range []string{"0", "-0", "1e-12", "99999999999999999999999999.999999999999", "-99999999999999999999999999.999999999999", "10000000000000000000000000.000000000003"} {
		c, err := Decimal(input)
		if err != nil {
			t.Fatal(input, err)
		}
		r, err := Decimal(Format(c, DecimalScale))
		if err != nil || c.Cmp(r) != 0 {
			t.Fatal("round trip", input, err)
		}
	}
	for _, input := range []string{"", "NaN", "Infinity", "1,000", "1e-13", "1.0000000000001", "100000000000000000000000000", "1e99999999", " 1"} {
		if _, err := Decimal(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	for _, input := range []string{"-9223372036854775808", "9223372036854775807", "0", "1.00e2"} {
		if _, err := Integer(input); err != nil {
			t.Fatal(input, err)
		}
	}
	for _, input := range []string{"9223372036854775808", "-9223372036854775809", "1.1"} {
		if _, err := Integer(input); err == nil {
			t.Fatal("integer accepted", input)
		}
	}
}

func TestTimeBoundariesAndDST(t *testing.T) {
	for _, input := range []string{"1900-01-01T00:00:00Z", "1969-12-31T23:59:59.999999999Z", "2262-04-11T23:47:16.854775807Z", "2024-02-29T12:30:00.123456789+08:00"} {
		tm, err := ParseTime(input)
		if err != nil {
			t.Fatal(input, err)
		}
		if !time.Unix(0, tm.UnixNano()).Equal(tm) {
			t.Fatal("nanoseconds lost", input)
		}
	}
	for _, input := range []string{"1899-12-31T23:59:59.999999999Z", "2262-04-11T23:47:16.854775808Z", "2024-01-01T00:00:00.1234567891Z", "2024-01-01T00:00:00", "2024-01-01T00:00:00+24:00", "2024-01-01T00:00:00,1Z"} {
		if _, err := ParseTime(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, input := range []string{"2024-03-10 02:30:00", "2024-11-03 01:30:00"} {
		if _, err := ParseLocal(input, "2006-01-02 15:04:05", "America/New_York"); err == nil {
			t.Fatal("DST accepted", input)
		}
	}
	tm, err := ParseLocal("2024-11-03 03:30:00", "2006-01-02 15:04:05", "America/New_York")
	if err != nil || tm.Format(time.RFC3339) != "2024-11-03T08:30:00Z" {
		t.Fatal(tm, err)
	}
}

func TestQualityKeepsOriginalClassification(t *testing.T) {
	lo, hi := big.NewInt(-2), big.NewInt(2)
	for _, source := range []string{"", "good", "unmarked", "suspect", "bad"} {
		q, outside, err := Quality(source, big.NewInt(3), lo, hi)
		want := "suspect"
		if source == "bad" {
			want = "bad"
		}
		if err != nil || !outside || q != want {
			t.Fatal(source, q, outside, err)
		}
	}
	q, outside, err := Quality("good", hi, lo, hi)
	if err != nil || outside || q != "good" {
		t.Fatal("inclusive bound", q, err)
	}
	if _, _, err := Quality("unknown", hi, lo, hi); err == nil {
		t.Fatal("unmapped quality accepted")
	}
}

func TestExactMomentsAndHalfEven(t *testing.T) {
	for _, tc := range []struct {
		values    []string
		mean, std string
		digits    int
	}{
		{[]string{"-2", "-1", "-3"}, "-2.000000000000", "0.816496580928", 12},
		{[]string{"-2", "-1"}, "-1.500000000000", "0.500000000000", 12},
		{[]string{"10000000000000000000000000.000000000001", "10000000000000000000000000.000000000003"}, "10000000000000000000000000.000000000002", "0.000000000001", 12},
		{[]string{"0", "1"}, "0", "0", 0},
		{[]string{"0", "3"}, "2", "2", 0},
		{[]string{"-3"}, "-3.000000000000", "0.000000000000", 12},
	} {
		var all, left, right Moments
		for i, s := range tc.values {
			c, e := Decimal(s)
			if e != nil {
				t.Fatal(e)
			}
			all.Add(c)
			if i%2 == 0 {
				left.Add(c)
			} else {
				right.Add(c)
			}
		}
		left.Merge(&right)
		if left.N.Cmp(&all.N) != 0 || left.S1.Cmp(&all.S1) != 0 || left.S2.Cmp(&all.S2) != 0 {
			t.Fatal("merge differs")
		}
		mean, std, e := all.Results(tc.digits)
		if e != nil || *mean != tc.mean || *std != tc.std {
			t.Fatal(tc, mean, std, e)
		}
	}
	var empty Moments
	m, s, e := empty.Results(12)
	if e != nil || m != nil || s != nil {
		t.Fatal("empty statistics")
	}
}

func TestExcelDates(t *testing.T) {
	for _, tc := range []struct {
		serial   string
		date1904 bool
		want     string
	}{{"1", false, "1900-01-01T00:00:00Z"}, {"59", false, "1900-02-28T00:00:00Z"}, {"61.5", false, "1900-03-01T12:00:00Z"}, {"0", true, "1904-01-01T00:00:00Z"}, {"1.00000000001", true, "1904-01-02T00:00:00.000000864Z"}} {
		tm, e := ExcelTime(tc.serial, tc.date1904)
		if e != nil || tm.Format(time.RFC3339Nano) != tc.want {
			t.Fatal(tc, tm, e)
		}
	}
	for _, s := range []string{"60", "60.5", "-1", "0", "1.00000000000001", "1e99999999"} {
		if _, e := ExcelTime(s, false); e == nil {
			t.Fatal("invalid Excel serial", s)
		}
	}
}
