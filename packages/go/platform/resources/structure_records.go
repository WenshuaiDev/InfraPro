package resources

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// BoreholePosition records two independent measurements. Nil means unknown; zero is a real measurement.
type BoreholePosition struct {
	ID                string     `json:"id,omitempty"`
	BoreholeID        string     `json:"borehole_id"`
	Revision          int        `json:"revision,omitempty"`
	DistanceM         *float64   `json:"distance_m"`
	ReferenceStart    string     `json:"reference_start"`
	Direction         string     `json:"direction"`
	Unit              string     `json:"unit"`
	VerticalDepthM    *float64   `json:"vertical_depth_m"`
	VerticalDatum     string     `json:"vertical_datum"`
	Source            string     `json:"source"`
	MeasuredAt        *string    `json:"measured_at"`
	BoreholeRevision  int        `json:"borehole_revision"`
	ConsistencyReason string     `json:"consistency_reason"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
}
type StructureRecordInput struct {
	WriteMeta
	Attachment  *AttachmentRef    `json:"attachment,omitempty"`
	Action      string            `json:"action"`
	Position    *BoreholePosition `json:"position,omitempty"`
	Target      *StructureRef     `json:"target,omitempty"`
	RecordID    string            `json:"record_id,omitempty"`
	Purpose     string            `json:"purpose,omitempty"`
	Disposition string            `json:"disposition,omitempty"`
}
type StructureRecordPreview struct {
	Hash     string   `json:"preview_hash"`
	Shared   bool     `json:"shared"`
	Warnings []string `json:"warnings"`
}
type StructureRecordPage struct {
	Items            []json.RawMessage `json:"items"`
	NextCursor       string            `json:"next_cursor"`
	NearestBorehole  *StructureRef     `json:"nearest_borehole,omitempty"`
	CurrentPosition  *BoreholePosition `json:"current_position,omitempty"`
	BasisNeedsReview bool              `json:"basis_needs_review"`
	PositionState    string            `json:"position_state,omitempty"`
	TypeDefinitionID string            `json:"type_definition_id,omitempty"`
	TypeVersionID    string            `json:"type_version_id"`
}

func visibleStructureTarget(ctx context.Context, tx pgx.Tx, auth authorization, facility string, ref StructureRef) bool {
	if auth.Entry != "project" {
		return true
	}
	_, _, e := projectGrantTx(ctx, tx, auth, scoped(auth.Scope, Target{Kind: ref.Kind, ID: ref.ID, Facility: facility}), true)
	return e == nil && auth.can(ref.Kind, "detail")
}
func (s *Service) StructureRecords(ctx context.Context, a access.Actor, scope Scope, target Target, kind, cursor string, limit int) (StructureRecordPage, error) {
	out := StructureRecordPage{Items: []json.RawMessage{}}
	t := scoped(scope, target)
	if e := checkTarget(t); e != nil {
		return out, e
	}
	if t.Kind == Channel {
		return out, notFound()
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 || cursor != "" && !access.ValidID(cursor) {
		return out, problem("invalid_request", "分页参数无效。", 400)
	}
	if kind != "positions" && kind != "references" && kind != "legacy" && kind != "changes" && kind != "attachments" {
		return out, invalid("/kind", "record_kind", "请选择定位、实体参照、旧断面核对或结构变更记录。")
	}
	e := s.transact(ctx, a, scope.operation(string(t.Kind), "structure-details"), scope, false, func(tx pgx.Tx, auth authorization) error {
		f, e := lockFacility(ctx, tx, t, false, false)
		if e != nil {
			return e
		}
		if _, e = load(ctx, tx, t, false); e != nil {
			return e
		}
		if auth.Entry == "project" {
			if _, _, e = projectGrantTx(ctx, tx, auth, t, true); e != nil {
				return e
			}
		}
		out.TypeVersionID = f.TypeVersionID
		if e = tx.QueryRow(ctx, `SELECT definition_id::text FROM engineering_versions WHERE id=$1`, f.TypeVersionID).Scan(&out.TypeDefinitionID); e != nil {
			return e
		}
		if t.Kind == Point {
			var hole *string
			if e = tx.QueryRow(ctx, `SELECT engineering_nearest_structure($1,$2,'point',$3,'borehole')::text`, t.Tenant, t.Facility, t.ID).Scan(&hole); e != nil {
				return e
			}
			if hole != nil {
				out.NearestBorehole = &StructureRef{Borehole, *hole}
			}
			out.PositionState = "unassigned"
			if hole != nil {
				out.PositionState = "not_recorded"
			}
			var b []byte
			err := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM point_borehole_positions p WHERE tenant_id=$1 AND point_id=$2 AND ended_at IS NULL`, t.Tenant, t.ID).Scan(&b)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil {
				var p BoreholePosition
				if e = json.Unmarshal(b, &p); e != nil {
					return e
				}
				out.CurrentPosition = &p
				out.PositionState = "provided"
				// Review is based on the actual measurement basis, not unrelated name edits.
				if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_resource_changes c WHERE c.tenant_id=$1 AND c.resource_kind='borehole' AND c.resource_id=$2 AND c.resource_revision>$3 AND c.before->'borehole' IS DISTINCT FROM c.after->'borehole')`, t.Tenant, p.BoreholeID, p.BoreholeRevision).Scan(&out.BasisNeedsReview); e != nil {
					return e
				}
			}
			if out.BasisNeedsReview {
				out.PositionState = "basis_needs_review"
			}
			if hole != nil && !visibleStructureTarget(ctx, tx, auth, t.Facility, StructureRef{Borehole, *hole}) {
				out.NearestBorehole = nil
				out.CurrentPosition = nil
				out.BasisNeedsReview = false
				out.PositionState = "restricted"
			}
		}
		history := auth.Entry != "project" || auth.can(Facility, "changes")
		var query string
		args := []any{t.Tenant, t.Facility, t.Kind, t.ID, cursor, limit + 1, history, auth.Project}
		visible := `($8='' OR EXISTS(SELECT 1 FROM project_resource_visibility($1,NULLIF($8,'')::uuid,$2) v WHERE v.kind=r.target_kind AND v.id=r.target_id AND v.full_access))`
		switch kind {
		case "attachments":
			query = `SELECT to_jsonb(c),c.id::text FROM engineering_resource_attachments c WHERE tenant_id=$1 AND facility_id=$2 AND resource_kind=$3 AND resource_id=$4 AND ($7 OR ended_at IS NULL) AND ($5='' OR id>NULLIF($5,'')::uuid) AND (($8='' AND scope_kind='tenant') OR project_id=NULLIF($8,'')::uuid) ORDER BY id LIMIT $6`
		case "positions":
			query = `SELECT to_jsonb(p),p.id::text FROM point_borehole_positions p WHERE tenant_id=$1 AND facility_id=$2 AND $3='point' AND point_id=$4 AND ($7 OR ended_at IS NULL) AND ($5='' OR id>NULLIF($5,'')::uuid) AND ($8='' OR EXISTS(SELECT 1 FROM project_resource_visibility($1,NULLIF($8,'')::uuid,$2) v WHERE v.kind='borehole' AND v.id=p.borehole_id AND v.full_access)) ORDER BY id LIMIT $6`
		case "references":
			query = `SELECT to_jsonb(r),r.id::text FROM engineering_resource_references r WHERE tenant_id=$1 AND facility_id=$2 AND source_kind=$3 AND source_id=$4 AND ($7 OR ended_at IS NULL) AND ($5='' OR id>NULLIF($5,'')::uuid) AND ` + visible + ` ORDER BY id LIMIT $6`
		case "legacy":
			query = `SELECT to_jsonb(e)||jsonb_build_object('status',COALESCE(r.status,CASE WHEN e.historical THEN 'historical' ELSE 'pending' END),'target_kind',r.target_kind,'target_id',r.target_id,'resolution_reason',r.reason),e.id::text FROM engineering_legacy_section_evidence e LEFT JOIN LATERAL(SELECT * FROM engineering_legacy_section_resolutions r WHERE r.evidence_id=e.id ORDER BY created_at DESC,id DESC LIMIT 1)r ON true WHERE e.tenant_id=$1 AND e.facility_id=$2 AND e.resource_kind=$3 AND e.resource_id=$4 AND ($7 OR NOT e.historical) AND ($5='' OR e.id>NULLIF($5,'')::uuid) AND (r.target_id IS NULL OR ` + visible + `) ORDER BY e.id LIMIT $6`
		case "changes":
			query = `SELECT to_jsonb(c),c.id::text FROM engineering_structure_record_changes c WHERE tenant_id=$1 AND facility_id=$2 AND resource_kind=$3 AND resource_id=$4 AND $7 AND ($5='' OR id>NULLIF($5,'')::uuid) AND ($8='' OR actor_project_id=NULLIF($8,'')::uuid) ORDER BY id LIMIT $6`
		}
		rows, e := tx.Query(ctx, query, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		ids := []string{}
		for rows.Next() {
			var b json.RawMessage
			var id string
			if e = rows.Scan(&b, &id); e != nil {
				return e
			}
			out.Items = append(out.Items, b)
			ids = append(ids, id)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = ids[limit-1]
		}
		rows.Close()
		if kind == "attachments" {
			out.Items, e = s.filterAttachments(ctx, tx, a, auth, t, out.Items)
			if e != nil {
				return e
			}
		}
		if auth.Entry == "project" && kind == "changes" {
			for i, raw := range out.Items {
				var entry map[string]any
				if e = json.Unmarshal(raw, &entry); e != nil {
					return e
				}
				visible := structureEvidenceVisible(ctx, tx, auth, t.Facility, entry["evidence"])
				if evidence, ok := entry["evidence"].(map[string]any); ok {
					if attachment, ok := evidence["attachment"].(map[string]any); ok {
						id, _ := attachment["attachment_id"].(string)
						revision, _ := attachment["attachment_revision_id"].(string)
						if _, err := s.resolveAttachment(ctx, tx, a, scope, &AttachmentRef{id, revision}); err != nil {
							visible = false
						}
					}
				}
				if !visible {
					delete(entry, "evidence")
					entry["evidence_restricted"] = true
					out.Items[i], e = json.Marshal(entry)
					if e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
	return out, e
}
func validateBoreholePosition(p *BoreholePosition, h Resource) error {
	if p == nil || p.BoreholeID != h.ID || p.BoreholeRevision != h.Revision {
		return problem("borehole_basis_changed", "请重新读取最近孔位资料与修订。", 409)
	}
	if p.ID != "" || p.Revision != 0 || p.EndedAt != nil {
		return invalid("/position", "server_fields", "定位身份和历史字段由服务端生成。")
	}
	if p.Unit != "m" {
		return invalid("/position/unit", "unit", "孔内长度以 m 记录。")
	}
	for _, n := range []*float64{p.DistanceM, p.VerticalDepthM} {
		if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0 || *n > 9007199254740991) {
			return invalid("/position", "finite_number", "距离或埋深须为有限非负数，未知时留空。")
		}
	}
	if p.DistanceM != nil && (strings.TrimSpace(p.ReferenceStart) == "" || strings.TrimSpace(p.Direction) == "") {
		return invalid("/position", "reference_required", "沿孔距离需要明确起点和方向。")
	}
	if p.VerticalDepthM != nil && strings.TrimSpace(p.VerticalDatum) == "" {
		return invalid("/position/vertical_datum", "required", "垂直埋深需要独立参照基准。")
	}
	for _, v := range []string{p.ReferenceStart, p.Direction, p.VerticalDatum, p.Source, p.ConsistencyReason} {
		if len([]rune(v)) > 2000 {
			return invalid("/position", "input_limit", "说明最多 2000 字符。")
		}
	}
	if p.MeasuredAt != nil {
		if _, e := time.Parse("2006-01-02", *p.MeasuredAt); e != nil {
			return invalid("/position/measured_at", "date", "请填写有效测量日期。")
		}
	}
	if h.Borehole != nil && h.Borehole.MeasuredDepthM != nil && p.DistanceM != nil && *p.DistanceM > *h.Borehole.MeasuredDepthM && strings.TrimSpace(p.ConsistencyReason) == "" {
		return invalid("/position/consistency_reason", "depth_inconsistency", "沿孔距离超过实测孔深；请核对并记录采用该资料的原因。")
	}
	return nil
}
func (s *Service) PreviewStructureRecord(ctx context.Context, a access.Actor, scope Scope, target Target, in StructureRecordInput) (StructureRecordPreview, error) {
	_, p, e := s.writeStructureRecord(ctx, a, scope, target, in, access.Meta{}, true)
	return p, e
}
func (s *Service) WriteStructureRecord(ctx context.Context, a access.Actor, scope Scope, target Target, in StructureRecordInput, meta access.Meta) (Result, error) {
	r, _, e := s.writeStructureRecord(ctx, a, scope, target, in, meta, false)
	return r, e
}
func (s *Service) writeStructureRecord(ctx context.Context, a access.Actor, scope Scope, target Target, in StructureRecordInput, meta access.Meta, dry bool) (Result, StructureRecordPreview, error) {
	out := Result{}
	preview := StructureRecordPreview{Warnings: []string{}}
	t := scoped(scope, target)
	if e := checkTarget(t); e != nil {
		return out, preview, e
	}
	if t.Kind == Channel {
		return out, preview, notFound()
	}
	if e := validStructureMeta(in.WriteMeta); e != nil {
		return out, preview, e
	}
	fields := map[string]any{"action": in.Action, "position": in.Position, "target": in.Target, "record_id": in.RecordID, "purpose": in.Purpose, "disposition": in.Disposition, "attachment": in.Attachment}
	e := s.transact(ctx, a, scope.operation(string(t.Kind), "structure-records"), scope, false, func(tx pgx.Tx, auth authorization) error {
		if !dry {
			var replay bool
			var e error
			out, replay, e = mutationReceipt(ctx, tx, auth, t, "structure-records", in.WriteMeta, fields)
			if e != nil {
				return e
			}
			if replay {
				if in.Action == "end_reference" {
					var ref StructureRef
					if e = tx.QueryRow(ctx, `SELECT target_kind,target_id::text FROM engineering_resource_references WHERE tenant_id=$1 AND facility_id=$2 AND source_kind=$3 AND source_id=$4 AND id=$5`, t.Tenant, t.Facility, t.Kind, t.ID, in.RecordID).Scan(&ref.Kind, &ref.ID); e != nil {
						return notFound()
					}
					if !visibleStructureTarget(ctx, tx, auth, t.Facility, ref) {
						return access.ErrDenied
					}
				}
				if in.Target != nil && !visibleStructureTarget(ctx, tx, auth, t.Facility, *in.Target) {
					return access.ErrDenied
				}
				if in.Position != nil && !visibleStructureTarget(ctx, tx, auth, t.Facility, StructureRef{Borehole, in.Position.BoreholeID}) {
					return access.ErrDenied
				}
				if in.Action == "attachment" {
					_, e = s.resolveAttachment(ctx, tx, a, scope, in.Attachment)
				}
				if in.Action == "end_attachment" {
					_, e = s.attachmentRecordTx(ctx, tx, a, auth, t, in.RecordID)
				}
				return e
			}
		}
		f, e := lockFacility(ctx, tx, t, true, false)
		if e != nil {
			return e
		}
		r, e := load(ctx, tx, t, false)
		if e != nil {
			return e
		}
		if r.Revision != in.Revision || f.StructureRevision != in.StructureRevision {
			return problem("resource_revision_conflict", "对象或工程结构已变化，请重新预览。", 409)
		}
		ancestors, e := ancestorsTx(ctx, tx, t, r, f)
		if e != nil {
			return e
		}
		if e = writable(r, ancestors, "update"); e != nil {
			return e
		}
		link, e := validateProjectWrite(ctx, tx, auth, t, "update", in.WriteMeta, r, &r)
		if e != nil {
			return e
		}
		var nearest *string
		if t.Kind == Point {
			if e = tx.QueryRow(ctx, `SELECT engineering_nearest_structure($1,$2,'point',$3,'borehole')::text`, t.Tenant, t.Facility, t.ID).Scan(&nearest); e != nil {
				return e
			}
		}
		if in.Target != nil {
			if !isStructureContainer(in.Target.Kind) && in.Target.Kind != Point || !access.ValidID(in.Target.ID) || in.Target.Kind == t.Kind && in.Target.ID == t.ID {
				return invalid("/target", "reference_target", "请选择同设施内另一正式实体。")
			}
			other, e := load(ctx, tx, scoped(scope, Target{Kind: in.Target.Kind, ID: in.Target.ID, Facility: t.Facility}), false)
			if e != nil {
				return e
			}
			if other.Status != "normal" {
				return problem("resource_retired", "参照实体已退役。", 409)
			}
			if !visibleStructureTarget(ctx, tx, auth, t.Facility, *in.Target) {
				return access.ErrDenied
			}
		}
		switch in.Action {
		case "attachment":
			if strings.TrimSpace(in.Purpose) == "" || len([]rune(in.Purpose)) > 2000 {
				return invalid("/purpose", "required", "Please describe the engineering evidence.")
			}
			if _, e = s.resolveAttachment(ctx, tx, a, scope, in.Attachment); e != nil {
				return e
			}
		case "end_attachment":
			if !access.ValidID(in.RecordID) {
				return notFound()
			}
			if _, e = s.attachmentRecordTx(ctx, tx, a, auth, t, in.RecordID); e != nil {
				return e
			}
		case "position":
			if t.Kind != Point || nearest == nil || in.Position == nil || *nearest != in.Position.BoreholeID {
				return invalid("/position/borehole_id", "nearest_borehole", "孔内定位必须引用测点的当前最近孔位。")
			}
			if !visibleStructureTarget(ctx, tx, auth, t.Facility, StructureRef{Borehole, *nearest}) {
				return access.ErrDenied
			}
			h, e := load(ctx, tx, scoped(scope, Target{Kind: Borehole, ID: *nearest, Facility: t.Facility}), false)
			if e != nil {
				return e
			}
			if e = validateBoreholePosition(in.Position, h); e != nil {
				return e
			}
			if h.Borehole != nil && h.Borehole.MeasuredDepthM != nil && in.Position.DistanceM != nil && *in.Position.DistanceM > *h.Borehole.MeasuredDepthM {
				preview.Warnings = append(preview.Warnings, "沿孔距离超过实测孔深；本次记录保留已填写的依据异常说明。")
			}
		case "reference":
			if in.Target == nil || strings.TrimSpace(in.Purpose) == "" || len([]rune(in.Purpose)) > 2000 {
				return invalid("/purpose", "required", "请选择实体并填写参照用途，最多 2000 字符。")
			}
		case "end_reference":
			if !access.ValidID(in.RecordID) {
				return notFound()
			}
			var ref StructureRef
			if e = tx.QueryRow(ctx, `SELECT target_kind,target_id::text FROM engineering_resource_references WHERE tenant_id=$1 AND facility_id=$2 AND source_kind=$3 AND source_id=$4 AND id=$5 AND ended_at IS NULL`, t.Tenant, t.Facility, t.Kind, t.ID, in.RecordID).Scan(&ref.Kind, &ref.ID); e == pgx.ErrNoRows {
				return notFound()
			} else if e != nil {
				return e
			}
			if !visibleStructureTarget(ctx, tx, auth, t.Facility, ref) {
				return access.ErrDenied
			}
		case "legacy":
			if !access.ValidID(in.RecordID) {
				return notFound()
			}
			var exists bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_legacy_section_evidence WHERE tenant_id=$1 AND facility_id=$2 AND resource_kind=$3 AND resource_id=$4 AND id=$5)`, t.Tenant, t.Facility, t.Kind, t.ID, in.RecordID).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				return notFound()
			}
			if in.Disposition != "pending" && in.Disposition != "historical" && in.Disposition != "mapped" {
				return invalid("/disposition", "resolution", "请选择待核对、仅作历史依据或映射正式断面。")
			}
			if in.Disposition == "mapped" && (in.Target == nil || in.Target.Kind != MonitoringSection) || in.Disposition != "mapped" && in.Target != nil {
				return invalid("/target", "resolution_target", "只有实体映射可指定正式监测断面。")
			}
		default:
			return invalid("/action", "record_action", "不支持的结构记录操作。")
		}
		impact, e := s.mutationImpact(ctx, tx, auth, t, "update", in.WriteMeta, fields, &r, r)
		if e != nil {
			return e
		}
		preview.Shared = impact.Shared
		var state string
		if e = tx.QueryRow(ctx, `SELECT md5(jsonb_build_array((SELECT COALESCE(jsonb_agg(to_jsonb(p) ORDER BY id),'[]') FROM point_borehole_positions p WHERE tenant_id=$1 AND facility_id=$2),(SELECT COALESCE(jsonb_agg(to_jsonb(p) ORDER BY id),'[]') FROM engineering_resource_references p WHERE tenant_id=$1 AND facility_id=$2),(SELECT COALESCE(jsonb_agg(to_jsonb(p) ORDER BY p.id),'[]') FROM engineering_legacy_section_resolutions p JOIN engineering_legacy_section_evidence e ON e.id=p.evidence_id WHERE e.tenant_id=$1 AND e.facility_id=$2))::text)`, t.Tenant, t.Facility).Scan(&state); e != nil {
			return e
		}
		var attachmentState string
		if e = tx.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY r.id),'[]'::jsonb)::text) FROM engineering_resource_attachments r WHERE tenant_id=$1 AND facility_id=$2`, t.Tenant, t.Facility).Scan(&attachmentState); e != nil {
			return e
		}
		preview.Hash = fingerprint([]any{impact.Hash, fields, state, nearest, attachmentState})
		if dry {
			return nil
		}
		if in.PreviewHash == "" || in.PreviewHash != preview.Hash {
			return previewChanged()
		}
		switch in.Action {
		case "attachment", "end_attachment":
			e = s.recordAttachment(ctx, tx, a, scope, t, in)
		case "position":
			if _, e = tx.Exec(ctx, `UPDATE point_borehole_positions SET ended_at=clock_timestamp(),ended_reason=$3 WHERE tenant_id=$1 AND point_id=$2 AND ended_at IS NULL`, t.Tenant, t.ID, in.Reason); e != nil {
				return e
			}
			p := in.Position
			_, e = tx.Exec(ctx, `INSERT INTO point_borehole_positions(tenant_id,facility_id,point_id,borehole_id,revision,distance_m,reference_start,direction,unit,vertical_depth_m,vertical_datum,source,measured_at,borehole_revision,consistency_reason,created_by,reason) VALUES($1,$2,$3,$4,(SELECT COALESCE(max(revision),0)+1 FROM point_borehole_positions WHERE tenant_id=$1 AND point_id=$3),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, t.Tenant, t.Facility, t.ID, p.BoreholeID, p.DistanceM, p.ReferenceStart, p.Direction, p.Unit, p.VerticalDepthM, p.VerticalDatum, p.Source, p.MeasuredAt, p.BoreholeRevision, p.ConsistencyReason, a.ID, in.Reason)
		case "reference":
			_, e = tx.Exec(ctx, `INSERT INTO engineering_resource_references(tenant_id,facility_id,source_kind,source_id,target_kind,target_id,purpose,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, t.Tenant, t.Facility, t.Kind, t.ID, in.Target.Kind, in.Target.ID, in.Purpose, a.ID)
		case "end_reference":
			_, e = tx.Exec(ctx, `UPDATE engineering_resource_references SET ended_at=clock_timestamp(),ended_reason=$2 WHERE id=$1`, in.RecordID, in.Reason)
		case "legacy":
			var k, id *string
			if in.Target != nil {
				v := string(in.Target.Kind)
				k = &v
				id = &in.Target.ID
			}
			_, e = tx.Exec(ctx, `INSERT INTO engineering_legacy_section_resolutions(evidence_id,status,target_kind,target_id,reason,actor_id,request_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, in.RecordID, in.Disposition, k, id, in.Reason, a.ID, in.RequestID)
			if e == nil && in.Target != nil {
				_, e = tx.Exec(ctx, `INSERT INTO engineering_resource_references(tenant_id,facility_id,source_kind,source_id,target_kind,target_id,purpose,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, t.Tenant, t.Facility, t.Kind, t.ID, in.Target.Kind, in.Target.ID, "旧断面文字核对："+in.Reason, a.ID)
			}
		}
		if e != nil {
			return e
		}
		before := r
		r.Revision++
		r.UpdatedBy = a.ID
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.UpdatedAt); e != nil {
			return e
		}
		if e = storeResource(ctx, tx, r, false); e != nil {
			return e
		}
		out = Result{ID: r.ID, FacilityID: t.Facility, Revision: r.Revision, StructureRevision: f.StructureRevision, Changed: true, ProjectID: auth.Project, AccessRevision: auth.AccessRevision}
		if link != nil {
			out.LinkID = link.ID
			out.LinkRevision = link.Revision
		}
		if e = saveStructureEvidence(ctx, tx, auth, t, r.Revision, in.Action, in.WriteMeta, fields); e != nil {
			return e
		}
		if e = recordChange(ctx, tx, a, auth, t, "structure-records", in.WriteMeta, &before, r, t.Facility, out.LinkID, meta); e != nil {
			return e
		}
		return saveMutationReceipt(ctx, tx, auth, t, "structure-records", in.WriteMeta, fields, out)
	})
	return out, preview, e
}
func saveStructureEvidence(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, revision int, action string, m WriteMeta, evidence any) error {
	_, e := tx.Exec(ctx, `INSERT INTO engineering_structure_record_changes(tenant_id,facility_id,resource_kind,resource_id,resource_revision,action,evidence,reason,request_id,actor_id,actor_project_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid)`, t.Tenant, t.Facility, t.Kind, t.ID, revision, action, evidence, m.Reason, m.RequestID, auth.Account, auth.Project)
	return e
}

// Historical evidence cannot retain access to identities whose current grant was revoked.
func structureEvidenceVisible(ctx context.Context, tx pgx.Tx, auth authorization, facility string, value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if k, ok := v["kind"].(string); ok {
			if id, ok := v["id"].(string); ok && !visibleStructureTarget(ctx, tx, auth, facility, StructureRef{Kind(k), id}) {
				return false
			}
		}
		if id, ok := v["borehole_id"].(string); ok && id != "" && !visibleStructureTarget(ctx, tx, auth, facility, StructureRef{Borehole, id}) {
			return false
		}
		for _, child := range v {
			if !structureEvidenceVisible(ctx, tx, auth, facility, child) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !structureEvidenceVisible(ctx, tx, auth, facility, child) {
				return false
			}
		}
	}
	return true
}
