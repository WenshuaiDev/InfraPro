package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

// The creator's membership and project are audit evidence, never credentials
// borrowed by the automatic worker. Only the current tenant mandate can run it.
func (s *Service) AuthorizeRawSystemTx(ctx context.Context, tx pgx.Tx, permit monitoring.RawSystemPermit, needs []monitoring.RawNeed, history bool) error {
	if history || !access.ValidID(permit.Tenant) || !access.ValidID(permit.RunID) {
		return access.ErrDenied
	}
	if _, e := tx.Exec(ctx, `SELECT revision FROM derived_graph_states WHERE tenant_id=$1 FOR SHARE`, permit.Tenant); e != nil {
		return e
	}
	var allowed []monitoring.RawNeed
	e := tx.QueryRow(ctx, `SELECT m.input_requirements FROM derived_runs r
 JOIN derived_channels c ON c.tenant_id=r.tenant_id AND c.channel_id=r.channel_id
 JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id
 JOIN tenants t ON t.id=r.tenant_id
 WHERE r.tenant_id=$1 AND r.id=$2 AND r.actor_id IS NULL AND r.project_id IS NULL AND r.mode='automatic'
 AND t.status='active' AND c.control_state='enabled' AND c.control_revision=r.control_revision
 AND c.mandate_revision=r.mandate_revision AND m.revision=r.mandate_revision AND m.revision=$3
 AND c.config_revision=r.config_revision AND m.output_ranges @> r.output_range
 AND EXISTS(SELECT 1 FROM derived_config_assignments a WHERE a.tenant_id=r.tenant_id AND a.channel_id=r.channel_id AND a.config_id=r.config_id AND a.superseded_revision IS NULL AND a.effective_range @> r.output_range)
 AND NOT EXISTS(WITH RECURSIVE ancestors AS(SELECT kind,id,parent_kind,parent_id,status,deleted_at FROM engineering_resource_nodes WHERE tenant_id=r.tenant_id AND kind='channel' AND id=r.channel_id UNION ALL SELECT n.kind,n.id,n.parent_kind,n.parent_id,n.status,n.deleted_at FROM engineering_resource_nodes n JOIN ancestors a ON n.kind=a.parent_kind AND n.id=a.parent_id WHERE n.tenant_id=r.tenant_id) SELECT 1 FROM ancestors WHERE status<>'normal' OR deleted_at IS NOT NULL)`, permit.Tenant, permit.RunID, permit.MandateRevision).Scan(&allowed)
	if e == pgx.ErrNoRows {
		return &monitoring.TaskError{Code: "mandate_changed", Message: "通道控制或执行委托已经变化。", Disposition: "cancelled"}
	}
	if e != nil {
		return e
	}
	grouped := map[string][]planning.Range{}
	for _, n := range allowed {
		from := monitoringvalue.MinTime.UnixNano()
		if n.From != nil {
			from = *n.From
		}
		grouped[n.Channel] = append(grouped[n.Channel], planning.Range{From: from, To: n.To})
	}
	ids := []string{}
	for _, n := range needs {
		from := monitoringvalue.MinTime.UnixNano()
		if n.From != nil {
			from = *n.From
		}
		covered, e := planning.Covered([]planning.Range{{From: from, To: n.To}}, grouped[n.Channel])
		if e != nil || !covered {
			return access.ErrDenied
		}
		ids = append(ids, n.Channel)
	}
	// Retired inputs keep historical meaning; deleted input identities cannot
	// support a new run. Future extension across retirement is limited by planning.
	var unavailable bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM unnest($2::uuid[]) x(id) LEFT JOIN engineering_channels c ON c.tenant_id=$1 AND c.id=x.id WHERE c.id IS NULL OR c.deleted_at IS NOT NULL OR c.source_kind NOT IN ('direct','derived'))`, permit.Tenant, ids).Scan(&unavailable); e != nil {
		return e
	}
	if unavailable {
		return access.ErrDenied
	}
	return nil
}
