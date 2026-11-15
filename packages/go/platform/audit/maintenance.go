package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"time"

	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Status struct {
	Version         int        `json:"version"`
	Mode            string     `json:"mode"`
	Epoch           string     `json:"epoch"`
	Projection      string     `json:"projection_version"`
	Unclassified    string     `json:"unclassified"`
	ActiveExports   string     `json:"active_exports"`
	ObjectIntents   string     `json:"object_intents"`
	OpenTransfers   string     `json:"open_transfers"`
	PendingHandoffs string     `json:"pending_handoffs"`
	WorkerSeen      *time.Time `json:"worker_seen"`
	Storage         string     `json:"storage"`
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	out := Status{Storage: "not_configured"}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT version,mode,restore_epoch::text,projection_version::text,unclassified::text,active_exports::text,object_intents::text,open_transfers::text,pending_handoffs::text,worker_seen FROM audit_status_metadata`).Scan(&out.Version, &out.Mode, &out.Epoch, &out.Projection, &out.Unclassified, &out.ActiveExports, &out.ObjectIntents, &out.OpenTransfers, &out.PendingHandoffs, &out.WorkerSeen)
	})
	if s.objects != nil {
		out.Storage = "configured"
	}
	return out, e
}

type Plan struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`
	Revision string         `json:"revision"`
	Epoch    string         `json:"epoch"`
	Cutoff   time.Time      `json:"cutoff"`
	State    string         `json:"state"`
	Summary  map[string]any `json:"summary"`
}

