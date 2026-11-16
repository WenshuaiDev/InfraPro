package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

type auditDisclosure struct{ b Business }

func auditRestricted(e error) bool {
	if errors.Is(e, pgx.ErrNoRows) {
		return true
	}
	status := func(n int) bool { return n == 403 || n == 404 || n == 410 }
	var au *audit.Error
	var ae *access.Error
	var ke *k.Error
	var ee *engineering.Error
	var re *resources.Error
	var me *monitoring.Error
	var de *derivedmonitoring.Error
	var di *display.Error
	var fe *files.Error
	return errors.As(e, &au) && status(au.Status) || errors.As(e, &ae) && status(ae.Status) || errors.As(e, &ke) && status(ke.Status) || errors.As(e, &ee) && status(ee.Status) || errors.As(e, &re) && status(re.Status) || errors.As(e, &me) && status(me.Status) || errors.As(e, &de) && status(de.Status) || errors.As(e, &di) && status(di.Status) || errors.As(e, &fe) && status(fe.Status)
}
func (r auditDisclosure) ResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope audit.Scope, refs []audit.SubjectRef) (map[string]audit.Disclosure, error) {
	if len(refs) > 100 {
		return nil, audit.Problem("audit_query_budget_exceeded", 503)
	}
	out := map[string]audit.Disclosure{}
	for _, ref := range refs {
		// The event reader has already authorized scope. Domain entry points still
		// recheck their own metadata permissions and full-object grants here.
		var name string
		var e error
		allowed := true
		switch ref.Metadata {
		case "parsing":
			name, e = r.b.Parsing.AuditMetadataTx(ctx, tx, a, k.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
		case "governance":
			name, e = access.AuditMetadataTx(ctx, tx, a, ref.Scope, ref.ObjectType, ref.ObjectID)
		case "engineering":
			name, e = r.b.Engineering.AuditMetadataTx(ctx, tx, a, ref.Scope.Tenant, ref.ObjectID)
		case "resources":
			name, e = r.b.Resources.AuditMetadataTx(ctx, tx, a, resources.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
		case "devicecatalog":
			name, e = r.b.DeviceCatalog.AuditMetadataTx(ctx, tx, a, devicecatalog.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
		case "devices":
			name, e = r.b.Devices.AuditMetadataTx(ctx, tx, a, devices.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
		case "sources":
			name, e = r.b.Sources.AuditMetadataTx(ctx, tx, a, sources.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectID)
		case "files":
			name, e = r.b.Files.AuditMetadataTx(ctx, tx, a, files.Scope{Kind: ref.Scope.Kind, TenantID: ref.Scope.Tenant, ProjectID: ref.Scope.Project}, ref.ObjectID)
		case "derived":
			name, e = r.b.DerivedMonitoring.AuditMetadataTx(ctx, tx, a, derivedmonitoring.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
		case "monitoring":
			if ref.ObjectType == "monitoring_export" || ref.ObjectType == "export" || ref.ObjectType == "source" {
				var kind, id string
				kind, id, e = r.b.Monitoring.AuditReferenceTx(ctx, tx, a, monitoring.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectType, ref.ObjectID)
				if e != nil {
					break
				}
				ref.ObjectType, ref.ObjectID = kind, id
			}
			kind := "monitoring_job"
			if ref.ObjectType == "monitoring_import" || ref.ObjectType == "monitoring_batch" {
				kind = "monitoring_import"
			}
			values, err := r.b.Monitoring.NotificationTargetsTx(ctx, tx, a.ID, monitoring.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, kind, []string{ref.ObjectID})
			e = err
			v := values[ref.ObjectID]
			name = v.Name
			allowed = v.Allowed
		case "spatial":
			_, _, name, e = r.b.Spatial.AuditReferenceTx(ctx, tx, a, spatial.Scope{Entry: ref.Scope.Kind, Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, ref.ObjectID, ref.ObjectType)
		case "display":
			values, err := r.b.Displays.NotificationTargetsTx(ctx, tx, a.ID, display.Scope{Tenant: ref.Scope.Tenant, Project: ref.Scope.Project}, []string{ref.ObjectID})
			e = err
			v := values[ref.ObjectID]
			name = v.Name
			allowed = v.Allowed
		default:
			allowed = false
		}
		if auditRestricted(e) {
			allowed = false
			e = nil
		}
		if e != nil {
			return nil, fmt.Errorf("audit metadata %s/%s: %w", ref.Module, ref.ObjectType, e)
		}
		state := "available"
		if !allowed {
			state = "restricted"
		}
		out[ref.Key] = audit.Disclosure{Allowed: allowed, CurrentName: name, State: state}
	}
	return out, nil
}

type auditTrace struct{ b Business }

func (r auditTrace) ResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, links []ar.Link, technical bool) (map[string]audit.ResolvedLink, error) {
	out := map[string]audit.ResolvedLink{}
	notificationIDs := []string{}
	for _, l := range links {
		if l.Kind == "notification_event" {
			notificationIDs = append(notificationIDs, l.TargetID)
		}
	}
	delivery, e := notifications.AuditDeliverySummaryTx(ctx, tx, notificationIDs)
	if e != nil {
		return nil, e
	}
	for _, l := range links {
		item := audit.ResolvedLink{ID: l.ID, Kind: l.Kind, Label: "关联对象", State: "restricted"}
		var err error
		switch l.Kind {
		case "notification_event":
			if v, ok := delivery[l.TargetID]; ok {
				summary := &audit.DeliverySummary{State: v.State, ObservedAt: v.ObservedAt}
				if technical {
					summary.Count = &v.Count
					summary.ErrorCode = v.ErrorCode
				}
				item.Label = "通知投递"
				item.State = v.State
				item.Delivery = summary
			}
		case "task_execution":
			ref := taskrecord.SourceRef{Module: l.Module, Kind: l.TargetKind, RootID: l.TargetID}
			var views []tasks.View
			switch l.Module {
			case "monitoring":
				views, err = r.b.Monitoring.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "derived":
				views, err = r.b.DerivedMonitoring.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "spatial":
				views, err = r.b.Spatial.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "display":
				views, err = r.b.Displays.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "parsing":
				views, err = r.b.Parsing.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "audit":
				views, err = r.b.Audit.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
			case "files":
				var view tasks.View
				view, err = r.b.Files.AuditTaskTx(ctx, tx, a, ref)
				if err == nil {
					views = []tasks.View{view}
				}
			}
			if len(views) == 1 && err == nil {
				v := views[0]
				item.Label = v.Title
				item.State = "recorded"
				if v.Generation == l.Generation {
					item.State = v.State
				}
				item.Available = true
				item.Target = &audit.Target{Available: true, Kind: "task", Scope: l.Scope, ID: ref.ID(), Generation: l.Generation}
			}
		case "domain_file":
			if (l.Module == "monitoring" || l.Module == "derived") && l.TargetKind == "export" {
				v, e := r.b.Monitoring.AuditExportTargetTx(ctx, tx, a, monitoring.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetID, l.Generation)
				err = e
				if e == nil && v.Available {
					item.Label = "业务导出成果"
					item.State = "recorded"
					item.Available = true
					item.Target = &audit.Target{Available: true, Kind: v.Kind, Scope: l.Scope, ID: v.ID, Generation: l.Generation, Parameters: v.Parameters}
				}
			}
			if l.TargetKind == "source" && (l.Module == "monitoring" || l.Module == "spatial") {
				kind := "monitoring_import"
				if l.Module == "spatial" {
					kind = "spatial_version"
				}
				targetID := l.TargetID
				if l.Module == "monitoring" {
					kind, targetID, err = r.b.Monitoring.AuditReferenceTx(ctx, tx, a, monitoring.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, "source", l.TargetID)
					if err != nil {
						break
					}
				}
				values, e := (auditDisclosure{r.b}).ResolveTx(ctx, tx, a, l.Scope, []audit.SubjectRef{{Key: l.ID, Module: l.Module, Scope: l.Scope, ObjectType: kind, ObjectID: targetID, Metadata: l.Module}})
				err = e
				if e == nil && values[l.ID].Allowed {
					item.Label = values[l.ID].CurrentName
					item.State = "recorded"
					item.Available = true
					item.Target = &audit.Target{Available: true, Kind: kind, Scope: l.Scope, ID: targetID, Revision: l.Revision}
				}
			}
			if l.Module == "audit" {
				v, e := r.b.Audit.TaskTargetTx(ctx, tx, a, taskrecord.SourceRef{Module: "audit", Kind: "export", RootID: l.TargetID}, l.Generation, "result")
				err = e
				if e == nil && v.Available {
					item.Label = "审计导出成果"
					item.State = "recorded"
					item.Available = true
					item.Target = &audit.Target{Available: true, Kind: "audit_export", Scope: l.Scope, ID: l.TargetID, Generation: l.Generation}
				}
			}
		case "file_revision":
			_, err = r.b.Files.AuditReferenceTx(ctx, tx, a, files.Scope{Kind: l.Scope.Kind, TenantID: l.Scope.Tenant, ProjectID: l.Scope.Project}, l.TargetID, l.Revision)
			if err == nil {
				item.Label = "附件修订"
				item.State = "recorded"
				item.Available = true
				item.Target = &audit.Target{Available: true, Kind: "file_attachment", Scope: l.Scope, ID: l.TargetID, Revision: l.Revision}
			}
		case "business":
			if l.Module == "display" && l.TargetKind == "display_package" {
				var instance, name string
				instance, name, err = r.b.Displays.AuditPackageTx(ctx, tx, a, display.Scope{Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetID)
				if err == nil {
					item.Label = name + " · 大屏包"
					item.State = "available"
					item.Available = true
					item.Target = &audit.Target{Available: true, Kind: "display_package", Scope: l.Scope, ID: l.TargetID, Parameters: map[string]string{"instance": instance}}
				}
				break
			}
			metadata := map[string]string{"identity": "governance", "access": "governance", "tenancy": "governance", "projects": "governance", "engineering": "engineering", "resources": "resources", "devicecatalog": "devicecatalog", "devices": "devices", "sources": "sources", "monitoring": "monitoring", "derived": "derived", "spatial": "spatial", "display": "display", "files": "files"}[l.Module]
			if metadata != "" {
				values, e := (auditDisclosure{r.b}).ResolveTx(ctx, tx, a, l.Scope, []audit.SubjectRef{{Key: l.ID, Module: l.Module, ObjectType: l.TargetKind, ObjectID: l.TargetID, Revision: l.Revision, Scope: l.Scope, Metadata: metadata}})
				err = e
				if values[l.ID].Allowed {
					item.Label = values[l.ID].CurrentName
					item.State = "available"
					item.Available = true
					item.Target = &audit.Target{Available: true, Kind: l.TargetKind, Scope: l.Scope, ID: l.TargetID, Revision: l.Revision}
					if l.Module == "resources" && (l.TargetKind == "engineering_facility" || l.TargetKind == "engineering_component" || l.TargetKind == "engineering_point" || l.TargetKind == "engineering_channel") {
						item.Target.Parameters, err = r.b.Resources.AuditLocationTx(ctx, tx, a, resources.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetKind, l.TargetID)
					}
					if l.Module == "files" {
						item.Target.Kind, err = r.b.Files.AuditReferenceTx(ctx, tx, a, files.Scope{Kind: l.Scope.Kind, TenantID: l.Scope.Tenant, ProjectID: l.Scope.Project}, l.TargetID, "")
						if err == nil && item.Target.Kind == "file_job" {
							item.Target.ID = (taskrecord.SourceRef{Module: "files", Kind: "job", RootID: l.TargetID}).ID()
							item.Target.Kind = "task"
							item.Target.Generation = l.Generation
							if item.Target.Generation == "" {
								item.Target = nil
								item.Available = false
								item.State = "historical_reference_unavailable"
							}
						}
					}
					if l.Module == "spatial" {
						item.Target.Kind, item.Target.ID, _, err = r.b.Spatial.AuditReferenceTx(ctx, tx, a, spatial.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetID, l.TargetKind)
					}
					if l.Module == "monitoring" && (l.TargetKind == "monitoring_export" || l.TargetKind == "source") {
						item.Target.Kind, item.Target.ID, err = r.b.Monitoring.AuditReferenceTx(ctx, tx, a, monitoring.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetKind, l.TargetID)
					}
					if l.Module == "engineering" {
						var kind string
						kind, err = r.b.Engineering.AuditKindTx(ctx, tx, a, l.Scope.Tenant, l.TargetID)
						item.Target.Parameters = map[string]string{"kind": kind}
					}
					if l.Module == "sources" {
						item.Target.Parameters, err = r.b.Sources.AuditLocationTx(ctx, tx, a, sources.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetID)
					}
					if l.Module == "devices" && (l.TargetKind == "device_output" || l.TargetKind == "device_installation") {
						item.Target.Parameters, err = r.b.Devices.AuditLocationTx(ctx, tx, a, devices.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetKind, l.TargetID)
					}
					if l.Module == "resources" && (l.TargetKind == "project_resource_policy" || l.TargetKind == "project_resource_grant_set" || l.TargetKind == "project_facility_link") {
						item.Target.Parameters, err = r.b.Resources.AuditGrantLocationTx(ctx, tx, a, resources.Scope{Entry: l.Scope.Kind, Tenant: l.Scope.Tenant, Project: l.Scope.Project}, l.TargetKind, l.TargetID)
					}
				}
			}
		}
		if auditRestricted(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("audit reference %s/%s: %w", l.Module, l.TargetKind, err)
		}
		if item.State != "restricted" {
			out[l.ID] = item
		}
	}
	return out, nil
}

func (r auditDisclosure) RevisionTx(ctx context.Context, tx pgx.Tx, scope audit.Scope) (string, error) {
	if scope.Kind != "project" {
		return "", nil
	}
	parts := []string{}
	for _, read := range []func(context.Context, pgx.Tx, string, string) (string, error){r.b.Resources.AuditRevisionTx, r.b.Devices.AuditRevisionTx, r.b.Monitoring.AuditRevisionTx, r.b.Spatial.AuditRevisionTx} {
		v, e := read(ctx, tx, scope.Tenant, scope.Project)
		if e != nil {
			return "", e
		}
		parts = append(parts, v)
	}
	return ar.Hash(parts), nil
}
