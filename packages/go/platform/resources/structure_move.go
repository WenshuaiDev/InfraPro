package resources

import (
	"context"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

type MoveInput struct {
	WriteMeta
	ParentRef           StructureRef `json:"parent_ref"`
	PositionDisposition string       `json:"position_disposition"`
}
type StructureMembershipChange struct {
	Resource        StructureRef  `json:"resource"`
	BeforeComponent *StructureRef `json:"before_component"`
	AfterComponent  *StructureRef `json:"after_component"`
	BeforeSection   *StructureRef `json:"before_section"`
	AfterSection    *StructureRef `json:"after_section"`
	BeforeBorehole  *StructureRef `json:"before_borehole"`
	AfterBorehole   *StructureRef `json:"after_borehole"`
}
type MovePreview struct {
	Hash                     string                      `json:"preview_hash"`
	StructureRevision        int                         `json:"structure_revision"`
	BeforeParent             *StructureRef               `json:"before_parent"`
	AfterParent              StructureRef                `json:"after_parent"`
	Affected                 []StructureMembershipChange `json:"affected"`
	PositionsRequiringReview []StructureRef              `json:"positions_requiring_review"`
	Shared                   bool                        `json:"shared"`
	SpatialReviewRequired    bool                        `json:"spatial_review_required"`
	PreservesIdentities      bool                        `json:"preserves_identities"`
}

func (s *Service) PreviewMove(ctx context.Context, a access.Actor, scope Scope, target Target, in MoveInput) (MovePreview, error) {
	_, p, e := s.moveStructure(ctx, a, scope, target, in, access.Meta{}, true)
	return p, e
}
func (s *Service) Move(ctx context.Context, a access.Actor, scope Scope, target Target, in MoveInput, meta access.Meta) (Result, error) {
	r, _, e := s.moveStructure(ctx, a, scope, target, in, meta, false)
	return r, e
}
func validStructureMeta(m WriteMeta) error {
	if !access.ValidID(m.RequestID) || m.Revision < 1 || m.StructureRevision < 1 {
		return problem("invalid_request", "需要请求编号、对象修订和设施结构修订。", 400)
	}
	if strings.TrimSpace(m.Reason) == "" || len([]rune(m.Reason)) > 2000 {
		return invalid("/reason", "required", "请填写变更原因，最多 2000 字符。")
	}
	return nil
}
func (s *Service) moveStructure(ctx context.Context, a access.Actor, scope Scope, target Target, in MoveInput, meta access.Meta, dry bool) (Result, MovePreview, error) {
	out := Result{}
	preview := MovePreview{Affected: []StructureMembershipChange{}, PositionsRequiringReview: []StructureRef{}, PreservesIdentities: true, SpatialReviewRequired: true}
	t := scoped(scope, target)
	if e := checkTarget(t); e != nil {
		return out, preview, e
	}
	if target.Kind == Facility || target.Kind == Channel {
		return out, preview, invalid("/kind", "not_movable", "设施和通道不通过结构移动修改归属。")
	}
	if e := validStructureMeta(in.WriteMeta); e != nil {
		return out, preview, e
	}
	if !isStructureContainer(in.ParentRef.Kind) || !access.ValidID(in.ParentRef.ID) {
		return out, preview, invalid("/parent_ref", "parent_reference", "请选择具有种类和身份的结构容器。")
	}
	if in.PositionDisposition != "" && in.PositionDisposition != "history_pending" {
		return out, preview, invalid("/position_disposition", "disposition", "请选择保留历史并待补定位；重新定位在移动后明确录入。")
	}
	fields := map[string]any{"parent_ref": in.ParentRef, "position_disposition": in.PositionDisposition}
	e := s.transact(ctx, a, scope.operation(string(target.Kind), "move"), scope, false, func(tx pgx.Tx, auth authorization) error {
		if !dry {
			var replay bool
			var e error
			out, replay, e = mutationReceipt(ctx, tx, auth, t, "move", in.WriteMeta, fields)
			if e != nil || replay {
				return e
			}
		}
		facility, e := lockFacility(ctx, tx, t, true, false)
		if e != nil {
			return e
		}
		before, e := load(ctx, tx, t, false)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return problem("resource_revision_conflict", "对象已变化，请重新预览。", 409)
		}
		if facility.StructureRevision != in.StructureRevision {
			return problem("structure_revision_conflict", "工程树已变化，请重新预览。", 409)
		}
		graph, e := loadStructureGraph(ctx, tx, t.Tenant, t.Facility, false)
		if e != nil {
			return e
		}
		parent, ok := graph[refKey(in.ParentRef.Kind, in.ParentRef.ID)]
		if !ok {
			return notFound()
		}
		if parent.Status != "normal" || before.Status != "normal" {
			return problem("ancestor_retired", "移动对象和父级须处于正常状态。", 409)
		}
		ancestors, e := graph.ancestors(parent)
		if e != nil {
			return e
		}
		for _, v := range ancestors {
			if v.Status != "normal" {
				return problem("ancestor_retired", "目标父链包含退役对象。", 409)
			}
		}
		oldAncestors, e := graph.ancestors(before)
		if e != nil {
			return e
		}
		if e = writable(before, oldAncestors, "update"); e != nil {
			return e
		}
		affected := graph.subtree(resourceRef(before))
		link, e := authorizeStructureMove(ctx, tx, auth, t, in, affected, before)
		if e != nil {
			return e
		}
		after := before
		after.ParentRef = &in.ParentRef
		moved := structureGraph{}
		for k, v := range graph {
			moved[k] = v
		}
		moved[refKey(after.Kind, after.ID)] = after
		typ, _, e := engineering.ResolvePublicTx(ctx, tx, facility.TypeVersionID, engineering.TypeDefinition, false)
		if e != nil {
			return e
		}
		if e = moved.validate(typ.Content); e != nil {
			return e
		}
		for _, r := range affected {
			next := moved[refKey(r.Kind, r.ID)]
			change := StructureMembershipChange{Resource: resourceRef(r), BeforeComponent: graph.nearest(r, Component, false), AfterComponent: moved.nearest(next, Component, false), BeforeSection: graph.nearest(r, MonitoringSection, r.Kind == Channel), AfterSection: moved.nearest(next, MonitoringSection, r.Kind == Channel), BeforeBorehole: graph.nearest(r, Borehole, r.Kind == Channel), AfterBorehole: moved.nearest(next, Borehole, r.Kind == Channel)}
			preview.Affected = append(preview.Affected, change)
			if r.Kind == Point && fingerprint(change.BeforeBorehole) != fingerprint(change.AfterBorehole) {
				var exists bool
				if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_borehole_positions WHERE tenant_id=$1 AND point_id=$2 AND ended_at IS NULL)`, t.Tenant, r.ID).Scan(&exists); e != nil {
					return e
				}
				if exists {
					preview.PositionsRequiringReview = append(preview.PositionsRequiringReview, resourceRef(r))
				}
			}
		}
		preview.StructureRevision = facility.StructureRevision
		preview.BeforeParent = before.ParentRef
		preview.AfterParent = in.ParentRef
		impact, e := s.mutationImpact(ctx, tx, auth, t, "move", in.WriteMeta, fields, &before, after)
		if e != nil {
			return e
		}
		preview.Shared = impact.Shared
		var positions string
		if e = tx.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(p) ORDER BY p.id),'[]'::jsonb)::text) FROM point_borehole_positions p WHERE tenant_id=$1 AND facility_id=$2`, t.Tenant, t.Facility).Scan(&positions); e != nil {
			return e
		}
		preview.Hash = fingerprint([]any{impact.Hash, preview.Affected, facility.StructureRevision, positions, typ.ID})
		if dry {
			return nil
		}
		if in.PreviewHash == "" || in.PreviewHash != preview.Hash {
			return previewChanged()
		}
		if len(preview.PositionsRequiringReview) > 0 && in.PositionDisposition != "history_pending" {
			return problem("position_disposition_required", "最近孔位变化，需要明确将旧定位保留为历史并待补。", 409)
		}
		if fingerprint(before.ParentRef) == fingerprint(after.ParentRef) {
			out = Result{ID: before.ID, FacilityID: t.Facility, Revision: before.Revision, StructureRevision: facility.StructureRevision, ProjectID: auth.Project, AccessRevision: auth.AccessRevision}
			if link != nil {
				out.LinkID = link.ID
				out.LinkRevision = link.Revision
			}
			return saveMutationReceipt(ctx, tx, auth, t, "move", in.WriteMeta, fields, out)
		}
		for _, p := range preview.PositionsRequiringReview {
			if _, e = tx.Exec(ctx, `UPDATE point_borehole_positions SET ended_at=clock_timestamp(),ended_reason=$3 WHERE tenant_id=$1 AND point_id=$2 AND ended_at IS NULL`, t.Tenant, p.ID, in.Reason); e != nil {
				return e
			}
		}
		if e = storeStructureParent(ctx, tx, after); e != nil {
			return e
		}
		after.Revision++
		after.UpdatedBy = a.ID
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&after.UpdatedAt); e != nil {
			return e
		}
		if e = prepareStructureWrite(ctx, tx, t, &after, &before, map[string]any{}, facility); e != nil {
			return e
		}
		if e = storeResource(ctx, tx, after, false); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `UPDATE engineering_facilities SET structure_revision=structure_revision+1 WHERE tenant_id=$1 AND id=$2 RETURNING structure_revision`, t.Tenant, t.Facility).Scan(&facility.StructureRevision); e != nil {
			return e
		}
		out = Result{ID: after.ID, FacilityID: t.Facility, Revision: after.Revision, StructureRevision: facility.StructureRevision, Changed: true, ProjectID: auth.Project, AccessRevision: auth.AccessRevision}
		if link != nil {
			out.LinkID = link.ID
			out.LinkRevision = link.Revision
		}
		if lifecycle, ok := s.external.(ResourceLifecycle); ok {
			if e = lifecycle.ResourceChangedTx(ctx, tx, a, scope, &before, after, in.RequestID, in.Reason, meta); e != nil {
				return e
			}
		}
		if e = saveStructureEvidence(ctx, tx, auth, t, after.Revision, "move", in.WriteMeta, preview); e != nil {
			return e
		}
		if e = recordChange(ctx, tx, a, auth, t, "move", in.WriteMeta, &before, after, t.Facility, out.LinkID, meta); e != nil {
			return e
		}
		return saveMutationReceipt(ctx, tx, auth, t, "move", in.WriteMeta, fields, out)
	})
	return out, preview, e
}
func authorizeStructureMove(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, in MoveInput, affected []Resource, before Resource) (*Link, error) {
	if auth.Entry != "project" {
		return nil, nil
	}
	if auth.ProjectStatus == "archived" {
		return nil, access.ErrDenied
	}
	if in.AccessRevision != auth.AccessRevision {
		return nil, accessChanged()
	}
	var link *Link
	for _, r := range affected {
		g, l, e := projectGrantTx(ctx, tx, auth, scoped(auth.Scope, Target{Kind: r.Kind, ID: r.ID, Facility: t.Facility}), true)
		if e != nil || !slices.Contains(g.Capabilities, "update") || !auth.can(r.Kind, "update") {
			return nil, access.ErrDenied
		}
		link = l
	}
	if link == nil || link.Revision != in.LinkRevision {
		return nil, accessChanged()
	}
	for _, p := range []*StructureRef{before.ParentRef, &in.ParentRef} {
		if p == nil {
			return nil, notFound()
		}
		g, _, e := projectGrantTx(ctx, tx, auth, scoped(auth.Scope, Target{Kind: p.Kind, ID: p.ID, Facility: t.Facility}), true)
		if e != nil {
			return nil, access.ErrDenied
		}
		cap := "update"
		if p == &in.ParentRef {
			cap = "create_" + string(t.Kind)
		}
		if !slices.Contains(g.Capabilities, cap) || !auth.can(p.Kind, "update") {
			return nil, access.ErrDenied
		}
	}
	return link, nil
}

