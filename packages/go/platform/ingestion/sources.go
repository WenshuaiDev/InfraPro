package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func (s *Service) Relays(ctx context.Context, a access.Actor) (map[string]any, error) {
	out := map[string]any{"items": []json.RawMessage{}, "configured": []map[string]string{}, "enabled": s.options.Enabled}
	scope := k.Platform()
	e := s.transact(ctx, a, scope, "relays", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT to_jsonb(r)||jsonb_build_object('revision',r.revision::text,'fresh',COALESCE(observed_at>clock_timestamp()-interval '60 seconds',false)) FROM ingestion_relays r ORDER BY created_at LIMIT 100`)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []json.RawMessage{}
		for rows.Next() {
			var raw json.RawMessage
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			items = append(items, raw)
		}
		out["items"] = items
		available := []map[string]string{}
		for _, v := range s.options.Relays {
			available = append(available, map[string]string{"id": v.ID, "configuration_ref": v.Ref})
		}
		out["configured"] = available
		return rows.Err()
	})
	return out, e
}
func (s *Service) CreateRelay(ctx context.Context, a access.Actor, in RelayInput) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	cfg, e := s.relay(in.ID)
	if e != nil || cfg.Ref != in.ConfigurationRef {
		return out, k.Invalid("/configuration_ref", "unavailable", "请选择部署已批准的云节点配置引用。")
	}
	if e = k.String(in.Name, "/name", 100, true); e != nil {
		return out, e
	}
	e = s.transact(ctx, a, k.Platform(), "relays", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		if prev, ok, e := replay(ctx, tx, auth, "relays.create", in.Meta, in); e != nil || ok {
			out = prev
			return e
		}
		out = Result{ID: in.ID, Revision: "1", State: "configured"}
		if _, e := tx.Exec(ctx, `INSERT INTO ingestion_relays(id,name,configuration_ref) VALUES($1,$2,$3)`, in.ID, in.Name, in.ConfigurationRef); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "relays.create", in.Meta, in, out, nil, map[string]string{"id": out.ID, "state": out.State})
	})
	return out, e
}
func (s *Service) Sources(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Source], error) {
	out := k.Page[Source]{Items: []Source{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "sources", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, filterContext(auth, f, "ingestion.sources"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+sourceJSON+` FROM ingestion_sources s WHERE ($1='platform' OR tenant_id=$2::uuid) AND ($3='' OR name ILIKE '%'||$3||'%') AND ($4='' OR state=$4) AND ($5::uuid IS NULL OR project_id=$5) AND ($6::uuid IS NULL OR id>$6) ORDER BY id LIMIT $7`, scope.Entry, k.Nullable(scope.Tenant), f.Query, f.State, k.Nullable(f.ProjectID), k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := read[Source](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Source(ctx context.Context, a access.Actor, scope Scope, id string) (SourceDetail, error) {
	out := SourceDetail{Revisions: []SourceRevision{}}
	e := s.transact(ctx, a, scope, "sources", "detail", false, func(tx pgx.Tx, _ k.Authorization) error {
		var e error
		out.Source, e = sourceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+revisionJSON+` FROM ingestion_source_revisions r WHERE source_id=$1 ORDER BY revision DESC LIMIT 100`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			v, e := read[SourceRevision](rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Revisions = append(out.Revisions, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT status,observed_at FROM ingestion_relays WHERE id=$1`, out.Source.RelayID).Scan(&out.Status, &out.ObservedAt)
		out.Fresh = out.ObservedAt != nil && time.Since(*out.ObservedAt) < time.Minute
		// Tenant status is scoped to its source; never disclose another tenant's
		// queue sizes or node-wide connection directory.
		if scope.Entry != "platform" {
			out.Status = nil
		}
		return e
	})
	return out, e
}
func (s *Service) SaveSource(ctx context.Context, a access.Actor, id string, in SourceInput) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if !in.Segregated {
		return out, k.Invalid("/upstream_segregated", "unsupported_source", "本期仅支持上游已按项目和厂家分流的入口。")
	}
	if e := k.String(in.Name, "/name", 120, true); e != nil {
		return out, e
	}
	budget, e := strconv.ParseInt(in.Budget, 10, 64)
	if e != nil || budget < r.MaxBody {
		return out, k.Invalid("/budget_bytes", "budget", "请设置明确的云缓冲预算。")
	}
	if in.IdleSeconds == 0 {
		in.IdleSeconds = 180
	}
	action := "revision"
	if id == "" {
		action = "create"
	}
	e = s.transact(ctx, a, k.Platform(), "sources", action, true, func(tx pgx.Tx, auth k.Authorization) error {
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "sources."+action, in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		src := Source{ID: id, TenantID: in.TenantID, ProjectID: in.ProjectID, ManufacturerID: in.ManufacturerID, ManufacturerScope: in.ManufacturerScope, RelayID: in.RelayID, Name: in.Name, Revision: "0", AppliedGeneration: "0", Synthetic: in.Synthetic, NamespaceID: uuid.NewString()}
		if id != "" {
			var e error
			src, e = sourceTx(ctx, tx, k.Platform(), id)
			if e != nil {
				return e
			}
			if src.TenantID != in.TenantID || src.ProjectID != in.ProjectID || src.ManufacturerID != in.ManufacturerID || src.ManufacturerScope != in.ManufacturerScope || src.RelayID != in.RelayID || src.Synthetic != in.Synthetic {
				return k.Problem("source_identity_immutable", "业务归属变化须新建入口和命名空间。", 409)
			}
		} else {
			src.ID = uuid.NewString()
		}
		if e = k.Expect(in.Revision, src.Revision); e != nil {
			return e
		}
		if _, e = businessTx(ctx, tx, src, ""); e != nil {
			return e
		}
		var manufacturer bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_manufacturers WHERE id=$1 AND deleted_at IS NULL AND status='active' AND (($2='public' AND owner_tenant_id IS NULL) OR ($2='tenant' AND owner_tenant_id=$3)))`, src.ManufacturerID, src.ManufacturerScope, src.TenantID).Scan(&manufacturer); e != nil {
			return e
		}
		if !manufacturer {
			return k.Problem("routing_scope_mismatch", "厂家归属与入口不一致。", 409)
		}
		if _, e = s.relay(src.RelayID); e != nil {
			return e
		}
		protocol, e := s.parser.IngressProtocolTx(ctx, tx, in.ProtocolID, in.ProtocolVersion, true)
		if e != nil {
			return e
		}
		if protocol.IdentityConvention == "exclusive-authenticated-source" && (!in.Exclusive || len(in.FixedTerminal) != 11 || len(in.ProducerIDs) != 1) {
			return k.Problem("exclusive_source_required", "独占识别须固定认证身份和终端。", 409)
		}
		if protocol.IdentityConvention != "exclusive-authenticated-source" && in.FixedTerminal != "" {
			return k.Problem("fixed_terminal_not_supported", "此入口协议从报文识别身份。", 409)
		}
		// Validate every live association against the proposed exact protocol.
		if id != "" {
			rows, err := tx.Query(ctx, `SELECT `+associationJSON+` FROM ingestion_device_associations a WHERE source_id=$1 AND enabled AND end_sequence IS NULL`, id)
			if err != nil {
				return err
			}
			links := []Association{}
			for rows.Next() {
				v, err := read[Association](rows)
				if err != nil {
					rows.Close()
					return err
				}
				links = append(links, v)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return err
			}
			for _, link := range links {
				snap, err := s.parser.ProductionSnapshotTx(ctx, tx, src.TenantID, link.DeviceID, "")
				if err != nil {
					return err
				}
				if _, err = s.parser.CompatibilityTx(ctx, tx, protocol, link.Demultiplexer, link.InputView, snap.Snapshot.Method, false); err != nil {
					return k.Problem("active_ingress_incompatible", "新入口协议与有效设备关联不兼容，请先明确停用冲突关联。", 409)
				}
			}
		}
		snapshot := r.Source{IngressID: src.ID, NamespaceID: src.NamespaceID, Revision: num(src.Revision) + 1, RelayID: src.RelayID, TenantID: src.TenantID, ProjectID: src.ProjectID, ManufacturerID: src.ManufacturerID, ManufacturerScope: src.ManufacturerScope, ProtocolID: protocol.ID, ProtocolVersion: protocol.Version, ProtocolDigest: protocol.Digest, Transport: protocol.Transport, Address: in.Address, Port: in.Port, SourceCode: in.SourceCode, ProducerIDs: in.ProducerIDs, Adapter: protocol.Adapter, Active: true, Synthetic: src.Synthetic, Exclusive: in.Exclusive, FixedTerminal: in.FixedTerminal, IdleSeconds: in.IdleSeconds, Budget: budget}
		snapshot.Digest = r.SourceDigest(snapshot)
		if e = r.ValidateSource(snapshot); e != nil {
			return k.Invalid("/source", "invalid_source", "入口配置缺项或无效。")
		}
		rows, e := tx.Query(ctx, `SELECT r.snapshot FROM ingestion_sources s JOIN ingestion_source_revisions r ON r.source_id=s.id AND r.revision=s.revision WHERE s.id<>$1 AND s.state NOT IN ('retired','paused')`, src.ID)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw []byte
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			var other r.Source
			if e = json.Unmarshal(raw, &other); e != nil {
				rows.Close()
				return e
			}
			if r.EndpointConflicts(snapshot, other) {
				rows.Close()
				return k.Problem("listener_conflict", "监听地址／端口已被其他入口预约。", 409)
			}
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		if id == "" {
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_sources(id,namespace_id,tenant_id,project_id,manufacturer_id,manufacturer_scope,relay_id,name,synthetic) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, src.ID, src.NamespaceID, src.TenantID, src.ProjectID, src.ManufacturerID, src.ManufacturerScope, src.RelayID, in.Name, src.Synthetic); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_source_revisions(source_id,revision,snapshot,digest,protocol_id,protocol_version,command_id,expected_generation,segregated,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,$9,$10)`, src.ID, snapshot.Revision, k.JSON(snapshot), snapshot.Digest, protocol.ID, protocol.Version, uuid.NewString(), src.AppliedGeneration, a.ID, in.Reason); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_sources SET revision=$2,state='awaiting_tenant_confirmation',name=$3 WHERE id=$1`, src.ID, snapshot.Revision, in.Name); e != nil {
			return e
		}
		out = Result{ID: src.ID, Revision: numstr(snapshot.Revision), State: "awaiting_tenant_confirmation"}
		return complete(ctx, tx, auth, "sources."+action, in.Meta, payload, out, nil, map[string]string{"revision": out.Revision, "state": out.State})
	})
	return out, e
}
func (s *Service) ConfirmSource(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "sources", "confirm", true, func(tx pgx.Tx, auth k.Authorization) error {
		src, e := sourceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "sources.confirm", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if e = k.Expect(in.Revision, src.Revision); e != nil {
			return e
		}
		if _, e = businessTx(ctx, tx, src, ""); e != nil {
			return e
		}
		rev, e := revisionTx(ctx, tx, id, in.Revision)
		if e != nil {
			return e
		}
		if rev.Digest != in.Digest {
			return k.Problem("revision_conflict", "请重新核对准确入口修订及摘要。", 409)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_source_confirmations(source_id,revision,tenant_id,digest,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, id, in.Revision, scope.Tenant, in.Digest, a.ID, in.Reason); e != nil {
			return e
		}
		out = Result{ID: id, Revision: src.Revision, State: "confirmed"}
		return complete(ctx, tx, auth, "sources.confirm", in.Meta, payload, out, nil, map[string]string{"revision": out.Revision, "state": out.State})
	})
	return out, e
}
func (s *Service) SourceCommand(ctx context.Context, a access.Actor, id string, in Command) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if !s.options.Enabled {
		return out, k.Problem("ingestion_disabled", "接入功能尚未由部署启用。", 409)
	}
	e := s.transact(ctx, a, k.Platform(), "sources", "commands", true, func(tx pgx.Tx, auth k.Authorization) error {
		src, e := sourceTx(ctx, tx, k.Platform(), id)
		if e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "sources."+in.Action, in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if e = k.Expect(in.Revision, src.Revision); e != nil {
			return e
		}
		rev, e := revisionTx(ctx, tx, id, src.Revision)
		if e != nil {
			return e
		}
		switch in.Action {
		case "publish":
			if !rev.Confirmed {
				return k.Problem("awaiting_source_confirmation", "租户尚未确认这份准确修订。", 409)
			}
			if _, e = businessTx(ctx, tx, src, ""); e != nil {
				return e
			}
			if _, e = s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE ingestion_sources SET state='publishing',error_code='' WHERE id=$1`, id)
		case "pause", "resume":
			if src.AppliedRevision == nil {
				return k.Problem("source_not_applied", "入口尚未实际发布。", 409)
			}
			old, e := revisionTx(ctx, tx, id, *src.AppliedRevision)
			if e != nil {
				return e
			}
			snap := old.Snapshot
			snap.Revision = num(src.Revision) + 1
			snap.Active = in.Action == "resume"
			snap.Digest = r.SourceDigest(snap)
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_source_revisions(source_id,revision,snapshot,digest,protocol_id,protocol_version,command_id,expected_generation,segregated,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,$9,$10)`, id, snap.Revision, k.JSON(snap), snap.Digest, snap.ProtocolID, snap.ProtocolVersion, uuid.NewString(), src.AppliedGeneration, a.ID, in.Reason); e != nil {
				return e
			}
			// Stop/resume changes only reception. It inherits the exact previously
			// confirmed business agreement, recorded as an explicit inherited row.
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_source_confirmations(source_id,revision,tenant_id,digest,actor_id,reason,inherited_revision) SELECT source_id,$2,tenant_id,$3,actor_id,reason,revision FROM ingestion_source_confirmations WHERE source_id=$1 AND revision=$4`, id, snap.Revision, snap.Digest, *src.AppliedRevision); e != nil {
				return e
			}
			src.Revision = numstr(snap.Revision)
			_, e = tx.Exec(ctx, `UPDATE ingestion_sources SET revision=$2,state='publishing' WHERE id=$1`, id, snap.Revision)
		case "unblock":
			_, e = tx.Exec(ctx, `UPDATE ingestion_sources SET blocked=false WHERE id=$1`, id)
		case "block":
			_, e = tx.Exec(ctx, `UPDATE ingestion_sources SET blocked=true,planning_paused=true WHERE id=$1`, id)
		default:
			return k.Invalid("/action", "enum", "请选择发布、暂停接收、恢复接收或紧急阻断。")
		}
		if e != nil {
			return e
		}
		out = Result{ID: id, Revision: src.Revision, State: "publishing"}
		if in.Action == "block" {
			out.State = "blocked"
		}
		return complete(ctx, tx, auth, "sources."+in.Action, in.Meta, payload, out, nil, map[string]string{"state": out.State, "revision": out.Revision})
	})
	return out, e
}

