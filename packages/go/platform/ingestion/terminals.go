package ingestion

import (
	"context"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateTerminal(ctx context.Context, a access.Actor, scope Scope, in TerminalInput) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if len(in.Key) != 11 || k.String(in.Name, "/name", 120, true) != nil || in.Mode != "single" && in.Mode != "multiple" || (in.Mode == "multiple") != (in.Demultiplexer != nil) {
		return out, k.Invalid("/terminal", "invalid_terminal", "请保留 11 字节终端编号，并明确单路／多路及固定解复用方法。")
	}
	for _, ch := range []byte(in.Key) {
		if ch < 32 || ch > 126 {
			return out, k.Invalid("/terminal_key", "identity_invalid", "终端编号必须保持原 ASCII 表示。")
		}
	}
	e := s.transact(ctx, a, scope, "terminals", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		src, e := sourceTx(ctx, tx, scope, in.SourceID)
		if e != nil {
			return e
		}
		rev, e := revisionTx(ctx, tx, src.ID, src.Revision)
		if e != nil {
			return e
		}
		if !rev.Confirmed {
			return k.Problem("awaiting_source_confirmation", "请先确认入口的准确修订。", 409)
		}
		if _, e = businessTx(ctx, tx, src, ""); e != nil {
			return e
		}
		if in.Demultiplexer != nil {
			m, e := s.parser.IdentificationTx(ctx, tx, *in.Demultiplexer, true)
			if e != nil {
				return e
			}
			if m.Purpose != "terminal_demultiplex" {
				return k.Invalid("/demultiplexer", "purpose", "请选择终端解复用能力。")
			}
		}
		if prev, ok, e := replay(ctx, tx, auth, "terminals.create", in.Meta, in); e != nil || ok {
			out = prev
			return e
		}
		out = Result{ID: uuid.NewString(), Revision: "1", State: "registered"}
		var demux any
		if in.Demultiplexer != nil {
			demux = k.JSON(in.Demultiplexer)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_terminals(id,tenant_id,source_id,terminal_key,name,mode,demultiplexer) VALUES($1,$2,$3,$4,$5,$6,$7)`, out.ID, scope.Tenant, src.ID, in.Key, in.Name, in.Mode, demux); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_terminal_revisions(terminal_id,revision,snapshot) SELECT id,revision,to_jsonb(t)||jsonb_build_object('revision',revision::text) FROM ingestion_terminals t WHERE id=$1`, out.ID); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "terminals.create", in.Meta, in, out, nil, map[string]string{"id": out.ID, "state": out.State})
	})
	return out, e
}
func (s *Service) Terminals(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Terminal], error) {
	out := k.Page[Terminal]{Items: []Terminal{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "terminals", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, filterContext(auth, f, "terminals"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+terminalJSON+` FROM ingestion_terminals t JOIN ingestion_sources s ON s.id=t.source_id WHERE t.tenant_id=$1 AND ($2::uuid IS NULL OR source_id=$2) AND ($3::uuid IS NULL OR s.project_id=$3) AND ($4='' OR t.name ILIKE '%'||$4||'%' OR terminal_key=$4) AND ($5::uuid IS NULL OR t.id>$5) ORDER BY t.id LIMIT $6`, scope.Tenant, k.Nullable(f.SourceID), k.Nullable(f.ProjectID), f.Query, k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := read[Terminal](rows)
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
func (s *Service) Terminal(ctx context.Context, a access.Actor, scope Scope, id string) (Terminal, error) {
	var out Terminal
	e := s.transact(ctx, a, scope, "terminals", "detail", false, func(tx pgx.Tx, _ k.Authorization) error {
		var e error
		out, e = terminalTx(ctx, tx, scope.Tenant, id)
		return e
	})
	return out, e
}
func (s *Service) Associations(ctx context.Context, a access.Actor, scope Scope, id string) ([]Association, error) {
	out := []Association{}
	e := s.transact(ctx, a, scope, "associations", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		if _, e := terminalTx(ctx, tx, scope.Tenant, id); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+associationJSON+` FROM ingestion_device_associations a WHERE terminal_id=$1 AND tenant_id=$2 ORDER BY created_at DESC LIMIT 200`, id, scope.Tenant)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := read[Association](rows)
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) associationPreviewTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, in AssociationInput) (Terminal, Source, error) {
	t, e := terminalTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return t, Source{}, e
	}
	src, e := sourceTx(ctx, tx, scope, t.SourceID)
	if e != nil {
		return t, src, e
	}
	if e = k.Expect(in.Revision, t.Revision); e != nil {
		return t, src, e
	}
	if !t.Enabled {
		return t, src, k.Problem("terminal_disabled", "终端已停用。", 409)
	}
	if t.Mode == "single" && (in.ChildKey != "" || (in.InputView != "complete_frame" && in.InputView != "json_reading")) || t.Mode == "multiple" && (in.ChildKey == "" || in.InputView != "json_reading") {
		return t, src, k.Invalid("/child_key", "input_mismatch", "整路与子地址的输入视图必须匹配终端模式。")
	}
	if _, e = businessTx(ctx, tx, src, in.DeviceID); e != nil {
		return t, src, e
	}
	rev, e := revisionTx(ctx, tx, src.ID, src.Revision)
	if e != nil {
		return t, src, e
	}
	if !rev.Confirmed {
		return t, src, k.Problem("awaiting_source_confirmation", "当前入口修订尚未确认。", 409)
	}
	p, e := s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false)
	if e != nil {
		return t, src, e
	}
	snapshot, e := s.parser.ProductionSnapshotTx(ctx, tx, scope.Tenant, in.DeviceID, "")
	if e != nil {
		return t, src, e
	}
	_, e = s.parser.CompatibilityTx(ctx, tx, p, t.Demultiplexer, in.InputView, snapshot.Snapshot.Method, false)
	return t, src, e
}
func (s *Service) ChangeAssociation(ctx context.Context, a access.Actor, scope Scope, id string, in AssociationInput, preview bool) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	action := "create"
	if preview {
		action = "preview"
	}
	e := s.transact(ctx, a, scope, "associations", action, !preview, func(tx pgx.Tx, auth k.Authorization) error {
		t, src, e := s.associationPreviewTx(ctx, tx, scope, id, in)
		if e != nil {
			return e
		}
		out = Result{ID: id, Revision: t.Revision, State: "compatible"}
		if preview {
			return nil
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "associations.create", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		out.ID = uuid.NewString()
		out.State = "pending_watermark"
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_change_intents(id,tenant_id,source_id,terminal_id,command_id,input,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, out.ID, scope.Tenant, src.ID, id, uuid.NewString(), k.JSON(in), a.ID, in.Reason); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "associations.create", in.Meta, payload, out, nil, map[string]string{"state": out.State, "device_id": in.DeviceID})
	})
	return out, e
}
func machineCommandActor(id string) access.Actor {
	return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}
