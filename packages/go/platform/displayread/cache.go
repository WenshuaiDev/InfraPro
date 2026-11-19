package displayread

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) WithWorkLifecycle(bind func(context.Context, pgx.Tx, string, json.RawMessage) error, release func(context.Context, pgx.Tx, Plan, json.RawMessage) error) {
	s.bindWork = bind
	s.releaseWork = release
}

// Completed computations can be shared, but each caller gets its own query and
// cancellation lease. Releasing one copy cannot cancel another consumer.
func (s *Service) copyCacheTx(ctx context.Context, tx pgx.Tx, p Plan) (string, error) {
	if strings.HasPrefix(p.Method, "views.") || p.Method == "spatial.readFile" || p.Method == "map.readTile" {
		return "", nil
	}
	seconds := 5
	if p.Method == "statistics.monitoring" {
		seconds = 10
		if q, ok := p.Request.(*MonitoringStatisticsRequest); ok && q.FollowSeconds == 0 {
			// Fixed ranges/all-history have no clock dependency. Their cache
			// remains valid until query expiry while the full source vector,
			// scope/field evidence and live owning proof are unchanged. A short
			// wall-clock TTL needlessly reruns identical formal statistics.
			seconds = int(QueryLifetime.Seconds())
		}
	}
	if p.Method == "statistics.resources" || p.Method == "statistics.devices" {
		seconds = 30
	}
	var source string
	var proof json.RawMessage
	e := tx.QueryRow(ctx, `SELECT id::text,proof FROM display_read_queries WHERE session_id=$1 AND config_id=$2 AND environment=$3 AND method=$4 AND scope_hash=$5 AND filter_hash=$6 AND authority_hash=$7 AND source_vector->'source_vector'=$8::jsonb
 AND NOT EXISTS (SELECT 1 FROM monitoring_cache_floors f WHERE f.consumer='display-cache' AND f.tenant_id=display_read_queries.tenant_id AND f.domain_key=f.tenant_id AND f.revision>(source_vector->'source_vector'->>'raw')::bigint) AND state IN ('ready','partial','empty') AND parent_query_id IS NULL AND expires_at>clock_timestamp() AND completed_at>clock_timestamp()-$9*interval '1 second' ORDER BY completed_at DESC LIMIT 1`, p.Authority.Owner.Session, p.Authority.Owner.Config, p.Authority.Owner.Environment, p.Method, Hash(p.Scopes), requestHash(p.Request), Hash([]string{p.Evidence.Authorization, p.Evidence.Safety}), Bytes(p.Evidence.Vector), seconds).Scan(&source, &proof)
	if e == pgx.ErrNoRows {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	if validate := s.validators[p.Method]; validate != nil {
		if e = validate(ctx, tx, p, proof); e != nil {
			return "", e
		}
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO display_read_queries(tenant_id,project_id,instance_id,subject_id,session_id,config_id,environment,method,request,scope_hash,filter_hash,authority_hash,source_vector,scopes,included_field_groups,state,read_snapshot_id,metadata_as_of,completed_at,metadata,completeness,row_count,result_bytes,proof,expires_at) SELECT tenant_id,project_id,instance_id,subject_id,session_id,config_id,environment,method,$2,scope_hash,filter_hash,authority_hash,source_vector,scopes,included_field_groups,state,read_snapshot_id,metadata_as_of,completed_at,metadata,completeness,row_count,result_bytes,proof,expires_at FROM display_read_queries WHERE id=$1 RETURNING id::text`, source, Bytes(p.Request)).Scan(&id)
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO display_read_query_rows SELECT $1,ordinal,sort_key,data FROM display_read_query_rows WHERE query_id=$2`, id, source); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO display_read_query_sections SELECT $1,section,ordinal,data FROM display_read_query_sections WHERE query_id=$2`, id, source); e != nil {
		return "", e
	}
	if s.bindWork != nil && s.executors[p.Method] != nil {
		if e = s.bindWork(ctx, tx, id, proof); e != nil {
			return "", e
		}
	}
	slog.Info("display read cache", "query_id", id, "method", p.Method, "cache_hit", true)
	return id, nil
}
