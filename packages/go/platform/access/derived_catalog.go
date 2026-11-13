package access

import "strings"

const (
	TenantDerivedMetricsRead    Code = "tenant.derived.metrics.read"
	TenantDerivedMetricsManage  Code = "tenant.derived.metrics.manage"
	TenantDerivedMetricsPublish Code = "tenant.derived.metrics.publish"
	ProjectDerivedMetricsRead   Code = "project.derived.metrics.read"
)

type derivedPermission struct {
	suffix, name string
	tenantOnly   bool
}

var derivedPermissions = []derivedPermission{
	{"metrics.read", "查看派生指标定义", false}, {"metrics.manage", "管理派生指标定义", true}, {"metrics.publish", "发布派生指标定义", true},
	{"channels.read", "查看派生通道配置", false}, {"channels.manage", "管理派生通道配置", false}, {"channels.control", "启停派生通道计算", false}, {"channels.backfill", "派生通道历史回算与修正", false},
	{"runs.read", "查看计算运行", false}, {"runs.manage", "取消或重试计算运行", false}, {"results.history", "查看派生历史代次", false},
	{"analysis.read", "查看历史分析配置", false}, {"analysis.manage", "管理历史分析配置", false}, {"analysis.run", "运行历史分析", false}, {"analysis.results.read", "查看历史分析成果", false}, {"analysis.export", "导出历史分析成果", false},
}

