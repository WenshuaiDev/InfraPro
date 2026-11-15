package access

import (
	"context"
	"slices"

	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

const (
	PlatformAuditExport    Code = "platform.audit.export"
	PlatformAuditTechnical Code = "platform.audit.technical.read"
	PlatformAuditManage    Code = "platform.audit.exports.manage"
	TenantAuditExport      Code = "tenant.audit.export"
	TenantAuditTechnical   Code = "tenant.audit.technical.read"
	TenantAuditManage      Code = "tenant.audit.exports.manage"
	ProjectAuditExport     Code = "project.audit.export"
	ProjectAuditTechnical  Code = "project.audit.technical.read"
	ProjectAuditManage     Code = "project.audit.exports.manage"
)

func auditCodes(scope string) []Code {
	return []Code{Code(scope + ".audit.export"), Code(scope + ".audit.technical.read"), Code(scope + ".audit.exports.manage")}
}
func auditCatalog() []Permission {
	out := []Permission{}
	for _, scope := range []string{"platform", "tenant", "project"} {
		for i, code := range auditCodes(scope) {
			out = append(out, Permission{Code: code, Name: []string{"导出本人获准审计记录", "查看审计技术详情", "管理审计导出任务"}[i], Group: "审计与操作追踪", Scope: scope, Description: "另需同空间审计读取权；技术详情不扩大业务读取，导出管理不授予他人成果下载权。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
		}
	}
	return out
}
func AuditOperation(scope, action string) Operation {
	return Operation("Audit:" + scope + ":" + action)
}
func auditPolicies() []Policy {
	out := []Policy{}
	for _, scope := range []string{"platform", "tenant", "project"} {
		base, class := "/api/v1/audit", "permission"
		if scope != "platform" {
			base = "/api/v1/tenants/:tenant_id"
			class = scope
			if scope == "project" {
				base += "/projects/:project_id"
			}
			base += "/audit"
		}
		for _, r := range []struct {
			action, method, path string
			export               bool
		}{
			{"capabilities", "GET", "/capabilities", false}, {"events", "GET", "/events", false}, {"event", "GET", "/events/:event_id", false}, {"trace", "GET", "/events/:event_id/trace", false}, {"target", "GET", "/events/:event_id/targets/:link_id", false},
			{"exports.create", "POST", "/exports", true}, {"exports", "GET", "/exports", false}, {"export", "GET", "/exports/:export_id", false}, {"download", "GET", "/exports/:export_id/generations/:generation/download", true}, {"download.head", "HEAD", "/exports/:export_id/generations/:generation/download", true},
		} {
			codes := []Code{Code(scope + ".audit.read")}
			if r.export {
				codes = append(codes, Code(scope+".audit.export"))
			}
			out = append(out, Policy{AuditOperation(scope, r.action), r.method, base + r.path, class, codes, false, false})
		}
	}
	return out
}

type AuditAuthorization struct {
	Scope          ar.Scope
	MembershipKey  string
	Revision       string
	Permissions    []Code
	AccountVersion string
}

func (a AuditAuthorization) Has(suffix string) bool {
	return slices.Contains(a.Permissions, Code(a.Scope.Kind+".audit."+suffix))
}

// AuditAuthorizationTx is the shared personnel-only scope gate. Callers hold
// the graph lock and revalidate their session (or durable worker delegation).
func AuditAuthorizationTx(ctx context.Context, tx pgx.Tx, actor Actor, scope ar.Scope, revision string) (AuditAuthorization, error) {
	out := AuditAuthorization{Scope: scope, Revision: revision, Permissions: []Code{}}
	if actor.ReadSubject() != nil || !scope.Valid() || !ar.ID(actor.ID) {
		return out, ErrDenied
	}
	var status string
	if e := tx.QueryRow(ctx, `SELECT status,auth_version::text FROM accounts WHERE id=$1`, actor.ID).Scan(&status, &out.AccountVersion); e != nil {
		return out, e
	}
	if status != "active" {
		return out, ErrDenied
	}
	var e error
	switch scope.Kind {
	case "platform":
		out.Permissions, e = PermissionsTx(ctx, tx, actor.ID)
	case "tenant":
		var snap Snapshot
		out.MembershipKey, snap, e = TenantSnapshotTx(ctx, tx, actor.ID, scope.Tenant, revision)
		out.Permissions = snap.Permissions
	case "project":
		var snap ProjectAuthorization
		snap, e = ProjectSnapshotTx(ctx, tx, actor.ID, scope.Tenant, scope.Project, revision)
		out.Permissions = snap.Permissions
		out.MembershipKey = snap.TenantMembershipID + ":" + snap.MembershipID
	}
	if e != nil {
		return out, e
	}
	if !out.Has("read") {
		return out, ErrDenied
	}
	return out, nil
}