func (s *Service) ChangeOne(ctx context.Context) (bool, error) {
	var id, tenant, source, terminal, command, actor string
	var raw []byte
	var saved []byte
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,source_id::text,terminal_id::text,command_id::text,actor_id::text,input,watermark FROM ingestion_change_intents WHERE state='pending' ORDER BY created_at LIMIT 1`).Scan(&id, &tenant, &source, &terminal, &command, &actor, &raw, &saved)
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var in AssociationInput
	if e = json.Unmarshal(raw, &in); e != nil {
		return true, e
	}
	var src Source
	e = s.access.Transact(ctx, func(tx pgx.Tx) error { var e error; src, e = sourceTx(ctx, tx, k.Tenant(tenant), source); return e })
	if e != nil {
		return true, e
	}
	var watermark r.Watermark
	if len(saved) > 0 {
		if e = json.Unmarshal(saved, &watermark); e != nil {
			return true, e
		}
	} else {
		cfg, e := s.relay(src.RelayID)
		if e != nil {
			return true, e
		}
		if e = relayCall(ctx, cfg, true, "/v1/control/source-watermarks", r.WatermarkRequest{CommandID: command, IngressID: source}, &watermark); e != nil {
			return true, e
		}
		if watermark.CommandID != command || watermark.IngressID != source || !access.ValidID(watermark.Epoch) {
			return true, k.Problem("relay_invalid_response", "水位与请求不一致。", 409)
		}
		if e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE ingestion_change_intents SET watermark=$2 WHERE id=$1 AND state='pending'`, id, k.JSON(watermark))
			return e
		}); e != nil {
			return true, e
		}
	}
	e = s.transact(ctx, machineCommandActor(actor), k.Tenant(tenant), "associations", "create", true, func(tx pgx.Tx, _ k.Authorization) error {
		var pending bool
		if e := tx.QueryRow(ctx, `SELECT state='pending' FROM ingestion_change_intents WHERE id=$1 FOR UPDATE`, id).Scan(&pending); e != nil {
			return e
		}
		if !pending {
			return nil
		}
		t, _, e := s.associationPreviewTx(ctx, tx, k.Tenant(tenant), terminal, in)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_epochs(relay_id,epoch,confirmed) SELECT $1,$2,NOT EXISTS(SELECT 1 FROM ingestion_epochs WHERE relay_id=$1) ON CONFLICT DO NOTHING`, src.RelayID, watermark.Epoch); e != nil {
			return e
		}
		var confirmed bool
		if e = tx.QueryRow(ctx, `SELECT confirmed FROM ingestion_epochs WHERE relay_id=$1 AND epoch=$2`, src.RelayID, watermark.Epoch).Scan(&confirmed); e != nil {
			return e
		}
		if !confirmed {
			return k.Problem("epoch_unconfirmed", "云缓冲时间线变化，须由平台明确核对。", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_device_associations SET end_sequence=$4 WHERE terminal_id=$1 AND child_key=$2 AND epoch=$3 AND end_sequence IS NULL`, terminal, in.ChildKey, watermark.Epoch, watermark.Sequence+1); e != nil {
			return e
		}
		var demux any
		if t.Demultiplexer != nil {
			demux = k.JSON(t.Demultiplexer)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_device_associations(tenant_id,source_id,terminal_id,terminal_revision,child_key,device_id,epoch,start_sequence,input_view,demultiplexer) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, tenant, source, terminal, t.Revision, in.ChildKey, in.DeviceID, watermark.Epoch, watermark.Sequence+1, in.InputView, demux); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_terminals SET revision=revision+1 WHERE id=$1`, terminal); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_terminal_revisions(terminal_id,revision,snapshot) SELECT id,revision,to_jsonb(t)||jsonb_build_object('revision',revision::text) FROM ingestion_terminals t WHERE id=$1`, terminal); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE ingestion_change_intents SET state='applied',error_code='' WHERE id=$1`, id)
		return e
	})
	if e != nil {
		_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE ingestion_change_intents SET error_code=$2 WHERE id=$1 AND state='pending'`, id, code(e))
			return err
		})
	}
	return true, e
}
func (s *Service) AssociationState(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if in.Action != "enable" && in.Action != "disable" {
		return out, k.Invalid("/action", "enum", "请选择启用或停用关联。")
	}
	e := s.transact(ctx, a, scope, "associations", "state", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, e := associationTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		t, e := terminalTx(ctx, tx, scope.Tenant, v.TerminalID)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, t.Revision); e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "associations.state", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if in.Action == "enable" {
			if v.EndSequence != nil {
				return k.Problem("historical_association", "已结束区间不能重新启用，请建立新关联区间。", 409)
			}
			if _, _, e = s.associationPreviewTx(ctx, tx, scope, t.ID, AssociationInput{Revision: t.Revision, ChildKey: v.ChildKey, DeviceID: v.DeviceID, InputView: v.InputView}); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_device_associations SET enabled=$2 WHERE id=$1`, id, in.Action == "enable"); e != nil {
			return e
		}
		out = Result{ID: id, Revision: t.Revision, State: in.Action}
		return complete(ctx, tx, auth, "associations.state", in.Meta, payload, out, nil, map[string]string{"state": out.State})
	})
	return out, e
}
func (s *Service) CancelChange(ctx context.Context, a access.Actor, scope Scope, id string, in Meta) (Result, error) {
	var out Result
	if e := meta(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "changes", "cancel", true, func(tx pgx.Tx, auth k.Authorization) error {
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "changes.cancel", in, payload); e != nil || ok {
			out = prev
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE ingestion_change_intents SET state='canceled' WHERE id=$1 AND tenant_id=$2 AND state='pending'`, id, scope.Tenant)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return k.NotFound("ingestion")
		}
		out = Result{ID: id, State: "canceled", Revision: "1"}
		return complete(ctx, tx, auth, "changes.cancel", in, payload, out, nil, map[string]string{"state": "canceled"})
	})
	return out, e
}
func (s *Service) CheckAdoptionTx(ctx context.Context, tx pgx.Tx, tenantID, deviceID string, target *parsing.Snapshot) (parsing.IngressImpact, error) {
	out := parsing.IngressImpact{Entries: []parsing.IngressLink{}, Unplanned: "0"}
	rows, e := tx.Query(ctx, `SELECT a.id::text,a.source_id::text,s.project_id::text,a.demultiplexer,a.input_view,r.snapshot FROM ingestion_device_associations a JOIN ingestion_sources s ON s.id=a.source_id JOIN ingestion_source_revisions r ON r.source_id=s.id AND r.revision=s.revision WHERE a.tenant_id=$1 AND a.device_id=$2 AND a.enabled AND a.end_sequence IS NULL AND s.state NOT IN ('retired','paused')`, tenantID, deviceID)
	if e != nil {
		return out, e
	}
	type item struct {
		link   parsing.IngressLink
		demux  *i.Method
		view   string
		source r.Source
	}
	list := []item{}
	for rows.Next() {
		var v item
		var demux, raw []byte
		if e = rows.Scan(&v.link.AssociationID, &v.link.SourceID, &v.link.ProjectID, &demux, &v.view, &raw); e != nil {
			rows.Close()
			return out, e
		}
		if len(demux) > 0 {
			if e = json.Unmarshal(demux, &v.demux); e != nil {
				rows.Close()
				return out, e
			}
		}
		if e = json.Unmarshal(raw, &v.source); e != nil {
			rows.Close()
			return out, e
		}
		list = append(list, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return out, e
	}
	for _, v := range list {
		if target == nil {
			v.link.ErrorCode = "parser_unconfigured"
		} else {
			p, err := s.parser.IngressProtocolTx(ctx, tx, v.source.ProtocolID, v.source.ProtocolVersion, false)
			if err == nil {
				_, err = s.parser.CompatibilityTx(ctx, tx, p, v.demux, v.view, target.Method, false)
			}
			v.link.Compatible = err == nil
			v.link.ErrorCode = code(err)
		}
		out.Entries = append(out.Entries, v.link)
	}
	if len(list) > 0 {
		out.Unknown = true
		e = tx.QueryRow(ctx, `SELECT count(*)::text FROM ingestion_units u WHERE u.tenant_id=$1 AND u.device_id=$2 AND NOT EXISTS(SELECT 1 FROM ingestion_parse_plans p WHERE p.unit_id=u.id)`, tenantID, deviceID).Scan(&out.Unplanned)
	}
	return out, e
}

func (s *Service) ReviseTerminal(ctx context.Context, a access.Actor, scope Scope, id string, in TerminalInput) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	if k.String(in.Name, "/name", 120, true) != nil || in.Mode != "single" && in.Mode != "multiple" || (in.Mode == "multiple") != (in.Demultiplexer != nil) {
		return out, k.Invalid("/terminal", "invalid_terminal", "请选择终端模式与准确解复用版本。")
	}
	e := s.transact(ctx, a, scope, "terminals", "revision", true, func(tx pgx.Tx, auth k.Authorization) error {
		old, e := terminalTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "terminals.revision", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if e = k.Expect(in.Revision, old.Revision); e != nil {
			return e
		}
		if old.SourceID != in.SourceID || old.Key != in.Key {
			return k.Problem("terminal_identity_immutable", "入口与终端编号不可修改，请新建终端。", 409)
		}
		changed := old.Mode != in.Mode || k.Hash(old.Demultiplexer) != k.Hash(in.Demultiplexer)
		if in.Demultiplexer != nil && changed {
			m, e := s.parser.IdentificationTx(ctx, tx, *in.Demultiplexer, true)
			if e != nil {
				return e
			}
			if m.Purpose != "terminal_demultiplex" {
				return k.Invalid("/demultiplexer", "purpose", "请选择终端解复用能力。")
			}
		}
		var busy bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingestion_change_intents WHERE terminal_id=$1 AND state='pending') OR ($2 AND EXISTS(SELECT 1 FROM ingestion_device_associations WHERE terminal_id=$1 AND enabled AND end_sequence IS NULL))`, id, changed).Scan(&busy); e != nil {
			return e
		}
		if busy {
			return k.Problem("active_association", "请先完成关联变更并明确停用冲突关联。", 409)
		}
		enabled := old.Enabled
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		var demux any
		if in.Demultiplexer != nil {
			demux = k.JSON(in.Demultiplexer)
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_terminals SET name=$2,mode=$3,demultiplexer=$4,enabled=$5,revision=revision+1 WHERE id=$1`, id, in.Name, in.Mode, demux, enabled); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_terminal_revisions(terminal_id,revision,snapshot) SELECT id,revision,to_jsonb(t)||jsonb_build_object('revision',revision::text) FROM ingestion_terminals t WHERE id=$1`, id); e != nil {
			return e
		}
		out = Result{ID: id, Revision: numstr(num(old.Revision) + 1), State: "registered"}
		return complete(ctx, tx, auth, "terminals.revision", in.Meta, payload, out, map[string]string{"revision": old.Revision}, map[string]string{"revision": out.Revision})
	})
	return out, e
}
func (s *Service) TerminalHistory(ctx context.Context, a access.Actor, scope Scope, id string) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	e := s.transact(ctx, a, scope, "terminals", "history", false, func(tx pgx.Tx, _ k.Authorization) error {
		if _, e := terminalTx(ctx, tx, scope.Tenant, id); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT snapshot FROM ingestion_terminal_revisions WHERE terminal_id=$1 ORDER BY revision DESC LIMIT 100`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var b json.RawMessage
			if e = rows.Scan(&b); e != nil {
				return e
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, e
}