func DerivedCode(scope, suffix string) Code { return Code(scope + ".derived." + suffix) }
func derivedCodes(scope string) []Code {
	out := []Code{}
	for _, p := range derivedPermissions {
		if scope == "tenant" || !p.tenantOnly {
			out = append(out, DerivedCode(scope, p.suffix))
		}
	}
	return out
}
func derivedCatalog() []Permission {
	out := []Permission{}
	for _, scope := range []string{"tenant", "project"} {
		for _, p := range derivedPermissions {
			if scope == "project" && p.tenantOnly {
				continue
			}
			risk := "high"
			if strings.HasSuffix(p.suffix, ".read") || p.suffix == "results.history" {
				risk = "normal"
			}
			out = append(out, Permission{Code: DerivedCode(scope, p.suffix), Name: p.name, Group: "派生监测与分析", Scope: scope, Description: "按当前空间、工程资源与递归输入时段权限执行；维护权限不隐式授予数据。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
		}
	}
	return out
}
func DerivedOperation(scope, action string) Operation {
	return Operation("Derived:" + scope + ":" + action)
}

type derivedRoute struct {
	action, method, path string
	permissions          []string
	tenantOnly, read     bool
}

var derivedRoutes = []derivedRoute{
	{"analysis.configs.list", "GET", "/analysis/configs", []string{"analysis.read"}, false, true},
	{"analysis.configs.detail", "GET", "/analysis/configs/:id", []string{"analysis.read"}, false, true},
	{"analysis.configs.create", "POST", "/analysis/configs", []string{"analysis.read", "analysis.manage"}, false, false},
	{"analysis.configs.save", "PATCH", "/analysis/configs/:id/draft", []string{"analysis.read", "analysis.manage"}, false, false},
	{"analysis.configs.copy", "POST", "/analysis/configs/:id/copy", []string{"analysis.read", "analysis.manage"}, false, false},
	{"analysis.configs.validate", "POST", "/analysis/configs/:id/validate", []string{"analysis.read", "analysis.manage"}, false, true},
	{"analysis.configs.publish", "POST", "/analysis/configs/:id/versions", []string{"analysis.read", "analysis.manage"}, false, false},
	{"analysis.configs.versions", "GET", "/analysis/configs/:id/versions", []string{"analysis.read"}, false, true},
	{"analysis.configs.archive", "POST", "/analysis/configs/:id/archive", []string{"analysis.read", "analysis.manage"}, false, false},
	{"analysis.trials", "POST", "/analysis/trials", []string{"analysis.read", "analysis.run"}, false, false},
	{"analysis.runs", "POST", "/analysis/runs", []string{"analysis.read", "analysis.run"}, false, false},
	{"analysis.artifacts.list", "GET", "/analysis/artifacts", []string{"analysis.results.read"}, false, true},
	{"analysis.artifacts.detail", "GET", "/analysis/artifacts/:id", []string{"analysis.results.read"}, false, true},
	{"analysis.artifacts.versions", "GET", "/analysis/artifacts/:id/versions", []string{"analysis.results.read"}, false, true},
	{"analysis.artifacts.query", "POST", "/analysis/artifacts/:id/query", []string{"analysis.results.read"}, false, true},
	{"analysis.artifacts.lineage", "POST", "/analysis/artifacts/:id/lineage", []string{"analysis.results.read"}, false, true},
	{"analysis.artifacts.archive", "POST", "/analysis/artifacts/:id/archive", []string{"analysis.results.read", "analysis.manage"}, false, false},
	{"analysis.exports.list", "GET", "/analysis/exports", []string{"analysis.results.read", "analysis.export"}, false, true},
	{"analysis.exports.detail", "GET", "/analysis/exports/:id", []string{"analysis.results.read", "analysis.export"}, false, true},
	{"analysis.exports.files", "GET", "/analysis/exports/:id/files", []string{"analysis.results.read", "analysis.export"}, false, true},
	{"analysis.exports.download", "GET", "/analysis/exports/:id/files/:part", []string{"analysis.results.read", "analysis.export"}, false, true},
	{"analysis.exports.cancel", "POST", "/analysis/exports/:id/cancel", []string{"analysis.results.read", "analysis.export", "runs.manage"}, false, true},
	{"analysis.exports.retry", "POST", "/analysis/exports/:id/retry", []string{"analysis.results.read", "analysis.export", "runs.manage"}, false, true},
	{"analysis.artifacts.exports", "POST", "/analysis/artifacts/:id/exports", []string{"analysis.results.read", "analysis.export"}, false, true},
	{"analysis.comparisons", "POST", "/analysis/comparisons", []string{"analysis.results.read"}, false, true},
	{"analysis.comparisons.query", "POST", "/analysis/comparisons/:id/query", []string{"analysis.results.read"}, false, true},
	{"channels.list", "GET", "/channels", []string{"channels.read"}, false, true},
	{"channels.detail", "GET", "/channels/:id", []string{"channels.read"}, false, true},
	{"channels.create", "POST", "/channels", []string{"channels.read", "channels.manage", "metrics.read"}, false, false},
	{"channels.configs.list", "GET", "/channels/:id/configs", []string{"channels.read"}, false, true},
	{"channels.configs.create", "POST", "/channels/:id/configs", []string{"channels.read", "channels.manage", "metrics.read"}, false, false},
	{"channels.assignments", "GET", "/channels/:id/assignments", []string{"channels.read"}, false, true},
	{"channels.config-preview", "POST", "/channels/:id/config-preview", []string{"channels.read", "channels.manage", "channels.control"}, false, false},
	{"channels.apply-config", "POST", "/channels/:id/apply-config", []string{"channels.read", "channels.manage", "channels.control"}, false, false},
	{"channels.dependencies", "GET", "/channels/:id/dependencies", []string{"channels.read"}, false, true},
	{"channels.backfill-preview", "POST", "/channels/:id/backfill-preview", []string{"channels.read", "channels.backfill"}, false, false},
	{"channels.backfill", "POST", "/channels/:id/backfill", []string{"channels.read", "channels.backfill"}, false, false},
	{"channels.generations", "GET", "/channels/:id/generations", []string{"channels.read", "results.history"}, false, true},
	{"generations.query", "POST", "/generations/:id/query", []string{"channels.read", "results.history"}, false, true},
	{"generations.lineage", "POST", "/generations/:id/lineage", []string{"channels.read", "results.history"}, false, true},
	{"generations.exports", "POST", "/generations/:id/exports", []string{"channels.read", "results.history"}, false, true},
	{"evidence.exports.list", "GET", "/evidence/exports", []string{"channels.read", "results.history"}, false, true},
	{"evidence.exports.detail", "GET", "/evidence/exports/:id", []string{"channels.read", "results.history"}, false, true},
	{"evidence.exports.files", "GET", "/evidence/exports/:id/files", []string{"channels.read", "results.history"}, false, true},
	{"evidence.exports.download", "GET", "/evidence/exports/:id/files/:part", []string{"channels.read", "results.history"}, false, true},
	{"evidence.exports.cancel", "POST", "/evidence/exports/:id/cancel", []string{"channels.read", "results.history", "runs.manage"}, false, true},
	{"evidence.exports.retry", "POST", "/evidence/exports/:id/retry", []string{"channels.read", "results.history", "runs.manage"}, false, true},
	{"generations.replay", "POST", "/generations/:id/replay", []string{"channels.read", "results.history"}, false, false},
	{"channels.trials", "POST", "/channels/:id/trials", []string{"channels.read"}, false, false},
	{"channels.control-preview", "POST", "/channels/:id/control-preview", []string{"channels.read", "channels.control"}, false, false},
	{"channels.control", "POST", "/channels/:id/control", []string{"channels.read", "channels.control"}, false, false},
	{"runs.list", "GET", "/runs", []string{"runs.read"}, false, true},
	{"runs.detail", "GET", "/runs/:id", []string{"runs.read"}, false, true},
	{"runs.cancel", "POST", "/runs/:id/cancel", []string{"runs.read", "runs.manage"}, false, false},
	{"runs.retry", "POST", "/runs/:id/retry", []string{"runs.read", "runs.manage"}, false, false},
	{"templates.list", "GET", "/templates", []string{"metrics.read"}, false, true},
	{"templates.expand", "POST", "/templates/:template/expand", []string{"metrics.read"}, false, true},
	{"metrics.list", "GET", "/metric-definitions", []string{"metrics.read"}, false, true},
	{"metrics.detail", "GET", "/metric-definitions/:id", []string{"metrics.read"}, false, true},
	{"metrics.create", "POST", "/metric-definitions", []string{"metrics.read", "metrics.manage"}, true, false},
	{"metrics.save", "PATCH", "/metric-definitions/:id/draft", []string{"metrics.read", "metrics.manage"}, true, false},
	{"metrics.copy", "POST", "/metric-definitions/:id/copy", []string{"metrics.read", "metrics.manage"}, true, false},
	{"metrics.validate", "POST", "/metric-definitions/:id/validate", []string{"metrics.read", "metrics.manage"}, true, true},
	{"metrics.publish", "POST", "/metric-definitions/:id/publish", []string{"metrics.read", "metrics.manage", "metrics.publish"}, true, false},
	{"metrics.versions", "GET", "/metric-definitions/:id/versions", []string{"metrics.read"}, false, true},
	{"versions.detail", "GET", "/metric-versions/:id", []string{"metrics.read"}, false, true},
	{"versions.state", "POST", "/metric-versions/:id/state", []string{"metrics.read", "metrics.manage"}, true, false},
	{"metrics.delete", "DELETE", "/metric-definitions/:id/draft", []string{"metrics.read", "metrics.manage"}, true, false},
}

func derivedPolicies() []Policy {
	out := []Policy{}
	for _, scope := range []string{"tenant", "project"} {
		base := "/api/v1/tenants/:tenant_id"
		if scope == "project" {
			base += "/projects/:project_id"
		}
		for _, r := range derivedRoutes {
			if scope == "project" && r.tenantOnly {
				continue
			}
			permissions := []Code{}
			for _, suffix := range r.permissions {
				permissions = append(permissions, DerivedCode(scope, suffix))
			}
			out = append(out, Policy{DerivedOperation(scope, r.action), r.method, base + "/monitoring/derived" + r.path, scope, permissions, false, false})
		}
	}
	return out
}
func DerivedReadOperation(op Operation) bool {
	for _, r := range derivedRoutes {
		if op == DerivedOperation("tenant", r.action) || op == DerivedOperation("project", r.action) {
			return r.read
		}
	}
	return false
}
