package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaintenanceStatus contains bounded aggregate labels. It deliberately omits
// observation values, connection details, object keys and download URLs.
func MaintenanceStatus(ctx context.Context, pool *pgxpool.Pool, tenant string) (json.RawMessage, error) {
	if tenant != "" && !access.ValidID(tenant) {
		return nil, fmt.Errorf("invalid tenant")
	}
	var value json.RawMessage
	e := pool.QueryRow(ctx, `SELECT jsonb_build_object(
 'environment',(SELECT jsonb_build_object('mode',mode,'backup_id',backup_id,'verified_at',verified_at) FROM monitoring_environment_state WHERE id=1),
 'integrity',(SELECT COALESCE(jsonb_agg(to_jsonb(q)),'[]') FROM (SELECT result_kind,state,count(*)::text AS results,max(checked_at) AS last_checked_at FROM derived_result_integrity WHERE $1::uuid IS NULL OR tenant_id=$1 GROUP BY result_kind,state ORDER BY result_kind,state)q),
 'retention',(SELECT COALESCE(jsonb_agg(to_jsonb(q)),'[]') FROM (SELECT kind,state,count(*)::text AS targets,max(completed_at) AS last_completed_at FROM monitoring_retention_receipts WHERE $1::uuid IS NULL OR tenant_id=$1 GROUP BY kind,state ORDER BY kind,state)q),
 'queues',(SELECT COALESCE(jsonb_agg(to_jsonb(q)),'[]') FROM (SELECT kind,state,count(*)::text AS tasks,COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0)::text AS oldest_age_seconds FROM monitoring_jobs WHERE ($1::uuid IS NULL OR tenant_id=$1) AND state IN ('queued','running','retry_wait','needs_action') GROUP BY kind,state ORDER BY kind,state)q),
 'dirty',(SELECT jsonb_build_object('ranges',count(*)::text,'channels',count(DISTINCT channel_id)::text,'oldest_age_seconds',COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0)::text,'earliest_from_ns',min(lower(affected))::text) FROM derived_dirty_ranges WHERE $1::uuid IS NULL OR tenant_id=$1),
 'consumers',(SELECT jsonb_build_object('tenant_count',count(*)::text,'max_position_lag',COALESCE(max(t.commit_sequence-COALESCE(c.durable_cursor,0)),0)::text,'total_position_lag',COALESCE(sum(t.commit_sequence-COALESCE(c.durable_cursor,0)),0)::text) FROM monitoring_tenant_states t LEFT JOIN monitoring_change_consumers c ON c.tenant_id=t.tenant_id AND c.consumer='derived-invalidation/1' WHERE $1::uuid IS NULL OR t.tenant_id=$1),
 'control',(SELECT COALESCE(jsonb_agg(to_jsonb(c)),'[]') FROM (SELECT control_state,health,blocked_reason,count(*)::text AS channels FROM derived_channels WHERE $1::uuid IS NULL OR tenant_id=$1 GROUP BY control_state,health,blocked_reason ORDER BY control_state,health,blocked_reason)c),
 'leases',(SELECT jsonb_build_object('expired',count(*)::text) FROM monitoring_jobs WHERE ($1::uuid IS NULL OR tenant_id=$1) AND state='running' AND lease_until<clock_timestamp()),
 'attempts',(SELECT COALESCE(jsonb_agg(to_jsonb(a)),'[]') FROM (SELECT state,count(*)::text AS attempts,COALESCE(sum((manifest->>'payload_bytes')::numeric),0)::text AS encoded_bytes FROM derived_write_attempts WHERE $1::uuid IS NULL OR tenant_id=$1 GROUP BY state ORDER BY state)a),
 'automatic_latency',(SELECT jsonb_build_object('published_generations',count(*)::text,'p95_seconds',COALESCE(percentile_cont(0.95) WITHIN GROUP(ORDER BY EXTRACT(EPOCH FROM g.created_at-c.created_at)),0)::text,'max_seconds',COALESCE(max(EXTRACT(EPOCH FROM g.created_at-c.created_at)),0)::text) FROM derived_generations g JOIN derived_runs r ON r.tenant_id=g.tenant_id AND r.id=g.run_id JOIN monitoring_changes c ON c.tenant_id=g.tenant_id AND c.source_position=g.source_position WHERE ($1::uuid IS NULL OR g.tenant_id=$1) AND r.mode='automatic'),
 'clock',(SELECT jsonb_build_object('pending',count(*)::text,'oldest_due_ns',min(s.next_due_ns)::text) FROM derived_schedules s JOIN derived_channels c ON c.tenant_id=s.tenant_id AND c.channel_id=s.channel_id WHERE ($1::uuid IS NULL OR s.tenant_id=$1) AND c.control_state='enabled' AND s.next_due_ns < (EXTRACT(EPOCH FROM clock_timestamp())*1000000000)::bigint),
 'generated_at',clock_timestamp())`, nullable(tenant)).Scan(&value)
	return value, e
}
