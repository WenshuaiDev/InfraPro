package access_test

import (
	"slices"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

func TestMonitoringArchiveAndBuiltinPermissions(t *testing.T) {
	for _, action := range []string{"overview", "details", "statistics", "exports.create", "snapshots.renew", "jobs.list"} {
		if access.ProjectActiveOnly(access.MonitoringOperation("project", action)) {
			t.Fatal("archived read denied", action)
		}
	}
	for _, action := range []string{"imports.create", "imports.publish", "imports.readopt", "batches.revoke", "imports.retry"} {
		if !access.ProjectActiveOnly(access.MonitoringOperation("project", action)) {
			t.Fatal("archived mutation permitted", action)
		}
	}
	for _, role := range access.Builtins() {
		if role.Key == access.TenantAdminKey && !slices.Contains(role.Permissions, access.TenantMonitoringDataRead) {
			t.Fatal("tenant administrator missing monitoring")
		}
		if role.Key == access.ProjectAdminKey && !slices.Contains(role.Permissions, access.ProjectMonitoringDataRead) {
			t.Fatal("project administrator missing monitoring")
		}
		if role.Key == access.ProjectMemberKey && slices.Contains(role.Permissions, access.ProjectMonitoringDataRead) {
			t.Fatal("ordinary member gained monitoring permission")
		}
	}
}
