package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/jackc/pgx/v5"
	"slices"
)

// fileHosts is the finite adapter for existing profiles. It reads only metadata
// inside the caller's transaction and never grants rights through governance.
type fileHosts struct{}

func (fileHosts) ResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope files.Scope, host files.HostRef, action files.HostAction) (files.HostAuthorization, error) {
	out := files.HostAuthorization{Host: host, Scope: scope, AccountID: a.ID}
	if a.ReadSubject() != nil || host != scope.Host() {
		return out, access.ErrDenied
	}
	var codes []access.Code
	var e error
	if scope.Kind == "tenant" {
		var snap access.Snapshot
		out.TenantMembershipID, snap, e = access.TenantSnapshotTx(ctx, tx, a.ID, scope.TenantID, "")
		if e != nil {
			return out, e
		}
		codes = snap.Permissions
		e = tx.QueryRow(ctx, `SELECT status,version::text FROM tenants WHERE id=$1 FOR SHARE`, scope.TenantID).Scan(&out.Status, &out.Version)
	} else if scope.Kind == "project" {
		var snap access.ProjectAuthorization
		snap, e = access.ProjectSnapshotTx(ctx, tx, a.ID, scope.TenantID, scope.ProjectID, "")
		if e != nil {
			return out, e
		}
		if snap.TenantStatus != "active" || snap.TenantMembershipStatus != "active" || snap.MembershipStatus != "active" || snap.AccountStatus != "active" {
			return out, access.ErrDenied
		}
		codes = snap.Permissions
		out.TenantMembershipID = snap.TenantMembershipID
		out.ProjectMembershipID = snap.MembershipID
		e = tx.QueryRow(ctx, `SELECT status,version::text FROM projects WHERE tenant_id=$1 AND id=$2 FOR SHARE`, scope.TenantID, scope.ProjectID).Scan(&out.Status, &out.Version)
	} else {
		return out, access.ErrDenied
	}
	if e != nil {
		return out, e
	}
	has := func(suffix string) bool { return slices.Contains(codes, access.Code(scope.Kind+"."+suffix)) }
	out.CanRead = has("profile.read") && has("attachments.read")
	out.CanDownload = out.CanRead && has("attachments.download")
	out.ManagePermission = out.CanRead && has("attachments.manage")
	out.WritePermission = out.ManagePermission && has("profile.update")
	out.CanManage = out.WritePermission && out.Status == "active"
	if !out.CanRead || action == files.Content && !out.CanDownload || action == files.Mutate && !out.CanManage {
		return out, access.ErrDenied
	}
	if !out.CanManage {
		out.Reason = "permission_denied"
		if out.Status == "archived" {
			out.Reason = "project_archived"
		}
	}
	return out, nil
}
