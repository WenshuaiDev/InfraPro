package planning

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"math"
	"slices"
)

// Range is a half-open nanosecond range. A nil upper endpoint represents the
// clipped domain upper edge, including MaxInt64 without computing max+1.
type Range struct {
	From int64
	To   *int64
}

func End(n int64) *int64 { return &n }
func (r Range) Valid() bool {
	return r.From >= monitoringvalue.MinTime.UnixNano() && (r.To == nil || r.From < *r.To)
}
func (r Range) Contains(t int64) bool    { return t >= r.From && (r.To == nil || t < *r.To) }
func beforeEnd(t int64, end *int64) bool { return end == nil || t < *end }
func Overlap(a, b Range) bool            { return beforeEnd(a.From, b.To) && beforeEnd(b.From, a.To) }
func minEnd(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a < *b {
		return a
	}
	return b
}
func maxEnd(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	if *a > *b {
		return a
	}
	return b
}
func Intersection(a, b Range) (Range, bool) {
	from := a.From
	if b.From > from {
		from = b.From
	}
	r := Range{from, minEnd(a.To, b.To)}
	return r, r.Valid()
}
func Union(ranges []Range) ([]Range, error) {
	ordered := append([]Range(nil), ranges...)
	for _, r := range ordered {
		if !r.Valid() {
			return nil, fmt.Errorf("invalid_range")
		}
	}
	slices.SortFunc(ordered, func(a, b Range) int {
		if a.From < b.From {
			return -1
		}
		if a.From > b.From {
			return 1
		}
		return 0
	})
	out := []Range{}
	for _, r := range ordered {
		if len(out) == 0 {
			out = append(out, r)
			continue
		}
		last := &out[len(out)-1]
		if last.To == nil || r.From <= *last.To {
			last.To = maxEnd(last.To, r.To)
		} else {
			out = append(out, r)
		}
	}
	return out, nil
}
func Covered(required, granted []Range) (bool, error) {
	needs, err := Union(required)
	if err != nil {
		return false, err
	}
	available, err := Union(granted)
	if err != nil {
		return false, err
	}
	j := 0
	for _, r := range needs {
		for j < len(available) && available[j].To != nil && *available[j].To <= r.From {
			j++
		}
		if j == len(available) || available[j].From > r.From {
			return false, nil
		}
		g := available[j]
		if g.To != nil && (r.To == nil || *g.To < *r.To) {
			return false, nil
		}
	}
	return true, nil
}
func shiftedEnd(end *int64, delta int64) *int64 {
	if end == nil {
		return nil
	}
	if delta > 0 && *end > math.MaxInt64-delta {
		return nil
	}
	v := *end + delta
	return &v
}
func leftShift(start, delta int64) int64 {
	minimum := monitoringvalue.MinTime.UnixNano()
	if delta > 0 && start < minimum+delta {
		return minimum
	}
	return start - delta
}

// DurationHalo is exact for the discrete (t-W,t] window. Authorization
// includes candidate/non-hit instants rather than only successful matches.
func DurationHalo(output Range, width int64) (Range, error) {
	if !output.Valid() || width <= 0 {
		return Range{}, fmt.Errorf("invalid_window")
	}
	return Range{leftShift(output.From, width-1), output.To}, nil
}
func MatchHalo(output Range, tolerance int64, nearest bool) (Range, error) {
	if !output.Valid() || tolerance < 0 {
		return Range{}, fmt.Errorf("invalid_tolerance")
	}
	r := Range{leftShift(output.From, tolerance), output.To}
	if nearest {
		r.To = shiftedEnd(r.To, tolerance)
	}
	return r, nil
}

// DurationImpact maps changed source [a,b) to all outputs whose (t-W,t]
// contains a changed ns. Discrete endpoints make the upper bound b+W-1.
func DurationImpact(changed Range, width int64) (Range, error) {
	if !changed.Valid() || width <= 0 {
		return Range{}, fmt.Errorf("invalid_window")
	}
	return Range{changed.From, shiftedEnd(changed.To, width-1)}, nil
}

// Subtract preserves empty replacement coverage without resurrecting rows from
// a previous generation. It is also used when a newer dirty position overlaps
// only part of the range completed by an older worker.
func Subtract(base Range, cut Range) []Range {
	overlap, ok := Intersection(base, cut)
	if !ok {
		return []Range{base}
	}
	out := []Range{}
	if base.From < overlap.From {
		out = append(out, Range{base.From, End(overlap.From)})
	}
	if overlap.To != nil && (base.To == nil || *overlap.To < *base.To) {
		out = append(out, Range{*overlap.To, base.To})
	}
	return out
}
