package resources

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

type TypeAdoptionInput struct {
	WriteMeta
	TypeVersionID string `json:"type_version_id"`
}
type TypeAdoptionPreview struct {
	Hash                         string              `json:"preview_hash"`
	From                         engineering.Version `json:"from"`
	To                           engineering.Version `json:"to"`
	BeforeApplicability          map[string]string   `json:"before_applicability"`
	AfterApplicability           map[string]string   `json:"after_applicability"`
	StructureRevision            int                 `json:"structure_revision"`
	ResourceCount                int                 `json:"resource_count"`
	Shared                       bool                `json:"shared"`
	TemplateRevalidationRequired bool                `json:"template_revalidation_required"`
	PreservesIdentities          bool                `json:"preserves_identities"`
}

func (s *Service) PreviewTypeAdoption(ctx context.Context, a access.Actor, tenant, facility string, in TypeAdoptionInput) (TypeAdoptionPreview, error) {
	_, p, e := s.adoptType(ctx, a, tenant, facility, in, access.Meta{}, true)
	return p, e
}
func (s *Service) AdoptType(ctx context.Context, a access.Actor, tenant, facility string, in TypeAdoptionInput, meta access.Meta) (Result, error) {
	r, _, e := s.adoptType(ctx, a, tenant, facility, in, meta, false)
	return r, e
}
func (s *Service) adoptType(ctx context.Context, a access.Actor, tenant, facility string, in TypeAdoptionInput, meta access.Meta, dry bool) (Result, TypeAdoptionPreview, error) {
	out := Result{}
	preview := TypeAdoptionPreview{BeforeApplicability: map[string]string{}, AfterApplicability: map[string]string{}, PreservesIdentities: true, TemplateRevalidationRequired: true}
	t := scoped(TenantScope(tenant), Target{Kind: Facility, ID: facility, Facility: facility})
	if e := checkTarget(t); e != nil {
		return out, preview, e
	}
	if e := validStructureMeta(in.WriteMeta); e != nil {
		return out, preview, e
	}
	if !access.ValidID(in.TypeVersionID) {
		return out, preview, invalid("/type_version_id", "reference", "请选择固定发布版本。")
	}
	fields := map[string]any{"type_version_id": in.TypeVersionID}
	e := s.transact(ctx, a, t.operation("facility", "adopt-type"), t.Scope, false, func(tx pgx.Tx, auth authorization) error {
		if !dry {
			var replay bool
			var e error
			out, replay, e = mutationReceipt(ctx, tx, auth, t, "adopt-type", in.WriteMeta, fields)
			if e != nil || replay {
				return e
			}
		}
		before, e := lockFacility(ctx, tx, t, true, false)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision || before.StructureRevision != in.StructureRevision {
			return previewChanged()
		}
		if e = writable(before, nil, "update"); e != nil {
			return e
		}
		old, _, e := engineering.ResolvePublicTx(ctx, tx, before.TypeVersionID, engineering.TypeDefinition, false)
		if e != nil {
			return e
		}
		next, _, e := engineering.ResolvePublicTx(ctx, tx, in.TypeVersionID, engineering.TypeDefinition, true)
		if e != nil {
			return e
		}
		if old.DefinitionID != next.DefinitionID {
			return problem("type_conversion_required", "这里只支持同一类型定义内的兼容采用。", 409)
		}
		if old.ID == next.ID {
			return invalid("/type_version_id", "same_version", "当前已采用此版本。")
		}
		graph, e := loadStructureGraph(ctx, tx, tenant, facility, true)
		if e != nil {
			return e
		}
		if e = graph.validate(next.Content); e != nil {
			return e
		}
		preview.From, preview.To = old, next
		preview.StructureRevision = before.StructureRevision
		preview.ResourceCount = len(graph)
		for _, kind := range []string{"monitoring_section", "borehole"} {
			preview.BeforeApplicability[kind] = engineering.StructureApplicability(old.Content, kind)
			preview.AfterApplicability[kind] = engineering.StructureApplicability(next.Content, kind)
		}
		after := before
		after.TypeVersionID = next.ID
		impact, e := s.mutationImpact(ctx, tx, auth, t, "adopt-type", in.WriteMeta, fields, &before, after)
		if e != nil {
			return e
		}
		preview.Shared = impact.Shared
		preview.Hash = fingerprint([]any{impact.Hash, graph, old.ID, next.ID, next.StateRevision, before.StructureRevision})
		if dry {
			return nil
		}
		if in.PreviewHash == "" || in.PreviewHash != preview.Hash {
			return previewChanged()
		}
		after.Revision++
		after.StructureRevision++
		after.UpdatedBy = a.ID
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&after.UpdatedAt); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO facility_type_adoptions(tenant_id,facility_id,from_version_id,to_version_id,resource_revision,structure_revision,actor_id,request_id,reason,preview_hash,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, tenant, facility, old.ID, next.ID, after.Revision, after.StructureRevision, a.ID, in.RequestID, in.Reason, preview.Hash, preview); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE engineering_facilities SET type_version_id=$3,revision=$4,structure_revision=$5,updated_at=$6,updated_by=$7 WHERE tenant_id=$1 AND id=$2`, tenant, facility, next.ID, after.Revision, after.StructureRevision, after.UpdatedAt, a.ID); e != nil {
			return e
		}
		if e = recordChange(ctx, tx, a, auth, t, "adopt-type", in.WriteMeta, &before, after, facility, "", meta); e != nil {
			return e
		}
		if lifecycle, ok := s.external.(ResourceLifecycle); ok {
			if e = lifecycle.ResourceChangedTx(ctx, tx, a, t.Scope, &before, after, in.RequestID, in.Reason, meta); e != nil {
				return e
			}
		}
		out = Result{ID: facility, FacilityID: facility, Revision: after.Revision, StructureRevision: after.StructureRevision, Changed: true}
		return saveMutationReceipt(ctx, tx, auth, t, "adopt-type", in.WriteMeta, fields, out)
	})
	return out, preview, e
}
