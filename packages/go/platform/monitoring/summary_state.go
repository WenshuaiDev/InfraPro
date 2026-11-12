package monitoring

import (
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
)

// Summary states are lossless and mergeable. Every count and coefficient is
// encoded as a decimal string; no database or plotting float enters this path.
type summaryPoint struct {
	Revoked                                           bool
	ID, Adoption, Batch, Kind, Value, Quality, Reason string
	At                                                int64  `json:",string"`
	Start                                             *int64 `json:",string"`
}

func pointOf(r rawObservation) *summaryPoint {
	return &summaryPoint{ID: r.Observation, Adoption: r.Adoption, Batch: r.Batch, Kind: r.Kind, Value: r.Value, Quality: r.Quality, Reason: r.QualityReason, At: r.At, Start: r.Start}
}
func (p *summaryPoint) dto(channel string) *Observation {
	if p == nil {
		return nil
	}
	v := observationDTO(rawObservation{Observation: p.ID, Adoption: p.Adoption, Batch: p.Batch, Channel: channel, At: p.At, Start: p.Start, Kind: p.Kind, Value: p.Value, Quality: p.Quality, QualityReason: p.Reason}, p.Revoked)
	return &v
}
func pointLess(a, b *summaryPoint) bool {
	if a.At != b.At {
		return a.At < b.At
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	return a.Adoption < b.Adoption
}

type enumState struct {
	N           uint64 `json:",string"`
	First, Last *summaryPoint
}
type exactState struct {
	N                     uint64 `json:",string"`
	Sum, Squares          string
	First, Last, Min, Max *summaryPoint
	MinCount, MaxCount    uint64               `json:",string"`
	States                map[string]enumState `json:",omitempty"`
}

func integerState(s string) *big.Int {
	if s == "" {
		return new(big.Int)
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid internally generated exact state")
	}
	return n
}
func compareValue(a, b *summaryPoint) int {
	x, _ := observation.Coefficient(a.Value, 12)
	y, _ := observation.Coefficient(b.Value, 12)
	return x.Cmp(y)
}
func (s *exactState) add(p *summaryPoint) {
	s.N++
	if s.First == nil || pointLess(p, s.First) {
		s.First = p
	}
	if s.Last == nil || pointLess(s.Last, p) {
		s.Last = p
	}
	if p.Kind == "enum" {
		s.Sum = "0"
		s.Squares = "0"
		if s.States == nil {
			s.States = map[string]enumState{}
		}
		v := s.States[p.Value]
		v.N++
		if v.First == nil || pointLess(p, v.First) {
			v.First = p
		}
		if v.Last == nil || pointLess(v.Last, p) {
			v.Last = p
		}
		s.States[p.Value] = v
		return
	}
	c, _ := observation.Coefficient(p.Value, 12)
	s.Sum = new(big.Int).Add(integerState(s.Sum), c).String()
	s.Squares = new(big.Int).Add(integerState(s.Squares), new(big.Int).Mul(c, c)).String()
	if s.Min == nil || compareValue(p, s.Min) < 0 {
		s.Min = p
		s.MinCount = 1
	} else if compareValue(p, s.Min) == 0 {
		s.MinCount++
		if pointLess(p, s.Min) {
			s.Min = p
		}
	}
	if s.Max == nil || compareValue(p, s.Max) > 0 {
		s.Max = p
		s.MaxCount = 1
	} else if compareValue(p, s.Max) == 0 {
		s.MaxCount++
		if pointLess(p, s.Max) {
			s.Max = p
		}
	}
}
func (s *exactState) merge(v exactState) {
	if v.N == 0 {
		return
	}
	s.N += v.N
	if s.First == nil || pointLess(v.First, s.First) {
		s.First = v.First
	}
	if s.Last == nil || pointLess(s.Last, v.Last) {
		s.Last = v.Last
	}
	s.Sum = new(big.Int).Add(integerState(s.Sum), integerState(v.Sum)).String()
	s.Squares = new(big.Int).Add(integerState(s.Squares), integerState(v.Squares)).String()
	if v.Min != nil {
		if s.Min == nil || compareValue(v.Min, s.Min) < 0 {
			s.Min = v.Min
			s.MinCount = v.MinCount
		} else if compareValue(v.Min, s.Min) == 0 {
			s.MinCount += v.MinCount
			if pointLess(v.Min, s.Min) {
				s.Min = v.Min
			}
		}
	}
	if v.Max != nil {
		if s.Max == nil || compareValue(v.Max, s.Max) > 0 {
			s.Max = v.Max
			s.MaxCount = v.MaxCount
		} else if compareValue(v.Max, s.Max) == 0 {
			s.MaxCount += v.MaxCount
			if pointLess(v.Max, s.Max) {
				s.Max = v.Max
			}
		}
	}
	if len(v.States) > 0 && s.States == nil {
		s.States = map[string]enumState{}
	}
	for k, b := range v.States {
		a := s.States[k]
		a.N += b.N
		if a.First == nil || pointLess(b.First, a.First) {
			a.First = b.First
		}
		if a.Last == nil || pointLess(a.Last, b.Last) {
			a.Last = b.Last
		}
		s.States[k] = a
	}
}

type summaryTotals struct {
	Active, History            map[string]exactState
	Ever                       uint64 `json:",string"`
	RequiredStart, First, Last *int64 `json:",string"`
}

func minNS(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b < *a {
		return b
	}
	return a
}
func maxNS(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b > *a {
		return b
	}
	return a
}
func (s *summaryTotals) add(r rawObservation, active bool) {
	if s.History == nil {
		s.History = map[string]exactState{}
	}
	p := pointOf(r)
	p.Revoked = !active
	v := s.History[r.Quality]
	v.add(p)
	s.History[r.Quality] = v
	if active {
		if s.Active == nil {
			s.Active = map[string]exactState{}
		}
		v = s.Active[r.Quality]
		v.add(p)
		s.Active[r.Quality] = v
	}
	if r.First {
		s.Ever++
	}
	lo := r.At
	if r.Start != nil {
		lo = *r.Start
	}
	s.RequiredStart = minNS(s.RequiredStart, &lo)
	s.First = minNS(s.First, &r.At)
	s.Last = maxNS(s.Last, &r.At)
}
func (s *summaryTotals) merge(v summaryTotals) {
	if s.Active == nil {
		s.Active = map[string]exactState{}
	}
	if s.History == nil {
		s.History = map[string]exactState{}
	}
	for k, b := range v.Active {
		a := s.Active[k]
		a.merge(b)
		s.Active[k] = a
	}
	for k, b := range v.History {
		a := s.History[k]
		a.merge(b)
		s.History[k] = a
	}
	s.Ever += v.Ever
	s.RequiredStart = minNS(s.RequiredStart, v.RequiredStart)
	s.First = minNS(s.First, v.First)
	s.Last = maxNS(s.Last, v.Last)
}
func (s summaryTotals) samples(qualities []string, history bool) exactState {
	out := exactState{}
	states := s.Active
	if history {
		states = s.History
	}
	for _, q := range qualities {
		out.merge(states[q])
	}
	return out
}

var allQualities = []string{"good", "unmarked", "suspect", "bad"}

func (s summaryTotals) covered(windows []Window, query *Window) bool {
	if s.First == nil {
		return true
	}
	if query != nil && (!query.Contains(*s.First, nil) || !query.Contains(*s.Last, nil)) {
		return false
	}
	for _, w := range windows {
		if (w.From == nil || *s.RequiredStart >= *w.From) && (w.To == nil || *s.Last < *w.To) {
			return true
		}
	}
	return false
}
func (s summaryTotals) overlaps(windows []Window, query *Window) bool {
	if s.First == nil {
		return false
	}
	if query != nil && (query.From != nil && *query.From > *s.Last || query.To != nil && *query.To <= *s.First) {
		return false
	}
	for _, w := range windows {
		if (w.From == nil || *w.From <= *s.Last) && (w.To == nil || *w.To > *s.First) {
			return true
		}
	}
	return false
}
func (s exactState) representatives() []*summaryPoint {
	out := []*summaryPoint{}
	seen := map[string]bool{}
	for _, p := range []*summaryPoint{s.First, s.Min, s.Max, s.Last} {
		if p != nil && !seen[p.ID] {
			out = append(out, p)
			seen[p.ID] = true
		}
	}
	slices.SortFunc(out, func(a, b *summaryPoint) int {
		if pointLess(a, b) {
			return -1
		}
		if pointLess(b, a) {
			return 1
		}
		return 0
	})
	return out
}
func (s exactState) moments(digits int) (*string, *string, error) {
	m := observation.Moments{}
	m.N.SetUint64(s.N)
	m.S1.Set(integerState(s.Sum))
	m.S2.Set(integerState(s.Squares))
	return m.Results(digits)
}
func nsText(n *int64) *string {
	if n == nil {
		return nil
	}
	v := time.Unix(0, *n).UTC().Format(time.RFC3339Nano)
	return &v
}
func decimalCount(n uint64) string { return fmt.Sprint(n) }

func inactiveContribution(v summaryTotals) summaryTotals {
	v.Active = nil
	history := map[string]exactState{}
	mark := func(p *summaryPoint) *summaryPoint {
		if p == nil {
			return nil
		}
		out := *p
		out.Revoked = true
		return &out
	}
	for q, st := range v.History {
		st.First = mark(st.First)
		st.Last = mark(st.Last)
		st.Min = mark(st.Min)
		st.Max = mark(st.Max)
		if st.States != nil {
			states := map[string]enumState{}
			for k, e := range st.States {
				e.First = mark(e.First)
				e.Last = mark(e.Last)
				states[k] = e
			}
			st.States = states
		}
		history[q] = st
	}
	v.History = history
	return v
}
