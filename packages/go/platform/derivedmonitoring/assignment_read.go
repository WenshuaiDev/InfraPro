package derivedmonitoring

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/jackc/pgx/v5"
)

// Split frozen result-root references at current configuration boundaries. A
// correction to a small suffix must not hide the untouched prefix of a block.
func splitAssignmentEntries(entries []resultstore.RangeEntry, assignments []ConfigAssignment) []resultstore.RangeEntry {
	out := []resultstore.RangeEntry{}
	for _, entry := range entries {
		cuts := []int64{entry.From}
		for _, assignment := range assignments {
			r, _ := assignment.Range.rangeValue()
			if r.From > entry.From && (entry.To == nil || r.From < *entry.To) {
				cuts = append(cuts, r.From)
			}
			if r.To != nil && *r.To > entry.From && (entry.To == nil || *r.To < *entry.To) {
				cuts = append(cuts, *r.To)
			}
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		for i, start := range cuts {
			v := entry
			v.From = start
			if i+1 < len(cuts) {
				v.To = planning.End(cuts[i+1])
			}
			out = append(out, v)
		}
	}
	return out
}
func (s *Service) currentAssignments(ctx context.Context, tenant, channel string) ([]ConfigAssignment, error) {
	var out []ConfigAssignment
	e := s.access.Transact(ctx, func(tx pgx.Tx) error { var e error; out, e = activeAssignmentsTx(ctx, tx, tenant, channel); return e })
	return out, e
}
