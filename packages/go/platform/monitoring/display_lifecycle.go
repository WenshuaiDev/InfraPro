package monitoring

import (
	"context"
	"encoding/json"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayBindWorkTx(ctx context.Context, tx pgx.Tx, id string, raw json.RawMessage) error {
	var w displayWork
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	if w.Empty || w.Input.Snapshot == "" {
		return nil
	}
	if _, e := tx.Exec(ctx, `UPDATE monitoring_snapshots SET display_managed=true WHERE id=$1`, w.Input.Snapshot); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO display_read_monitoring_refs(query_id,snapshot_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, w.Input.Snapshot)
	return e
}
func (s *Service) DisplayReleaseWorkTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage) error {
	var w displayWork
	if len(raw) == 0 {
		return nil
	}
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	if w.Input.Snapshot == "" {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE monitoring_jobs SET cancel_requested=true,state=CASE WHEN state='running' THEN state ELSE 'cancelled' END,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND display_subject_id=$2 AND kind='summary' AND target_id=$3 AND state IN ('queued','retry_wait','running') AND NOT EXISTS(SELECT 1 FROM display_read_monitoring_refs r JOIN display_read_queries q ON q.id=r.query_id WHERE r.snapshot_id=$3 AND q.state NOT IN ('failed','canceled','expired') AND q.expires_at>clock_timestamp())`, p.Authority.Owner.Tenant, p.Authority.Owner.Subject, w.Input.Snapshot)
	return e
}