// PublishOne resumes the persisted command after network failure without holding
// any business lock across remote IO or borrowing the submitting person's roles.
func (s *Service) PublishOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	var source Source
	var rev SourceRevision
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		source, e = read[Source](tx.QueryRow(ctx, `SELECT `+sourceJSON+` FROM ingestion_sources s WHERE state='publishing' ORDER BY created_at LIMIT 1`))
		if e != nil {
			return e
		}
		rev, e = revisionTx(ctx, tx, source.ID, source.Revision)
		return e
	})
	if e != nil {
		var p *k.Error
		if isMissing(e, &p) {
			return false, nil
		}
		return false, e
	}
	if !rev.Confirmed {
		return true, k.Problem("awaiting_source_confirmation", "修订未确认。", 409)
	}
	cfg, e := s.relay(source.RelayID)
	if e != nil {
		return true, e
	}
	var applied r.Applied
	e = relayCall(ctx, cfg, true, "/v1/control/source-revisions", r.Apply{CommandID: rev.CommandID, ExpectedGeneration: num(rev.ExpectedGeneration), Source: rev.Snapshot}, &applied)
	if e != nil {
		_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE ingestion_sources SET error_code='remote_application_unknown' WHERE id=$1 AND revision=$2`, source.ID, source.Revision)
			return e
		})
		return true, e
	}
	if applied.CommandID != rev.CommandID || applied.Digest != rev.Digest || applied.Generation != num(rev.ExpectedGeneration)+1 {
		return true, k.Problem("relay_invalid_response", "发布结果与命令不一致。", 409)
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ingestion_sources SET state=$3,applied_generation=$4,applied_revision=$2,application=$5,error_code=$6 WHERE id=$1 AND revision=$2`, source.ID, source.Revision, applied.State, applied.Generation, k.JSON(applied), applied.ErrorCode)
		return e
	})
	return true, e
}
func isMissing(e error, p **k.Error) bool {
	return errors.As(e, p) && (*p).Code == "ingestion_not_found"
}

// SourceOptions exposes only the scope directory needed to establish a trusted
// listener agreement, not product, device, message or credential information.
func (s *Service) SourceOptions(ctx context.Context, a access.Actor) (map[string]any, error) {
	out := map[string]any{}
	e := s.transact(ctx, a, k.Platform(), "source-options", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		for key, query := range map[string]string{
			"tenants":       `SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'name',name) ORDER BY name),'[]') FROM tenants WHERE status='active'`,
			"projects":      `SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'tenant_id',tenant_id,'name',name) ORDER BY name),'[]') FROM projects WHERE status='active'`,
			"manufacturers": `SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'tenant_id',owner_tenant_id,'name',name,'scope',CASE WHEN owner_tenant_id IS NULL THEN 'public' ELSE 'tenant' END) ORDER BY name),'[]') FROM device_manufacturers WHERE status='active' AND deleted_at IS NULL`,
		} {
			var value json.RawMessage
			if e := tx.QueryRow(ctx, query).Scan(&value); e != nil {
				return e
			}
			out[key] = value
		}
		return nil
	})
	return out, e
}