func loadPlanTx(ctx context.Context, tx pgx.Tx, id string) (Plan, error) {
	v := Plan{}
	e := tx.QueryRow(ctx, `SELECT id::text,kind,revision::text,epoch::text,cutoff,state,summary FROM audit_cleanup_plans WHERE id=$1 FOR UPDATE`, id).Scan(&v.ID, &v.Kind, &v.Revision, &v.Epoch, &v.Cutoff, &v.State, &v.Summary)
	return v, e
}
func maintenanceInput(request, reason string) error {
	if !ar.ID(request) || len(reason) == 0 || len([]rune(reason)) > 500 {
		return Problem("audit_maintenance_input_invalid", 400)
	}
	return nil
}
func (s *Service) Plan(ctx context.Context, kind, reason string, retention *time.Duration) (Plan, error) {
	out := Plan{}
	if kind != "facts" && kind != "objects" && kind != "policy" && kind != "backfill" {
		return out, Problem("audit_filter_invalid", 400)
	}
	if reason == "" || len([]rune(reason)) > 500 {
		return out, Problem("audit_filter_invalid", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var epoch, mode string
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM audit_protocol WHERE id=1`).Scan(&epoch, &mode); e != nil {
			return e
		}
		if mode == "reconciling" || mode == "migrating" {
			return Problem("audit_protocol_unavailable", 503)
		}
		summary := map[string]any{"catalog_hash": ar.Hash(ar.Catalog())}
		if kind == "policy" {
			summary["source"] = "awaiting_configuration"
			if retention != nil {
				if *retention <= 0 || *retention > 36500*24*time.Hour {
					return Problem("audit_filter_invalid", 400)
				}
				summary["platform_seconds"] = strconv.FormatInt(int64(retention.Seconds()), 10)
				summary["tenant_seconds"] = summary["platform_seconds"]
				summary["project_seconds"] = nil
				summary["source"] = "legacy_config_import"
			}
		}
		out.ID = uuid.NewString()
		_, e := tx.Exec(ctx, `INSERT INTO audit_cleanup_plans(id,kind,epoch,cutoff,reason,summary) VALUES($1,$2,$3,clock_timestamp(),$4,$5)`, out.ID, kind, epoch, safeReason(reason), ar.JSON(summary))
		if e != nil {
			return e
		}
		switch kind {
		case "facts":
			_, e = tx.Exec(ctx, `INSERT INTO audit_cleanup_items(plan_id,object_id,policy_version) SELECT $1,id::text,retention_policy_version FROM account_security_events WHERE retain_until<=clock_timestamp() ORDER BY retain_until,id LIMIT $2`, out.ID, s.options.FactBatch)
		case "objects":
			_, e = tx.Exec(ctx, `INSERT INTO audit_cleanup_items(plan_id,object_id) SELECT $1,o.id::text FROM audit_export_objects o JOIN audit_export_generations g ON g.export_id=o.export_id AND g.generation=o.generation WHERE (o.state IN ('available','expired','invalidated','missing','delete_pending') AND o.expires_at<=clock_timestamp()) OR (o.state='intent' AND g.state IN ('succeeded','failed','cancelled') AND o.created_at<clock_timestamp()-interval '20 minutes') ORDER BY o.created_at,o.id LIMIT $2`, out.ID, s.options.ObjectBatch)
		case "backfill":
			_, e = tx.Exec(ctx, `INSERT INTO audit_cleanup_items(plan_id,object_id) SELECT $1,id::text FROM account_security_events WHERE schema_version=0 AND id>COALESCE((SELECT backfill_cursor FROM audit_source_states WHERE source_module='audit' AND source_kind='legacy'),0) ORDER BY id LIMIT $2`, out.ID, s.options.FactBatch)
		}
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE audit_cleanup_plans SET summary=summary||jsonb_build_object('candidates',(SELECT count(*)::text FROM audit_cleanup_items WHERE plan_id=$1),'active_holds',(SELECT count(*)::text FROM audit_retention_holds WHERE released_at IS NULL AND expires_at>clock_timestamp())) WHERE id=$1`, out.ID)
		if e != nil {
			return e
		}
		out, e = loadPlanTx(ctx, tx, out.ID)
		if e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "planned", out.ID, reason, map[string]any{"kind": kind, "candidates": out.Summary["candidates"]})
	})
	return out, e
}
func (s *Service) ApplyPlan(ctx context.Context, id, revision, request, reason string) (map[string]any, error) {
	if e := maintenanceInput(request, reason); e != nil {
		return nil, e
	}
	if !ar.ID(id) || !ar.Number(revision, 1) {
		return nil, Problem("audit_filter_invalid", 400)
	}
	var out map[string]any
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var saved map[string]any
		e := tx.QueryRow(ctx, `SELECT summary FROM audit_maintenance_receipts WHERE request_id=$1 AND plan_id=$2`, request, id).Scan(&saved)
		if e == nil {
			out = saved
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		p, e := loadPlanTx(ctx, tx, id)
		if e != nil {
			return e
		}
		var epoch, mode string
		if e = tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM audit_protocol WHERE id=1 FOR SHARE`).Scan(&epoch, &mode); e != nil {
			return e
		}
		if p.Revision != revision || p.Epoch != epoch || p.State != "planned" && p.State != "partial" {
			return Problem("audit_plan_changed", 409)
		}
		if mode != "ready" {
			return Problem("audit_protocol_unavailable", 503)
		}
		if p.Kind == "facts" {
			if e = tx.QueryRow(ctx, `SELECT audit_apply_fact_cleanup($1,$2,$3,$4)`, id, revision, request, safeReason(reason)).Scan(&out); e != nil {
				return e
			}
			return maintenanceFactTx(ctx, tx, "applied", request, reason, out)
		}
		if p.Kind == "objects" {
			return Problem("audit_plan_requires_object_apply", 409)
		}
		if p.Summary["catalog_hash"] != ar.Hash(ar.Catalog()) {
			return Problem("audit_plan_changed", 409)
		}
		out = map[string]any{"plan_id": id, "kind": p.Kind}
		switch p.Kind {
		case "policy":
			if p.Summary["source"] != "legacy_config_import" {
				return Problem("audit_policy_configuration_required", 409)
			}
			for _, class := range []string{"platform", "tenant", "project"} {
				var seconds any
				if class != "project" {
					value, ok := p.Summary[class+"_seconds"].(string)
					if !ok || !ar.Number(value, 1) {
						return Problem("audit_filter_invalid", 400)
					}
					seconds = value
				}
				_, e = tx.Exec(ctx, `INSERT INTO audit_retention_policies(class,version,keep_seconds,source) SELECT $1,COALESCE(max(version),0)+1,$2,'configuration_plan' FROM audit_retention_policies WHERE class=$1`, class, seconds)
				if e != nil {
					return e
				}
			}
			out["policy_applied"] = true
		case "backfill":
			rows, e := tx.Query(ctx, `SELECT object_id FROM audit_cleanup_items WHERE plan_id=$1 ORDER BY object_id::bigint`, id)
			if e != nil {
				return e
			}
			ids, e := pgx.CollectRows(rows, pgx.RowTo[string])
			if e != nil {
				return e
			}
			updated, unknown := 0, 0
			last := "0"
			for _, eventID := range ids {
				last = eventID
				var event, tenant, project, entry string
				var details map[string]any
				var schema int
				e = tx.QueryRow(ctx, `SELECT event_type,COALESCE(tenant_id::text,''),COALESCE(project_id::text,''),actor_scope,details,schema_version FROM account_security_events WHERE id=$1 FOR UPDATE`, eventID).Scan(&event, &tenant, &project, &entry, &details, &schema)
				if e != nil {
					return e
				}
				if schema != 0 {
					continue
				}
				d, ok := ar.Lookup(event)
				if !ok {
					unknown++
					continue
				}
				scope := Scope{Kind: "platform", Tenant: tenant, Project: project}
				if tenant != "" {
					scope.Kind = "tenant"
				}
				if project != "" {
					scope.Kind = "project"
				}
				visibility, e := ar.Visibility(d, scope, entry)
				if e != nil {
					unknown++
					continue
				}
				class := scope.Kind
				if visibility == "platform" || visibility == "platform_tenant_notice" {
					class = "platform"
				}
				missing := []string{"operation_id", "task_generation", "notification_event"}
				if _, ok := details["before"]; !ok {
					missing = append(missing, "before")
				}
				if _, ok := details["after"]; !ok {
					missing = append(missing, "after")
				}
				_, e = tx.Exec(ctx, `UPDATE account_security_events e SET visibility_class=$2,category=$3,phase=$4,occurred_at=e.created_at,time_source='legacy_recorded',missing_fields=$5,retention_class=p.class,retention_policy_version=p.version,retain_until=CASE WHEN p.keep_seconds IS NOT NULL THEN e.created_at+p.keep_seconds*interval '1 second' END FROM (SELECT * FROM audit_retention_policies WHERE class=$6 ORDER BY version DESC LIMIT 1) p WHERE e.id=$1 AND e.schema_version=0 AND e.visibility_class='legacy_unclassified'`, eventID, visibility, d.Category, d.Phase, missing, class)
				if e != nil {
					return e
				}
				updated++
			}
			_, e = tx.Exec(ctx, `INSERT INTO audit_source_states(source_module,source_kind,backfill_cursor,complete,observed_at,pending_count) VALUES('audit','legacy',$1,$2,clock_timestamp(),$3) ON CONFLICT(source_module,source_kind) DO UPDATE SET backfill_cursor=GREATEST(audit_source_states.backfill_cursor,EXCLUDED.backfill_cursor),complete=EXCLUDED.complete,observed_at=EXCLUDED.observed_at,pending_count=EXCLUDED.pending_count`, last, len(ids) < s.options.FactBatch, unknown)
			if e != nil {
				return e
			}
			out["classified"] = updated
			out["unclassified"] = unknown
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_cleanup_plans SET state='applied',revision=revision+1,applied_at=clock_timestamp(),reason=$2 WHERE id=$1`, id, safeReason(reason)); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,plan_id,reason,summary) VALUES($1,$2,$3,$4,$5)`, request, p.Kind+".apply", id, safeReason(reason), ar.JSON(out))
		if e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "applied", request, reason, out)
	})
	return out, e
}

type Hold struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ObjectID  string    `json:"object_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) Hold(ctx context.Context, kind, id, reason string, until time.Time) (Hold, error) {
	out := Hold{}
	if kind != "all" && kind != "backup" && kind != "event" && kind != "export_object" || reason == "" || !until.After(s.now()) || until.After(s.now().Add(30*24*time.Hour)) {
		return out, Problem("audit_filter_invalid", 400)
	}
	if kind == "event" && !ar.Number(id, 1) || kind == "export_object" && !ar.ID(id) || len(id) > 100 {
		return out, Problem("audit_filter_invalid", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(834327122)`); e != nil {
			return e
		}
		var deleting bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_export_objects WHERE state='delete_pending' AND ($1 IN ('all','backup') OR $1='export_object' AND id::text=$2))`, kind, id).Scan(&deleting); e != nil {
			return e
		}
		if deleting {
			return Problem("audit_cleanup_in_progress", 409)
		}
		if e := tx.QueryRow(ctx, `INSERT INTO audit_retention_holds(object_kind,object_id,reason,expires_at) VALUES($1,$2,$3,$4) RETURNING id::text,object_kind,object_id,expires_at`, kind, id, safeReason(reason), until).Scan(&out.ID, &out.Kind, &out.ObjectID, &out.ExpiresAt); e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "hold_created", out.ID, reason, map[string]string{"kind": kind, "hold_id": out.ID})
	})
	return out, e
}
func (s *Service) ReleaseHold(ctx context.Context, id, request, reason string) error {
	if !ar.ID(id) {
		return Problem("audit_filter_invalid", 400)
	}
	if e := maintenanceInput(request, reason); e != nil {
		return e
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(834327122)`); e != nil {
			return e
		}
		var savedOperation, savedID string
		e := tx.QueryRow(ctx, `SELECT operation,summary->>'hold_id' FROM audit_maintenance_receipts WHERE request_id=$1`, request).Scan(&savedOperation, &savedID)
		if e == nil {
			if savedOperation != "hold.release" || savedID != id {
				return Problem("request_conflict", 409)
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE audit_retention_holds SET released_at=clock_timestamp() WHERE id=$1 AND released_at IS NULL`, id)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,reason,summary) VALUES($1,'hold.release',$2,$3) ON CONFLICT(request_id) DO NOTHING`, request, safeReason(reason), ar.JSON(map[string]string{"hold_id": id}))
		if e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "hold_released", request, reason, map[string]string{"hold_id": id})
	})
}

type Verification struct {
	Checked      string `json:"checked"`
	Invalid      string `json:"invalid"`
	Unclassified string `json:"unclassified"`
	Mode         string `json:"mode"`
}

func (s *Service) Verify(ctx context.Context, mode string) (Verification, error) {
	out := Verification{Mode: mode}
	if mode != "metadata" && mode != "content" {
		return out, Problem("audit_filter_invalid", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT checked::text,invalid::text,unclassified::text FROM audit_verification_metadata`).Scan(&out.Checked, &out.Invalid, &out.Unclassified)
	})
	if e != nil || mode == "metadata" {
		return out, e
	}
	if s.objects == nil {
		return out, Problem("audit_storage_unavailable", 503)
	}
	type object struct {
		key, hash string
		size      int64
	}
	objects := []object{}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT object_key,sha256,size_bytes FROM audit_export_objects WHERE state='available' ORDER BY id LIMIT $1`, s.options.ObjectBatch)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v object
			if e = rows.Scan(&v.key, &v.hash, &v.size); e != nil {
				return e
			}
			objects = append(objects, v)
		}
		return rows.Err()
	})
	if e != nil {
		return out, e
	}
	invalid := 0
	for _, o := range objects {
		r, e := s.objects.Get(ctx, o.key)
		if e != nil {
			invalid++
			continue
		}
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(r, o.size+1))
		closeErr := r.Close()
		if e != nil || closeErr != nil || n != o.size || hex.EncodeToString(h.Sum(nil)) != o.hash {
			invalid++
		}
	}
	out.Checked = strconv.Itoa(len(objects))
	out.Invalid = strconv.Itoa(invalid)
	return out, nil
}

func deniedMaintenance(e error) bool { var p *Error; return errors.As(e, &p) && p.Status == 409 }
