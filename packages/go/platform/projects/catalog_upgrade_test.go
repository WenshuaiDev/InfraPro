package projects_test

import (
	"slices"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/jackc/pgx/v5"
)

func TestProjectResourceCatalogUpgradePreservesFrozenProjects(t *testing.T) {
	for _, state := range []string{"archived", "disabled"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			p := f.create(t, "resource-upgrade", f.bid)
			member := f.add(t, f.b, p.ProjectID, f.cid)
			role := f.role(t, p.ProjectID, "既有自定义角色", []access.Code{access.ProjectProfileRead})
			f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{role.ObjectID}})
			if state == "archived" {
				f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
			} else {
				f.lifecycle(t, "disabled")
			}
			// Reproduce the v9 projection: these capabilities did not exist in the
			// published catalog or in any role, even though frozen projects did.
			must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
				if _, e := tx.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code LIKE 'project.engineering.%'`); e != nil {
					return e
				}
				_, e := tx.Exec(ctx, `DELETE FROM permission_catalog WHERE code LIKE 'project.engineering.%'`)
				return e
			}))
			var before string
			const facts = `SELECT jsonb_build_array(to_jsonb(p),to_jsonb(t),(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.id) FROM project_memberships m WHERE m.project_id=p.id),(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM project_member_roles b WHERE b.project_id=p.id))::text FROM projects p JOIN tenants t ON t.id=p.tenant_id WHERE p.id=$1`
			must(t, f.e.Pool.QueryRow(ctx, facts, p.ProjectID).Scan(&before))
			report, e := f.e.Service.Access().SyncPreview(ctx)
			must(t, e)
			if len(report.Conflicts) != 0 {
				t.Fatalf("v9 resource catalog upgrade blocked: %v", report.Conflicts)
			}
			must(t, f.e.Service.Access().Sync(ctx))
			must(t, f.e.Service.Access().Check(ctx))
			var after string
			must(t, f.e.Pool.QueryRow(ctx, facts, p.ProjectID).Scan(&after))
			if before != after {
				t.Fatal("catalog upgrade changed tenant/project state or member bindings")
			}
			if count(t, f, `SELECT count(*) FROM project_facility_links`) != 0 || count(t, f, `SELECT count(*) FROM project_resource_grants`) != 0 || count(t, f, `SELECT count(*) FROM project_resource_policies WHERE allow_facility_create`) != 0 {
				t.Fatal("catalog upgrade granted resources or enabled creation")
			}
			must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
				var adminID string
				if e := tx.QueryRow(ctx, `SELECT administrator_membership_id::text FROM projects WHERE id=$1`, p.ProjectID).Scan(&adminID); e != nil {
					return e
				}
				admin, e := access.ProjectAuthorizationTx(ctx, tx, f.tid, p.ProjectID, adminID, "")
				if e != nil {
					return e
				}
				if !slices.Contains(admin.ConfiguredPermissions, access.ProjectEngineeringResourceRead) || slices.Contains(admin.AllowedOperations, access.ProjectResourceOperation("facility", "create")) || slices.Contains(admin.AllowedOperations, access.ProjectResourceOperation("point", "update")) {
					t.Error("upgrade did not retain frozen project write protection")
				}
				if state == "disabled" && len(admin.AllowedOperations) != 0 {
					t.Error("disabled tenant gained effective project operations")
				}
				ordinary, e := access.ProjectAuthorizationTx(ctx, tx, f.tid, p.ProjectID, member.ObjectID, "")
				if e == nil && !slices.Equal(ordinary.ConfiguredPermissions, []access.Code{access.ProjectProfileRead}) {
					t.Error("existing custom role was expanded")
				}
				return e
			}))
			audits := count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1 AND event_type='project.authorization.preset_synced'`, p.ProjectID)
			if audits != 1 {
				t.Fatal("missing project catalog publication audit", audits)
			}
			must(t, f.e.Service.Access().Sync(ctx))
			if count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1 AND event_type='project.authorization.preset_synced'`, p.ProjectID) != audits {
				t.Fatal("repeat sync wrote duplicate audit")
			}
		})
	}
}

func TestProjectResourceCatalogSyncStillRejectsFrozenExpansion(t *testing.T) {
	for _, scenario := range []string{"existing_catalog", "creation_enabled"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			p := f.create(t, "resource-sync-guard", f.bid)
			f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
			_, e := f.e.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code LIKE 'project.engineering.%'`)
			must(t, e)
			if scenario == "creation_enabled" {
				_, e = f.e.Pool.Exec(ctx, `DELETE FROM permission_catalog WHERE code LIKE 'project.engineering.%'`)
				must(t, e)
				_, e = f.e.Pool.Exec(ctx, `UPDATE project_resource_policies SET allow_facility_create=true WHERE project_id=$1`, p.ProjectID)
				must(t, e)
			}
			report, e := f.e.Service.Access().SyncPreview(ctx)
			must(t, e)
			if len(report.Conflicts) == 0 {
				t.Fatal("non-upgrade expansion was allowed")
			}
			if e = f.e.Service.Access().Sync(ctx); e == nil {
				t.Fatal("frozen expansion was committed")
			}
		})
	}
}
