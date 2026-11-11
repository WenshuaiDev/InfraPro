package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Scope is selected by the registered route, never by an input field or target.
// Zero/ambiguous scopes are rejected before a database query.
type Scope struct {
	Entry   string `json:"kind"`
	Tenant  string `json:"tenant_id"`
	Project string `json:"project_id,omitempty"`
}

func TenantScope(tenant string) Scope { return Scope{Entry: "tenant", Tenant: tenant} }
func ProjectScope(tenant, project string) Scope {
	return Scope{Entry: "project", Tenant: tenant, Project: project}
}
func (s Scope) valid() bool {
	return access.ValidID(s.Tenant) && ((s.Entry == "tenant" && s.Project == "") || (s.Entry == "project" && access.ValidID(s.Project)))
}
func (s Scope) operation(kind, action string) access.Operation {
	if s.Entry == "project" {
		return access.ProjectResourceOperation(kind, action)
	}
	return access.ResourceOperation(kind, action)
}

type scopedTarget struct {
	Scope
	Target
	CursorScope string
}

func scoped(s Scope, t Target) scopedTarget {
	if t.Kind == Facility && t.Facility == "" {
		t.Facility = t.ID
	}
	return scopedTarget{Scope: s, Target: t}
}

type authorization struct {
	Scope
	Account             string
	TenantMembership    string
	ProjectAuth         access.ProjectAuthorization
	AccessRevision      string
	AllowFacilityCreate bool
	ProjectVersion      string
	ProjectStatus       string
	Governance          bool
}

func (s *Service) authorizeTx(ctx context.Context, tx pgx.Tx, a access.Actor, op access.Operation, scope Scope) (authorization, error) {
	out := authorization{Scope: scope, Account: a.ID}
	if scope.Entry == "project" {
		v, e := access.RequireProjectTx(ctx, tx, a.ID, op, scope.Tenant, scope.Project)
		if e != nil {
			return out, e
		}
		out.ProjectAuth = v
		out.TenantMembership = v.TenantMembershipID
		e = tx.QueryRow(ctx, `SELECT r.revision::text,r.allow_facility_create,p.version::text,p.status FROM project_resource_policies r JOIN projects p ON p.id=r.project_id WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&out.AccessRevision, &out.AllowFacilityCreate, &out.ProjectVersion, &out.ProjectStatus)
		return out, e
	}
	if e := access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
		return out, e
	}
	if e := tx.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, scope.Tenant, a.ID).Scan(&out.TenantMembership); e != nil {
		return out, e
	}
	out.Governance = access.RequireScopedTx(ctx, tx, a.ID, access.ResourceAccessOperation("access", "detail"), scope.Tenant) == nil
	return out, nil
}
