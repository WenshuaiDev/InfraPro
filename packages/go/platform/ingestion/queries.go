package ingestion

import (
	"context"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/jackc/pgx/v5"
)

type Message struct {
	Unit         Unit    `json:"unit"`
	Receipt      Receipt `json:"receipt"`
	Plans        []Plan  `json:"plans"`
	CanRaw       bool    `json:"can_raw"`
	CanResults   bool    `json:"can_results"`
	CanReprocess bool    `json:"can_reprocess"`
}

func (s *Service) unitAuthorizedTx(ctx context.Context, tx pgx.Tx, a k.Authorization, id string) (Unit, Receipt, error) {
	var empty Receipt
	u, e := unitTx(ctx, tx, id)
	if e != nil {
		return u, empty, e
	}
	if u.TenantID == nil || *u.TenantID != a.Tenant || u.DeviceID == nil || u.TerminalID == nil {
		return Unit{}, empty, k.NotFound("ingestion")
	}
	if e = a.Require(access.TenantDeviceRead, access.TenantIngestionTerminalRead, access.TenantIngestionMessageRead); e != nil {
		return u, empty, e
	}
	receipt, e := receiptTx(ctx, tx, u.ReceiptID)
	if e != nil {
		return u, empty, e
	}
	src, e := sourceTx(ctx, tx, a.Scope, receipt.SourceID)
	if e != nil {
		return u, empty, e
	}
	if _, e = businessTx(ctx, tx, src, *u.DeviceID); e != nil {
		return Unit{}, empty, k.NotFound("ingestion")
	}
	return u, receipt, nil
}
func (s *Service) Messages(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[json.RawMessage], error) {
	out := k.Page[json.RawMessage]{Items: []json.RawMessage{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "messages", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, filterContext(auth, f, "messages"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT u.id::text,jsonb_build_object('id',u.id,'receipt_id',r.id,'relay_receipt_id',r.receipt_id,'source_id',r.source_id,'source_project_id',r.project_id,'terminal_id',u.terminal_id,'terminal_key',u.terminal_key,'child_key',u.child_key,'device_id',u.device_id,'state',u.state,'error_code',u.error_code,'message_kind',u.message_kind,'received_at',r.received_at,'stored_at',r.stored_at,'byte_length',u.view->>'byte_length','sha256',u.view->>'sha256','synthetic',s.synthetic,'ingress_protocol',r.envelope->>'ingress_protocol_id','ingress_version',r.envelope->>'ingress_protocol_version','method_version',(SELECT snapshot->'parsing'->'entry'->>'method_version' FROM ingestion_parse_plans WHERE unit_id=u.id LIMIT 1),'acknowledgement',(SELECT state FROM ingestion_ack_outbox WHERE receipt_id=r.id)) FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id JOIN ingestion_sources s ON s.id=r.source_id JOIN devices d ON d.id=u.device_id AND d.tenant_id=u.tenant_id JOIN projects p ON p.id=s.project_id WHERE u.tenant_id=$1 AND d.status='enabled' AND d.deleted_at IS NULL AND p.status='active' AND EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=u.tenant_id AND g.project_id=s.project_id AND g.device_id=u.device_id AND g.kind='device' AND g.ended_at IS NULL) AND ($2::uuid IS NULL OR s.id=$2) AND ($3::uuid IS NULL OR s.project_id=$3) AND ($4::uuid IS NULL OR u.terminal_id=$4) AND ($5::uuid IS NULL OR u.device_id=$5) AND ($6::uuid IS NULL OR r.receipt_id=$6 OR r.id=$6) AND ($7='' OR u.state=$7) AND ($8='' OR u.error_code=$8) AND ($9::timestamptz IS NULL OR r.received_at>=$9) AND ($10::timestamptz IS NULL OR r.received_at<$10) AND ($11::uuid IS NULL OR u.id>$11) AND ($12='' OR EXISTS(SELECT 1 FROM ingestion_parse_plans x WHERE x.unit_id=u.id AND x.snapshot->'parsing'->'entry'->>'method_version'=$12)) ORDER BY u.id LIMIT $13`, scope.Tenant, k.Nullable(f.SourceID), k.Nullable(f.ProjectID), k.Nullable(f.TerminalID), k.Nullable(f.DeviceID), k.Nullable(f.ReceiptID), f.State, f.ErrorCode, f.From, f.Until, k.Nullable(cur.ID), f.MethodVersion, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		last := ""
		for rows.Next() {
			var id string
			var raw json.RawMessage
			if e = rows.Scan(&id, &raw); e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: last}.Encode()
				break
			}
			last = id
			out.Items = append(out.Items, raw)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Message(ctx context.Context, a access.Actor, scope Scope, id string) (Message, error) {
	out := Message{Plans: []Plan{}}
	e := s.transact(ctx, a, scope, "messages", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out.Unit, out.Receipt, e = s.unitAuthorizedTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		out.CanRaw = auth.Has(access.TenantIngestionRawRead)
		out.CanResults = auth.Has(access.TenantParsingRunRead)
		out.CanReprocess = auth.Has(access.TenantIngestionReprocess) && auth.Has(access.TenantParsingMethodRead)
		rows, e := tx.Query(ctx, `SELECT `+planJSON+` FROM ingestion_parse_plans p WHERE unit_id=$1 ORDER BY created_at LIMIT 100`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := read[Plan](rows)
			if e != nil {
				return e
			}
			out.Plans = append(out.Plans, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) CheckContent(ctx context.Context, a access.Actor, scope Scope, id string, result bool) error {
	action := "content"
	if result {
		action = "response"
	}
	return s.transact(ctx, a, scope, "messages", action, false, func(tx pgx.Tx, auth k.Authorization) error {
		_, _, e := s.unitAuthorizedTx(ctx, tx, auth, id)
		return e
	})
}
func (s *Service) Content(ctx context.Context, a access.Actor, scope Scope, id string) ([]byte, error) {
	var obj object
	var view i.View
	e := s.transact(ctx, a, scope, "messages", "content", false, func(tx pgx.Tx, auth k.Authorization) error {
		u, r, e := s.unitAuthorizedTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		view = u.View
		obj, e = objectTx(ctx, tx, r.ObjectID)
		return e
	})
	if e != nil {
		return nil, e
	}
	raw, e := s.readObject(ctx, obj)
	if e != nil {
		return nil, e
	}
	raw, e = i.Materialize(raw, view)
	if e != nil {
		return nil, e
	}
	e = s.transact(ctx, a, scope, "messages", "content", true, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := s.unitAuthorizedTx(ctx, tx, auth, id); e != nil {
			return e
		}
		return access.Audit(ctx, tx, "ingestion.messages.read", "ingestion_messages", id, "", nil, map[string]any{"id": id, "sha256": c.Digest(raw), "size_bytes": len(raw)}, access.Meta{ActorID: a.ID, ActorScope: scope.Entry, TenantID: scope.Tenant})
	})
	return raw, e
}
func (s *Service) Decisions(ctx context.Context, a access.Actor, scope Scope, id string) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	e := s.transact(ctx, a, scope, "messages", "decisions", false, func(tx pgx.Tx, auth k.Authorization) error {
		u, _, e := s.unitAuthorizedTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		// Only this authorized unit's transform and assignment are returned, never
		// the batch identification response's sibling identity directory.
		rows, e := tx.Query(ctx, `SELECT jsonb_build_object('unit_id',u.id,'unit_key',u.unit_key,'input_view',u.view,'assignment',u.assignment,'state',u.state,'error_code',u.error_code,'decision_id',d.id,'generation',d.generation::text,'method',d.method,'decision_state',d.state,'decision_error',d.error_code,'created_at',u.created_at) FROM ingestion_units u JOIN ingestion_identity_decisions d ON d.id=u.decision_id WHERE u.receipt_id=$1 AND u.unit_key=$2 AND u.tenant_id=$3 AND u.device_id=$4 ORDER BY u.created_at DESC LIMIT 100`, u.ReceiptID, u.Key, scope.Tenant, *u.DeviceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v json.RawMessage
			if e = rows.Scan(&v); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}

type ResultView struct {
	ID       string            `json:"id"`
	PlanID   string            `json:"plan_id"`
	Status   string            `json:"status"`
	SHA256   string            `json:"sha256"`
	Response c.Response        `json:"response"`
	Mapping  []parsing.Mapping `json:"mapping"`
	Quality  string            `json:"business_quality"`
	Adopted  bool              `json:"formally_adopted"`
}

func (s *Service) Results(ctx context.Context, a access.Actor, scope Scope, id, resultID string) ([]ResultView, []byte, error) {
	views := []ResultView{}
	objects := []object{}
	canRaw := false
	action := "results"
	if resultID != "" {
		action = "response"
	}
	authorize := func() error {
		return s.transact(ctx, a, scope, "messages", action, false, func(tx pgx.Tx, auth k.Authorization) error {
			_, _, e := s.unitAuthorizedTx(ctx, tx, auth, id)
			if e != nil {
				return e
			}
			canRaw = auth.Has(access.TenantIngestionRawRead)
			if len(views) > 0 {
				return nil
			}
			rows, e := tx.Query(ctx, `SELECT r.id::text,r.plan_id::text,r.status,r.mapping,o.id::text,o.object_key,o.sha256,o.byte_length FROM ingestion_results r JOIN ingestion_parse_plans p ON p.id=r.plan_id JOIN ingestion_objects o ON o.id=r.response_object_id WHERE p.unit_id=$1 AND p.tenant_id=$2 AND ($3::uuid IS NULL OR r.id=$3) ORDER BY r.created_at DESC LIMIT 100`, id, scope.Tenant, k.Nullable(resultID))
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var v ResultView
				var obj object
				var mapping []byte
				if e = rows.Scan(&v.ID, &v.PlanID, &v.Status, &mapping, &obj.ID, &obj.Key, &obj.Hash, &obj.Size); e != nil {
					return e
				}
				if e = json.Unmarshal(mapping, &v.Mapping); e != nil {
					return e
				}
				v.SHA256 = obj.Hash
				v.Quality = "not_evaluated"
				views = append(views, v)
				objects = append(objects, obj)
			}
			return rows.Err()
		})
	}
	if e := authorize(); e != nil {
		return nil, nil, e
	}
	for n, obj := range objects {
		raw, e := s.readObject(ctx, obj)
		if e != nil {
			return nil, nil, e
		}
		if e = authorize(); e != nil {
			return nil, nil, e
		}
		if resultID != "" {
			e = s.transact(ctx, a, scope, "messages", "response", true, func(tx pgx.Tx, auth k.Authorization) error {
				if _, _, e := s.unitAuthorizedTx(ctx, tx, auth, id); e != nil {
					return e
				}
				return access.Audit(ctx, tx, "ingestion.messages.response", "ingestion_messages", id, "", nil, map[string]string{"result_id": resultID, "sha256": obj.Hash}, access.Meta{ActorID: a.ID, ActorScope: scope.Entry, TenantID: scope.Tenant})
			})
			return views, raw, e
		}
		if e = c.StrictJSON(raw, &views[n].Response); e != nil {
			return nil, nil, c.Fail("integrity_failed")
		}
		if !canRaw {
			views[n].Response = c.RedactRaw(views[n].Response)
		}
	}
	return views, nil, nil
}
func (s *Service) Unmatched(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[json.RawMessage], error) {
	out := k.Page[json.RawMessage]{Items: []json.RawMessage{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	kind := "unmatched"
	if scope.Entry == "platform" {
		kind = "quarantine"
	}
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, filterContext(auth, f, kind))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT r.id::text,jsonb_build_object('id',r.id,'relay_id',r.relay_id,'receipt_id',r.receipt_id,'epoch',r.epoch,'source_id',r.source_id,'source_revision',r.source_revision::text,'state',r.state,'error_code',r.error_code,'received_at',r.received_at,'stored_at',r.stored_at,'byte_length',r.envelope->>'byte_length','evidence_kind',r.envelope->>'evidence_kind','truncated',r.envelope->'truncated','unmatched_units',(SELECT count(*)::text FROM ingestion_units u WHERE u.receipt_id=r.id AND u.device_id IS NULL)) FROM ingestion_receipts r LEFT JOIN ingestion_sources s ON s.id=r.source_id WHERE ($1='platform' AND r.state='quarantined' OR $1='tenant' AND s.tenant_id=$2::uuid AND EXISTS(SELECT 1 FROM ingestion_source_confirmations c WHERE c.source_id=s.id) AND (r.state IN ('quarantined','awaiting_identity','blocked') OR EXISTS(SELECT 1 FROM ingestion_units u WHERE u.receipt_id=r.id AND u.state IN ('awaiting_identity','awaiting_association','awaiting_configuration','quarantined','blocked')))) AND ($3::uuid IS NULL OR r.source_id=$3) AND ($4='' OR r.error_code=$4) AND ($5::uuid IS NULL OR r.id>$5) ORDER BY r.id LIMIT $6`, scope.Entry, k.Nullable(scope.Tenant), k.Nullable(f.SourceID), f.ErrorCode, k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		last := ""
		for rows.Next() {
			var id string
			var raw json.RawMessage
			if e = rows.Scan(&id, &raw); e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: last}.Encode()
				break
			}
			last = id
			out.Items = append(out.Items, raw)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) CloseUnmatched(ctx context.Context, a access.Actor, scope Scope, id string, in Meta) (Result, error) {
	var out Result
	if e := meta(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "unmatched", "close", true, func(tx pgx.Tx, auth k.Authorization) error {
		r, e := receiptTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if _, e = sourceTx(ctx, tx, scope, r.SourceID); e != nil {
			return e
		}
		payload := []any{id, in}
		if prev, ok, e := replay(ctx, tx, auth, "unmatched.close", in, payload); e != nil || ok {
			out = prev
			return e
		}
		out = Result{ID: id, Revision: r.Generation, State: "disposition_recorded"}
		return complete(ctx, tx, auth, "unmatched.close", in, payload, out, nil, map[string]string{"state": out.State})
	})
	return out, e
}
func (s *Service) DeviceSummary(ctx context.Context, a access.Actor, scope Scope, id string) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	e := s.transact(ctx, a, scope, "device", "detail", false, func(tx pgx.Tx, _ k.Authorization) error {
		var allowed bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices d WHERE d.tenant_id=$1 AND d.id=$2 AND d.deleted_at IS NULL AND d.status='enabled' AND ($3='tenant' OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=d.tenant_id AND g.project_id=$4 AND g.device_id=d.id AND g.kind='device' AND g.ended_at IS NULL)))`, scope.Tenant, id, scope.Entry, k.Nullable(scope.Project)).Scan(&allowed)
		if e != nil {
			return e
		}
		if !allowed {
			return k.NotFound("device")
		}
		rows, e := tx.Query(ctx, `SELECT jsonb_build_object('source_id',s.id,'source_project_id',s.project_id,'state',s.state,'planning_paused',s.planning_paused,'last_received_at',(SELECT max(r.received_at) FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE u.device_id=$2 AND u.tenant_id=$1 AND r.source_id=s.id),'last_parsed_at',(SELECT max(p.finished_at) FROM ingestion_parse_plans p WHERE p.device_id=$2 AND p.tenant_id=$1 AND p.snapshot->'source'->>'ingress_id'=s.id::text AND p.state='parsed')) FROM ingestion_device_associations a JOIN ingestion_sources s ON s.id=a.source_id WHERE a.tenant_id=$1 AND a.device_id=$2 AND a.enabled AND a.end_sequence IS NULL AND EXISTS(SELECT 1 FROM projects pr JOIN project_device_grants g ON g.project_id=pr.id WHERE pr.id=s.project_id AND pr.status='active' AND g.device_id=$2 AND g.ended_at IS NULL AND g.kind='device') AND ($3='tenant' OR s.project_id=$4::uuid) ORDER BY a.created_at LIMIT 100`, scope.Tenant, id, scope.Entry, k.Nullable(scope.Project))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw json.RawMessage
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			out = append(out, raw)
		}
		return rows.Err()
	})
	return out, e
}

func (s *Service) IntegrityEvents(ctx context.Context, a access.Actor) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	e := s.transact(ctx, a, k.Platform(), "integrity-events", "list", false, func(tx pgx.Tx, _ k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT to_jsonb(e)||jsonb_build_object('occurrences',occurrences::text,'source_revision',source_revision::text) FROM ingestion_integrity_events e ORDER BY last_seen_at DESC LIMIT 100`)
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