func authorizeMoveReplay(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, m WriteMeta, current Resource) error {
	graph, e := loadStructureGraph(ctx, tx, t.Tenant, t.Facility, false)
	if e != nil {
		return e
	}
	affected := graph.subtree(resourceRef(current))
	var evidence MovePreview
	e = tx.QueryRow(ctx, `SELECT evidence FROM engineering_structure_record_changes WHERE tenant_id=$1 AND request_id=$2 AND resource_kind=$3 AND resource_id=$4 AND actor_id=$5 AND actor_project_id=$6 AND action='move'`, t.Tenant, m.RequestID, t.Kind, t.ID, auth.Account, auth.Project).Scan(&evidence)
	if e != nil && e != pgx.ErrNoRows {
		return e
	}
	before := current
	parent := current.ParentRef
	if e == nil {
		before.ParentRef = evidence.BeforeParent
		parent = &evidence.AfterParent
		for _, change := range evidence.Affected {
			r, err := load(ctx, tx, scoped(auth.Scope, Target{Kind: change.Resource.Kind, ID: change.Resource.ID, Facility: t.Facility}), false)
			if err != nil {
				return err
			}
			affected = append(affected, r)
		}
	}
	if parent == nil {
		return access.ErrDenied
	}
	_, e = authorizeStructureMove(ctx, tx, auth, t, MoveInput{WriteMeta: m, ParentRef: *parent}, affected, before)
	return e
}
