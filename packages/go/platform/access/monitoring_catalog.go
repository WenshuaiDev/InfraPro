package access

import "strings"

const (
	TenantMonitoringJobsRead            Code = "tenant.monitoring.jobs.read"
	TenantMonitoringJobsManage          Code = "tenant.monitoring.jobs.manage"
	ProjectMonitoringJobsRead           Code = "project.monitoring.jobs.read"
	ProjectMonitoringJobsManage         Code = "project.monitoring.jobs.manage"
	TenantMonitoringDataRead            Code = "tenant.monitoring.data.read"
	TenantMonitoringHistoryRead         Code = "tenant.monitoring.data.history.read"
	TenantMonitoringDataExport          Code = "tenant.monitoring.data.export"
	TenantMonitoringImportsRead         Code = "tenant.monitoring.imports.read"
	TenantMonitoringImportsManage       Code = "tenant.monitoring.imports.manage"
	TenantMonitoringBatchesRevoke       Code = "tenant.monitoring.batches.revoke"
	TenantMonitoringSourceRead          Code = "tenant.monitoring.source.read"
	TenantMonitoringAuthorizationRead   Code = "tenant.monitoring.authorization.read"
	TenantMonitoringAuthorizationManage Code = "tenant.monitoring.authorization.manage"
	ProjectMonitoringDataRead           Code = "project.monitoring.data.read"
	ProjectMonitoringHistoryRead        Code = "project.monitoring.data.history.read"
	ProjectMonitoringDataExport         Code = "project.monitoring.data.export"
	ProjectMonitoringImportsRead        Code = "project.monitoring.imports.read"
	ProjectMonitoringImportsManage      Code = "project.monitoring.imports.manage"
	ProjectMonitoringBatchesRevoke      Code = "project.monitoring.batches.revoke"
	ProjectMonitoringSourceRead         Code = "project.monitoring.source.read"
)

func monitoringCodes(scope string) []Code {
	if scope == "tenant" {
		return []Code{TenantMonitoringDataRead, TenantMonitoringHistoryRead, TenantMonitoringDataExport, TenantMonitoringImportsRead, TenantMonitoringImportsManage, TenantMonitoringBatchesRevoke, TenantMonitoringSourceRead, TenantMonitoringAuthorizationRead, TenantMonitoringAuthorizationManage, TenantMonitoringJobsRead, TenantMonitoringJobsManage}
	}
	return []Code{ProjectMonitoringDataRead, ProjectMonitoringHistoryRead, ProjectMonitoringDataExport, ProjectMonitoringImportsRead, ProjectMonitoringImportsManage, ProjectMonitoringBatchesRevoke, ProjectMonitoringSourceRead, ProjectMonitoringJobsRead, ProjectMonitoringJobsManage}
}

