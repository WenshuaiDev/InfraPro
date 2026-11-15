package access

const (
	ProjectDisplayView      Code = "project.display.view"
	ProjectDisplaySettings  Code = "project.display.settings"
	ProjectDisplayManage    Code = "project.display.manage"
	ProjectDisplayUpload    Code = "project.display.upload"
	ProjectDisplayConfigure Code = "project.display.configure"
	ProjectDisplayGrant     Code = "project.display.grant"
	ProjectDisplayPreview   Code = "project.display.preview"
	ProjectDisplayPublish   Code = "project.display.publish"
	ProjectDisplayTerminals Code = "project.display.terminals"
)

func displayCodes() []Code {
	return []Code{ProjectDisplayView, ProjectDisplaySettings, ProjectDisplayManage, ProjectDisplayUpload, ProjectDisplayConfigure, ProjectDisplayGrant, ProjectDisplayPreview, ProjectDisplayPublish, ProjectDisplayTerminals}
}
func displayCatalog() []Permission {
	names := []string{"查看项目大屏", "启停项目大屏", "管理大屏实例", "上传大屏程序", "配置大屏与绑定", "委托大屏只读范围", "预览与开发调试", "发布与回退大屏", "管理固定终端"}
	out := []Permission{}
	for i, c := range displayCodes() {
		risk := "high"
		if c == ProjectDisplayView {
			risk = "normal"
		}
		out = append(out, Permission{Code: c, Name: names[i], Description: "限定当前项目与实例；业务读取和大屏资格分别验证。", Group: "项目大屏", Scope: "project", Risk: risk, Lifecycle: "active", ProjectCustomRoleAssignable: true})
	}
	return out
}
func DisplayOperation(action string) Operation { return Operation("Display:" + action) }

type displayRoute struct {
	action, method, path string
	permission           Code
	archived             bool
}

var displayRoutes = []displayRoute{
	{"list", "GET", "", ProjectDisplayView, true},
	{"settings", "PUT", "/settings", ProjectDisplaySettings, true},
	{"create", "POST", "/instances", ProjectDisplayManage, false},
	{"detail", "GET", "/instances/:id", ProjectDisplayView, true},
	{"instance", "PATCH", "/instances/:id", ProjectDisplayManage, true},
	{"tool.authorize", "POST", "/instances/:id/tool-authorize", ProjectDisplayUpload, false},
	{"upload", "POST", "/instances/:id/uploads", ProjectDisplayUpload, false},
	{"upload.get", "GET", "/instances/:id/uploads/:upload_id", ProjectDisplayUpload, true},
	{"upload.part", "PUT", "/instances/:id/uploads/:upload_id/parts/:part", ProjectDisplayUpload, false},
	{"upload.complete", "POST", "/instances/:id/uploads/:upload_id/complete", ProjectDisplayUpload, false},
	{"upload.cancel", "POST", "/instances/:id/uploads/:upload_id/cancel", ProjectDisplayUpload, true},
	{"package.retry", "POST", "/instances/:id/uploads/:upload_id/retry", ProjectDisplayUpload, false},
	{"package.delete", "DELETE", "/instances/:id/packages/:package_id", ProjectDisplayUpload, false},
	{"config", "POST", "/instances/:id/configs", ProjectDisplayConfigure, false},
	{"grant", "PUT", "/instances/:id/grant", ProjectDisplayGrant, true},
	{"prepare", "POST", "/instances/:id/prepare", ProjectDisplayPublish, false},
	{"publish", "POST", "/instances/:id/publish", ProjectDisplayPublish, false},
	{"rollback", "POST", "/instances/:id/rollback", ProjectDisplayPublish, false},
	{"withdraw", "POST", "/instances/:id/withdraw", ProjectDisplayPublish, true},
	{"launch", "POST", "/instances/:id/launch", ProjectDisplayView, true},
	{"preview", "POST", "/instances/:id/preview", ProjectDisplayPreview, false},
	{"terminal.create", "POST", "/instances/:id/terminals", ProjectDisplayTerminals, false},
	{"terminal.update", "PATCH", "/instances/:id/terminals/:terminal_id", ProjectDisplayTerminals, true},
}

func displayPolicies() []Policy {
	out := []Policy{{DisplayOperation("locate"), "GET", "/api/v1/displays/:id/location", "authenticated", nil, false, false}}
	for _, r := range displayRoutes {
		out = append(out, Policy{DisplayOperation(r.action), r.method, "/api/v1/tenants/:tenant_id/projects/:project_id/displays" + r.path, "project", []Code{r.permission}, false, false})
	}
	return out
}
func DisplayActiveOnly(op Operation) bool {
	for _, r := range displayRoutes {
		if op == DisplayOperation(r.action) {
			return !r.archived
		}
	}
	return true
}
