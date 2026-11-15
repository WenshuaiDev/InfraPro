package access

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

// AuditMetadataTx resolves only the finite governance references owned by the
// authorization graph. A historical fact does not grant a current detail route.
func AuditMetadataTx(ctx context.Context, tx pgx.Tx, a Actor, scope ar.Scope, kind, id string) (string, error) {
	if a.ReadSubject() != nil || !ValidID(id) {
		return "", ErrDenied
	}
	var op Operation
	switch kind {
	case "account":
		if scope.Kind != "platform" {
			return "", ErrDenied
		}
		op = OpAccountRead
	case "role":
		if scope.Kind != "platform" {
			return "", ErrDenied
		}
		op = OpRoleRead
	case "tenant":
		if scope.Kind == "platform" {
			op = OpTenantRead
		} else {
			op = OpTenantProfileRead
		}
	case "tenant_role":
		op = OpTenantRoleDetail
	case "tenant_member":
		op = OpTenantMemberDetail
	case "tenant_invitation":
		op = OpTenantInvitationDetail
	case "project":
		op = OpProjectProfile
		if scope.Kind == "tenant" {
			op = OpProjectGovernanceDetail
		}
	case "project_role":
		op = OpProjectRole
	case "project_member":
		op = OpProjectMember
	default:
		return "", ErrDenied
	}
	if scope.Kind == "project" {
		if _, e := RequireActorProjectTx(ctx, tx, a, op, scope.Tenant, scope.Project); e != nil {
			return "", e
		}
	} else if scope.Kind == "tenant" {
		if e := RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
			return "", e
		}
	} else if e := RequireTx(ctx, tx, a.ID, op); e != nil {
		return "", e
	}
	var name string
	var e error
	switch kind {
	case "account":
		e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(display_name,''),username) FROM accounts WHERE id=$1`, id).Scan(&name)
	case "role":
		e = tx.QueryRow(ctx, `SELECT name FROM roles WHERE id=$1 AND binding_scope='platform' AND deleted_at IS NULL`, id).Scan(&name)
	case "tenant":
		e = tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id=$1 AND ($2='' OR id::text=$2)`, id, scope.Tenant).Scan(&name)
	case "project":
		e = tx.QueryRow(ctx, `SELECT name FROM projects WHERE tenant_id=$1 AND id=$2 AND ($3='' OR id::text=$3)`, scope.Tenant, id, scope.Project).Scan(&name)
	case "tenant_role", "project_role":
		e = tx.QueryRow(ctx, `SELECT name FROM roles WHERE id=$1 AND owner_tenant_id=$2 AND owner_project_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid AND deleted_at IS NULL`, id, scope.Tenant, scope.Project).Scan(&name)
	case "tenant_member":
		e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(a.display_name,''),a.username) FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.id=$2`, scope.Tenant, id).Scan(&name)
	case "project_member":
		e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(a.display_name,''),a.username) FROM project_memberships m JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.project_id=$2 AND m.id=$3`, scope.Tenant, scope.Project, id).Scan(&name)
	case "tenant_invitation":
		e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(a.display_name,''),a.username) FROM tenant_invitations i JOIN accounts a ON a.id=i.target_account_id WHERE i.tenant_id=$1 AND i.id=$2`, scope.Tenant, id).Scan(&name)
	}
	return name, e
}