func monitoringCatalog() []Permission {
	names := []string{"查看监测数据", "查看撤销观测历史", "导出监测数据", "查看导入批次", "导入与核对监测数据", "撤销监测批次", "下载完整监测源文件", "查看项目数据授权", "管理项目数据授权", "查看任务摘要", "管理任务执行"}
	out := []Permission{}
	for _, scope := range []string{"tenant", "project"} {
		for i, code := range monitoringCodes(scope) {
			risk := "high"
			if strings.HasSuffix(string(code), ".read") {
				risk = "normal"
			}
			name := names[i]
			if strings.HasSuffix(string(code), ".jobs.read") {
				name = "查看任务摘要"
			}
			if strings.HasSuffix(string(code), ".jobs.manage") {
				name = "管理任务执行"
			}
			out = append(out, Permission{Code: code, Name: name, Group: "监测数据", Scope: scope, Description: "按当前通道、时间范围和操作资格执行；资源可见不自动授予数据权限。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
		}
	}
	return out
}

func MonitoringOperation(scope, action string) Operation {
	return Operation("Monitoring:" + scope + ":" + action)
}

type monitoringRoute struct {
	action, method, path string
	permissions          []int
	read                 bool
}

var monitoringRoutes = []monitoringRoute{
	{"points.list", "GET", "/points", []int{0}, true},
	{"channels.list", "GET", "/points/:id/channels", []int{0}, true},
	{"channels.directory", "GET", "/channels", []int{0}, true},
	{"selections.create", "POST", "/selections", []int{0}, true},
	{"overview", "POST", "/queries/overview", []int{0}, true},
	{"latest", "POST", "/queries/latest", []int{0}, true},
	{"details", "POST", "/queries/details", []int{0}, true},
	{"trends", "POST", "/queries/trends", []int{0}, true},
	{"statistics", "POST", "/queries/statistics", []int{0}, true},
	{"snapshots.renew", "POST", "/snapshots/:id/renew", []int{0}, true},
	{"imports.list", "GET", "/imports", []int{3}, true},
	{"imports.detail", "GET", "/imports/:id", []int{3}, true},
	{"imports.create", "POST", "/imports", []int{3, 4}, false},
	{"upload.part", "PUT", "/imports/:id/upload/parts/:part", []int{3, 4}, false},
	{"upload.complete", "POST", "/imports/:id/upload/complete", []int{3, 4}, false},
	{"imports.mapping", "POST", "/imports/:id/mappings", []int{3, 4}, false},
	{"imports.validate", "POST", "/imports/:id/validate", []int{3, 4}, false},
	{"imports.issues", "GET", "/imports/:id/issues", []int{3}, true},
	{"imports.resolve", "POST", "/imports/:id/resolutions", []int{3, 4}, false},
	{"imports.publish", "POST", "/imports/:id/publish", []int{3, 4}, false},
	{"imports.cancel", "POST", "/imports/:id/cancel", []int{3, 4}, false},
	{"imports.retry", "POST", "/imports/:id/retry", []int{3, 4}, false},
	{"imports.readopt", "POST", "/imports/:id/readopt", []int{3, 4}, false},
	{"batches.detail", "GET", "/batches/:id", []int{3}, true},
	{"batches.preview", "POST", "/batches/:id/revoke-preview", []int{3, 5}, false},
	{"batches.revoke", "POST", "/batches/:id/revoke", []int{3, 5}, false},
	{"exports.create", "POST", "/exports", []int{0, 2}, true},
	// The job's original permission and current range are checked by the
	// service, allowing import-only and export-only users to see their jobs.
	{"jobs.list", "GET", "/jobs", nil, true},
	{"jobs.detail", "GET", "/jobs/:id", nil, true},
	{"jobs.cancel", "POST", "/jobs/:id/cancel", nil, true},
	{"exports.files", "GET", "/exports/:id/files", []int{0, 2}, true},
	{"exports.download", "GET", "/exports/:id/files/:part/download", []int{0, 2}, true},
	{"sources.download", "GET", "/source-files/:id/download", []int{6}, true},
}

func monitoringPolicies() []Policy {
	out := []Policy{}
	for _, scope := range []string{"tenant", "project"} {
		base := "/api/v1/tenants/:tenant_id"
		if scope == "project" {
			base += "/projects/:project_id"
		}
		codes := monitoringCodes(scope)
		for _, r := range monitoringRoutes {
			required := []Code{}
			for _, i := range r.permissions {
				required = append(required, codes[i])
			}
			out = append(out, Policy{MonitoringOperation(scope, r.action), r.method, base + "/monitoring" + r.path, scope, required, false, false})
		}
	}
	for _, r := range []struct {
		action, method, path string
		code                 Code
	}{{"access.list", "GET", "", TenantMonitoringAuthorizationRead}, {"access.grant", "POST", "", TenantMonitoringAuthorizationManage}, {"access.preview", "POST", "/:id/revoke-preview", TenantMonitoringAuthorizationManage}, {"access.revoke", "POST", "/:id/revoke", TenantMonitoringAuthorizationManage}} {
		out = append(out, Policy{MonitoringOperation("tenant", r.action), r.method, "/api/v1/tenants/:tenant_id/projects/:project_id/monitoring-access" + r.path, "tenant", []Code{TenantMonitoringAuthorizationRead, r.code}, false, false})
	}
	return out
}

// Read classification is independent of HTTP verbs. Export creation, snapshot
// renewal and read-only POSTs remain available in archived projects.
func MonitoringReadOperation(op Operation) bool {
	for _, r := range monitoringRoutes {
		if op == MonitoringOperation("project", r.action) || op == MonitoringOperation("tenant", r.action) {
			return r.read
		}
	}
	return op == MonitoringOperation("tenant", "access.list")
}
