package resources

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func resourceFingerprint(t scopedTarget, action string, m WriteMeta, in map[string]any, auth authorization, version int) string {
	if version == 1 {
		// Preserve the exact pre-refactor Go payload shape and JSON field order.
		return fingerprint(struct {
			Target struct {
				Tenant, Facility, Point, ID string
				Kind                        Kind
			}
			Action string
			Meta   struct {
				RequestID         string `json:"request_id"`
				Revision          int    `json:"expected_revision"`
				StructureRevision int    `json:"expected_structure_revision"`
				Reason            string `json:"reason"`
			}
			Fields map[string]any
		}{struct {
			Tenant, Facility, Point, ID string
			Kind                        Kind
		}{t.Tenant, t.Facility, t.Point, t.ID, t.Kind}, action, struct {
			RequestID         string `json:"request_id"`
			Revision          int    `json:"expected_revision"`
			StructureRevision int    `json:"expected_structure_revision"`
			Reason            string `json:"reason"`
		}{m.RequestID, m.Revision, m.StructureRevision, m.Reason}, in})
	}
	return fingerprint(struct {
		Scope                 Scope
		Target                Target
		Action                string
		Meta                  WriteMeta
		Fields                map[string]any
		Member, ProjectMember string
	}{auth.Scope, t.Target, action, m, in, auth.TenantMembership, auth.ProjectAuth.MembershipID})
}
func projectReceiptReadable(ctx context.Context, tx pgx.Tx, auth authorization, id string, out *Result) error {
	var tenantMember, projectMember, link, kind, target string
	e := tx.QueryRow(ctx, `SELECT tenant_membership_id::text,project_membership_id::text,link_id::text,target_kind,target_id::text FROM project_resource_operation_receipts WHERE entry_scope='project' AND tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4`, auth.Tenant, auth.Project, auth.Account, id).Scan(&tenantMember, &projectMember, &link, &kind, &target)
	if e == pgx.ErrNoRows {
		return notFound()
	}
	if e != nil {
		return e
	}
	if tenantMember != auth.TenantMembership || projectMember != auth.ProjectAuth.MembershipID {
		return notFound()
	}
	t := scoped(auth.Scope, Target{Kind: Kind(kind), Facility: out.FacilityID, ID: target})
	_, current, e := projectGrantTx(ctx, tx, auth, t, true)
	if e != nil {
		return e
	}
	if current.ID != link {
		return notFound()
	}
	return nil
}
func mutationReceipt(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, action string, m WriteMeta, in map[string]any) (Result, bool, error) {
	out := Result{}
	var oldHash, oldOp string
	version := 2
	var e error
	if auth.Entry == "tenant" {
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result,fingerprint_version FROM engineering_resource_receipts WHERE tenant_id=$1 AND actor_id=$2 AND request_id=$3`, auth.Tenant, auth.Account, m.RequestID).Scan(&oldOp, &oldHash, &out, &version)
	} else {
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM project_resource_operation_receipts WHERE entry_scope='project' AND tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4`, auth.Tenant, auth.Project, auth.Account, m.RequestID).Scan(&oldOp, &oldHash, &out)
	}
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if auth.Entry == "project" {
		if e = projectReceiptReadable(ctx, tx, auth, m.RequestID, &out); e != nil {
			return Result{}, false, e
		}
		// A receipt lookup is read-only; a submitted replay still needs today's
		// action capability. Ignore obsolete optimistic revisions, not authority.
		check := scoped(auth.Scope, Target{Kind: t.Kind, Facility: out.FacilityID, Point: t.Point, ID: out.ID})
		r, loadErr := load(ctx, tx, check, false)
		if loadErr != nil {
			return Result{}, false, loadErr
		}
		if action == "create" {
			r, loadErr = apply(Resource{Kind: t.Kind, FacilityID: out.FacilityID, PointID: t.Point}, in)
			if loadErr != nil {
				return Result{}, false, loadErr
			}
		}
		link, linkErr := linkTx(ctx, tx, auth.Tenant, auth.Project, out.FacilityID)
		if linkErr != nil {
			return Result{}, false, linkErr
		}
		currentMeta := m
		currentMeta.AccessRevision = auth.AccessRevision
		if link != nil && !(action == "create" && t.Kind == Facility) {
			currentMeta.LinkRevision = link.Revision
		}
		if _, e = validateProjectWrite(ctx, tx, auth, t, action, currentMeta, r, nil); e != nil {
			return Result{}, false, e
		}
	}
	if oldOp != string(t.operation(string(t.Kind), action)) || oldHash != resourceFingerprint(t, action, m, in, auth, version) {
		return Result{}, false, problem("request_payload_conflict", "该请求编号已用于不同操作或内容。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func saveMutationReceipt(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, action string, m WriteMeta, in map[string]any, out Result) error {
	hash := resourceFingerprint(t, action, m, in, auth, 2)
	op := t.operation(string(t.Kind), action)
	if auth.Entry == "tenant" {
		_, e := tx.Exec(ctx, `INSERT INTO engineering_resource_receipts(tenant_id,actor_id,request_id,operation,payload_hash,result,fingerprint_version) VALUES($1,$2,$3,$4,$5,$6,2)`, auth.Tenant, auth.Account, m.RequestID, op, hash, out)
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO project_resource_operation_receipts(entry_scope,tenant_id,project_id,actor_id,request_id,operation,payload_hash,tenant_membership_id,project_membership_id,link_id,target_kind,target_id,result) VALUES('project',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, auth.Tenant, auth.Project, auth.Account, m.RequestID, op, hash, auth.TenantMembership, auth.ProjectAuth.MembershipID, out.LinkID, t.Kind, out.ID, out)
	return e
}
