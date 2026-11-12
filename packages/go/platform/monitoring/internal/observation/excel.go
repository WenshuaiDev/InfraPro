package observation

import (
	"errors"
	"math/big"
	"strconv"
	"time"
)

// ExcelTime reads the stored numeric token, not a display string or float64.
// Sub-nanosecond serials cannot be represented losslessly and require the
// source to supply an explicit text timestamp. The fictional day 60 is invalid.
func ExcelTime(text string, date1904 bool) (time.Time, error) {
	m := numberPattern.FindStringSubmatch(text)
	if len(text) > 1024 || m == nil {
		return time.Time{}, errors.New("invalid_excel_date")
	}
	if m[4] != "" {
		e, err := strconv.Atoi(m[4])
		if err != nil || e < -1024 || e > 1024 {
			return time.Time{}, errors.New("invalid_excel_date")
		}
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok || r.Sign() < 0 {
		return time.Time{}, errors.New("invalid_excel_date")
	}
	day := new(big.Int).Quo(r.Num(), r.Denom())
	if !day.IsInt64() {
		return time.Time{}, errors.New("time_out_of_range")
	}
	base := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		if day.Int64() == 60 {
			return time.Time{}, errors.New("excel_fictional_1900_02_29")
		}
		if day.Int64() > 60 {
			r.Sub(r, big.NewRat(1, 1))
		}
	}
	r.Mul(r, big.NewRat(86400*1e9, 1))
	if !r.IsInt() {
		return time.Time{}, errors.New("excel_subnanosecond_requires_text_timestamp")
	}
	nanos := new(big.Int).Add(r.Num(), big.NewInt(base.UnixNano()))
	if !nanos.IsInt64() {
		return time.Time{}, errors.New("time_out_of_range")
	}
	tm := time.Unix(0, nanos.Int64()).UTC()
	if err := CheckTime(tm); err != nil {
		return time.Time{}, err
	}
	return tm, nil
}
