package ingestion

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func grantTx(ctx context.Context, tx pgx.Tx, source, terminal string) (Grant, error) {
	return read[Grant](tx.QueryRow(ctx, `SELECT `+grantJSON+` FROM ingestion_execution_grants g WHERE source_id=$1 AND terminal_id IS NOT DISTINCT FROM $2::uuid`, source, k.Nullable(terminal)))
}
func (s *Service) Grants(ctx context.Context, a access.Actor, scope Scope, f Filter) ([]Grant, error) {
	out := []Grant{}
	e := s.transact(ctx, a, scope, "execution-grants", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT `+grantJSON+` FROM ingestion_execution_grants g WHERE tenant_id=$1 AND ($2::uuid IS NULL OR source_id=$2) ORDER BY updated_at DESC LIMIT 200`, scope.Tenant, k.Nullable(f.SourceID))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := read[Grant](rows)
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) SaveGrant(ctx context.Context, a access.Actor, scope Scope, in GrantInput) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if in.Budget < 1 || in.Budget > 100000 {
		return out, k.Invalid("/budget", "budget", "队列预算应为 1—100000。")
	}
	if in.TerminalID == "" && in.Measure {
		return out, k.Invalid("/measure", "terminal_required", "测量执行需明确终端范围。")
	}
	e := s.transact(ctx, a, scope, "execution-grants", "save", true, func(tx pgx.Tx, auth k.Authorization) error {
		src, e := sourceTx(ctx, tx, scope, in.SourceID)
		if e != nil {
			return e
		}
		if in.TerminalID != "" {
			t, e := terminalTx(ctx, tx, scope.Tenant, in.TerminalID)
			if e != nil {
				return e
			}
			if t.SourceID != src.ID {
				return k.NotFound("ingestion")
			}
		}
		rev, e := revisionTx(ctx, tx, src.ID, src.Revision)
		if e != nil {
			return e
		}
		if !rev.Confirmed {
			return k.Problem("awaiting_source_confirmation", "机器授权须基于已确认入口。", 409)
		}
		if prev, ok, e := replay(ctx, tx, auth, "grants.save", in.Meta, in); e != nil || ok {
			out = prev
			return e
		}
		current, e := grantTx(ctx, tx, in.SourceID, in.TerminalID)
		if e != nil {
			var p *k.Error
			if !isMissing(e, &p) {
				return e
			}
			current = Grant{ID: uuid.NewString(), Generation: "0"}
		}
		if e = k.Expect(in.Revision, current.Generation); e != nil {
			return e
		}
		out = Result{ID: current.ID, Revision: numstr(num(current.Generation) + 1), State: "enabled"}
		if !in.Enabled {
			out.State = "paused"
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_execution_grants(id,tenant_id,source_id,terminal_id,identify,measure,enabled,generation,budget,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(id) DO UPDATE SET identify=EXCLUDED.identify,measure=EXCLUDED.measure,enabled=EXCLUDED.enabled,generation=EXCLUDED.generation,budget=EXCLUDED.budget,created_by=EXCLUDED.created_by,reason=EXCLUDED.reason,updated_at=clock_timestamp()`, out.ID, scope.Tenant, src.ID, k.Nullable(in.TerminalID), in.Identify, in.Measure, in.Enabled, out.Revision, in.Budget, a.ID, in.Reason); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_execution_grant_history(grant_id,generation,snapshot) SELECT id,generation,`+grantJSON+` FROM ingestion_execution_grants g WHERE id=$1`, out.ID); e != nil {
			return e
		}
		if in.TerminalID == "" {
			if _, e = tx.Exec(ctx, `UPDATE ingestion_sources SET planning_paused=$2 WHERE id=$1`, src.ID, !in.Enabled || !in.Identify); e != nil {
				return e
			}
		}
		return complete(ctx, tx, auth, "grants.save", in.Meta, in, out, map[string]string{"revision": current.Generation}, map[string]string{"revision": out.Revision, "state": out.State})
	})
	return out, e
}
func (s *Service) Operations(ctx context.Context, a access.Actor, scope Scope, f Filter) (map[string]any, error) {
	out := map[string]any{"items": []json.RawMessage{}, "changes": []json.RawMessage{}}
	e := s.transact(ctx, a, scope, "operations", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT jsonb_build_object('source_id',s.id,'source_project_id',s.project_id,'source_state',s.state,'planning_paused',s.planning_paused,'connection_state',CASE WHEN r.observed_at IS NULL OR r.observed_at<clock_timestamp()-interval '60 seconds' THEN 'unknown' ELSE COALESCE(r.status->'sources'->s.id::text->>'connection_state','unknown') END,'observed_at',r.observed_at,'last_received_at',(SELECT max(received_at) FROM ingestion_receipts WHERE source_id=s.id),'last_measurement_at',(SELECT max(u.created_at) FROM ingestion_units u JOIN ingestion_receipts z ON z.id=u.receipt_id WHERE z.source_id=s.id AND u.message_kind='measurement' AND u.tenant_id=s.tenant_id),'last_parsed_at',(SELECT max(p.finished_at) FROM ingestion_parse_plans p JOIN ingestion_units u ON u.id=p.unit_id JOIN ingestion_receipts z ON z.id=u.receipt_id WHERE z.source_id=s.id AND p.state='parsed'),'unmatched',(SELECT count(*)::text FROM ingestion_receipts z WHERE z.source_id=s.id AND z.state IN ('awaiting_identity','awaiting_association','quarantined')),'queued',(SELECT count(*)::text FROM ingestion_parse_plans p WHERE p.snapshot->'source'->>'ingress_id'=s.id::text AND p.state IN ('queued','retry_wait','parsing'))) FROM ingestion_sources s JOIN ingestion_relays r ON r.id=s.relay_id WHERE s.tenant_id=$1 AND ($2::uuid IS NULL OR s.id=$2) ORDER BY s.created_at LIMIT 100`, scope.Tenant, k.Nullable(f.SourceID))
		if e != nil {
			return e
		}
		items := []json.RawMessage{}
		for rows.Next() {
			var v json.RawMessage
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		out["items"] = items
		rows, e = tx.Query(ctx, `SELECT to_jsonb(c)-'input'-'actor_id' FROM ingestion_change_intents c WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 100`, scope.Tenant)
		if e != nil {
			return e
		}
		defer rows.Close()
		changes := []json.RawMessage{}
		for rows.Next() {
			var v json.RawMessage
			if e = rows.Scan(&v); e != nil {
				return e
			}
			changes = append(changes, v)
		}
		out["changes"] = changes
		return rows.Err()
	})
	return out, e
}

// Planning pause is independent of reception and machine authorization. Explicit
// recovery keeps its fixed list and remains subject to both current grants.
func (s *Service) PlanningState(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if in.Action != "pause" && in.Action != "resume" {
		return out, k.Invalid("/action", "enum", "请选择暂停或恢复自动规划。")
	}
	e := s.transact(ctx, a, scope, "sources", "planning", true, func(tx pgx.Tx, auth k.Authorization) error {
		src, e := sourceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "sources.planning", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if e = k.Expect(in.Revision, src.Revision); e != nil {
			return e
		}
		if in.Action == "resume" {
			g, e := grantTx(ctx, tx, id, "")
			if e != nil || !g.Enabled || !g.Identify || src.Blocked {
				return k.Problem("execution_unconfigured", "请先恢复机器授权并解除阻断。", 409)
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_sources SET planning_paused=$2 WHERE id=$1`, id, in.Action == "pause"); e != nil {
			return e
		}
		out = Result{ID: id, Revision: src.Revision, State: in.Action}
		return complete(ctx, tx, auth, "sources.planning", in.Meta, payload, out, nil, map[string]string{"state": in.Action})
	})
	return out, e
}
