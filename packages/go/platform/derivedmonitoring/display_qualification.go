package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

// Metadata/count eligibility includes the current recursive input mandate, even
// when no output has been computed yet. This never starts a calculation.
func (s *Service) QualifyDisplayInputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, windows map[string][]monitoring.Window) error {
	ids := []string{}
	for id := range windows {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND source_kind='derived' AND deleted_at IS NULL`, scope.Tenant, ids)
	if e != nil {
		return e
	}
	derived := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		derived = append(derived, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	needs := []monitoring.RawNeed{}
	for _, id := range derived {
		for _, w := range windows[id] {
			needs = append(needs, monitoring.RawNeed{Channel: id, From: w.From, To: w.To})
		}
	}
	if len(needs) == 0 {
		return nil
	}
	return s.authorizeInputsTx(ctx, tx, a, scope, needs, false)
}
