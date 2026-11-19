package runtime

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/jackc/pgx/v5"
)

// ComposeNotifications installs only metadata readers; it does not open Redis,
// ClickHouse, object storage or a converter and is safe for the independent worker.
func ComposeNotifications(a *access.Service) *notifications.Service {
	b := Compose(a)
	s := notifications.New(a, notificationResolver{b})
	return s.WithMaintenance(tenancy.New(a).ExpireInvitations, func(ctx context.Context) error { return bridgeDisplayNotifications(ctx, a) })
}

type notificationResolver struct{ b Business }

func deniedNotification(e error) bool {
	var ae *access.Error
	return errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404)
}
func (r notificationResolver) ResolveTx(ctx context.Context, tx pgx.Tx, account string, refs []notifications.Reference) (map[string]notifications.Disclosure, error) {
	out := map[string]notifications.Disclosure{}
	groups := map[string][]notifications.Reference{}
	for _, ref := range refs {
		key := ref.Scope.Kind + ":" + ref.Scope.Tenant + ":" + ref.Scope.Project + ":" + ref.Target.Kind
		groups[key] = append(groups[key], ref)
	}
	for _, group := range groups {
		ref := group[0]
		scope := ref.Scope
		ids := []string{}
		for _, v := range group {
			if !slices.Contains(ids, v.Target.ID) {
				ids = append(ids, v.Target.ID)
			}
		}
		name := ""
		tenantMember, projectMember := "", ""
		allowed := true
		var codes []access.Code
		var e error
		// A targeted invitation is a capability of the recipient account, before
		// membership. Its own domain decides which current context can be disclosed.
		if ref.Target.Kind != "tenant_invitation" {
			switch scope.Kind {
			case "tenant":
				var snap access.Snapshot
				member, v, err := access.TenantSnapshotTx(ctx, tx, account, scope.Tenant, "")
				snap, e = v, err
				tenantMember = member
				codes = snap.Permissions
			case "project":
				var snap access.ProjectAuthorization
				snap, e = access.ProjectSnapshotTx(ctx, tx, account, scope.Tenant, scope.Project, "")
				codes = snap.Permissions
				tenantMember, projectMember = snap.TenantMembershipID, snap.MembershipID
			}
			if e != nil {
				if deniedNotification(e) {
					allowed = false
				} else {
					return nil, e
				}
			}
			if (scope.Kind == "tenant" || scope.Kind == "project") && len(codes) == 0 {
				allowed = false
			}
			if allowed && scope.Tenant != "" {
				query := `SELECT name FROM tenants WHERE id=$1`
				id := scope.Tenant
				if scope.Project != "" {
					query = `SELECT name FROM projects WHERE tenant_id=$1 AND id=$2`
					e = tx.QueryRow(ctx, query, scope.Tenant, scope.Project).Scan(&name)
				} else {
					e = tx.QueryRow(ctx, query, id).Scan(&name)
				}
				if e != nil {
					return nil, e
				}
			}
		}
		if !allowed {
			continue
		}
		data := map[string]ne.TargetContext{}
		switch ref.Target.Kind {
		case "security", "workspaces":
			for _, id := range ids {
				data[id] = ne.TargetContext{Allowed: true, Available: true}
			}
		case "project_access", "spatial_access":
			for _, id := range ids {
				data[id] = ne.TargetContext{Allowed: slices.Contains(codes, access.ProjectProfileUpdate), Available: true}
			}
		case "signup_invitation":
			data, e = identity.NotificationInvitationsTx(ctx, tx, account, ids)
		case "tenant_invitation":
			eventIDs := make([]string, 0, len(group))
			for _, item := range group {
				eventIDs = append(eventIDs, item.EventID)
			}
			data, e = tenancy.NotificationInvitationsTx(ctx, tx, account, scope.Tenant, eventIDs)
		case "monitoring_import", "monitoring_job", "derived_job":
			data, e = r.b.Monitoring.NotificationTargetsTx(ctx, tx, account, monitoring.Scope{Entry: scope.Kind, Tenant: scope.Tenant, Project: scope.Project}, ref.Target.Kind, ids)
		case "derived_channel":
			data, e = r.b.DerivedMonitoring.NotificationChannelsTx(ctx, tx, account, scope.Tenant, ids)
		case "spatial_version":
			data, e = r.b.Spatial.NotificationTargetsTx(ctx, tx, account, spatial.Scope{Entry: scope.Kind, Tenant: scope.Tenant, Project: scope.Project}, ids)
		case "audit_export":
			data, e = r.b.Audit.NotificationTargetsTx(ctx, tx, account, audit.Scope{Kind: scope.Kind, Tenant: scope.Tenant, Project: scope.Project}, ids)
		case "file_job":
			data, e = r.b.Files.NotificationTargetsTx(ctx, tx, account, files.Scope{Kind: scope.Kind, TenantID: scope.Tenant, ProjectID: scope.Project}, ids)
		case "display":
			data, e = r.b.Displays.NotificationTargetsTx(ctx, tx, account, display.Scope{Tenant: scope.Tenant, Project: scope.Project}, ids)
		}
		if e != nil {
			if deniedNotification(e) {
				continue
			}
			return nil, e
		}
		for _, v := range group {
			d := data[v.Target.ID]
			reason := d.Reason
			if !d.Available && reason == "" {
				reason = "object_unavailable"
			}
			sameMembership := (v.TenantMembership == "" || v.TenantMembership == tenantMember) && (v.ProjectMembership == "" || v.ProjectMembership == projectMember)
			if v.Target.Kind == "tenant_invitation" {
				sameMembership = true
			}
			out[v.EventID] = notifications.Disclosure{Allowed: d.Allowed && sameMembership, ScopeName: name, ObjectName: d.Name, CurrentState: d.State, TargetAvailable: d.Available, Reason: reason}
			if d.Allowed && sameMembership && v.TaskReference != nil {
				ref, err := ComposeTasks(nil, r.b).ResolveReferenceTx(ctx, tx, access.Actor{ID: account}, *v.TaskReference)
				if err == nil {
					resolved := out[v.EventID]
					resolved.TaskID = ref.TaskID
					resolved.TaskGeneration = ref.Generation
					out[v.EventID] = resolved
				} else if !deniedNotification(err) {
					return nil, err
				}
			}

		}
	}
	return out, nil
}
