package access

import "strings"

const (
	TenantEngineeringResourceAuthorizationRead   Code = "tenant.engineering.resource.authorization.read"
	TenantEngineeringResourceAuthorizationManage Code = "tenant.engineering.resource.authorization.manage"
	ProjectEngineeringResourceRead               Code = "project.engineering.resource.read"
	ProjectEngineeringResourceUpdate             Code = "project.engineering.resource.update"
	ProjectEngineeringResourceHistoryRead        Code = "project.engineering.resource.history.read"
	ProjectEngineeringResourceLifecycle          Code = "project.engineering.resource.lifecycle"
	ProjectEngineeringFacilityCreate             Code = "project.engineering.facility.create"
	ProjectEngineeringComponentCreate            Code = "project.engineering.component.create"
	ProjectEngineeringPointCreate                Code = "project.engineering.point.create"
	ProjectEngineeringChannelCreate              Code = "project.engineering.channel.create"
	ProjectEngineeringCatalogRead                Code = "project.engineering.catalog.read"
)

func projectResourceCodes() []Code {
	return []Code{ProjectEngineeringResourceRead, ProjectEngineeringResourceUpdate, ProjectEngineeringResourceHistoryRead, ProjectEngineeringResourceLifecycle, ProjectEngineeringFacilityCreate, ProjectEngineeringComponentCreate, ProjectEngineeringPointCreate, ProjectEngineeringChannelCreate, ProjectEngineeringCatalogRead}
}
func projectResourceCatalog() []Permission {
	out := []Permission{}
	for _, v := range []struct {
		code              Code
		name, description string
	}{
		{TenantEngineeringResourceAuthorizationRead, "查看项目资源授权", "同时需要租户资源查看和项目治理查看权限。"},
		{TenantEngineeringResourceAuthorizationManage, "管理项目资源授权", "同时需要授权查看、资源查看/维护和项目治理查看；不要求目标项目成员资格。"},
		{ProjectEngineeringResourceRead, "查看项目工程资源", "只读取租户明确授予本项目的对象。"},
		{ProjectEngineeringResourceUpdate, "编辑项目工程资源", "同时需要资源查看、对象编辑能力及运行中项目。"},
		{ProjectEngineeringResourceHistoryRead, "查看资源变更历史", "同时需要资源查看；只返回当前获准对象的公共历史。"},
		{ProjectEngineeringResourceLifecycle, "退役和恢复项目资源", "同时需要资源查看与对象生命周期能力；共享或不完整范围交由租户处理。"},
		{ProjectEngineeringFacilityCreate, "新建设施", "同时需要资源查看、公共目录读取及租户开启项目新建政策。"},
		{ProjectEngineeringComponentCreate, "新增构件", "同时需要资源查看与父对象新增构件能力。"},
		{ProjectEngineeringPointCreate, "新增测点", "同时需要资源查看与所属设施或构件新增测点能力。"},
		{ProjectEngineeringChannelCreate, "新增通道", "同时需要资源查看、公共目录读取及测点新增通道能力。"},
		{ProjectEngineeringCatalogRead, "读取公共建档目录", "仅可采用的公共类型、指标和单位；不含草稿及私有模板。"},
	} {
		scope := "project"
		risk := "high"
		if strings.HasPrefix(string(v.code), "tenant.") {
			scope = "tenant"
		}
		if strings.HasSuffix(string(v.code), ".read") {
			risk = "normal"
		}
		out = append(out, Permission{Code: v.code, Name: v.name, Description: v.description, Group: "工程资源", Scope: scope, Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
	}
	return out
}
func ProjectResourceOperation(kind, action string) Operation {
	return Operation("ProjectResource:" + kind + ":" + action)
}
func ResourceAccessOperation(kind, action string) Operation {
	return Operation("ProjectResourceAccess:" + kind + ":" + action)
}
func projectResourcePolicies() []Policy {
	out := []Policy{}
	base := "/api/v1/tenants/:tenant_id/projects/:project_id"
	for _, v := range []struct{ kind, path string }{
		{"facility", "/facilities"}, {"component", "/facilities/:facility_id/components"}, {"point", "/facilities/:facility_id/points"}, {"channel", "/facilities/:facility_id/points/:point_id/channels"},
	} {
		param := map[string]string{"facility": "facility_id", "component": "component_id", "point": "point_id", "channel": "channel_id"}[v.kind]
		for _, r := range []struct{ action, method, suffix string }{
			{"list", "GET", ""}, {"create", "POST", ""}, {"detail", "GET", "/:" + param}, {"update", "PATCH", "/:" + param}, {"retire", "POST", "/:" + param + "/retire"}, {"restore", "POST", "/:" + param + "/restore"}, {"preview", "POST", "/:" + param + "/mutation-preview"},
		} {
			if v.kind == "facility" && (r.action == "retire" || r.action == "restore") {
				continue
			}
			codes := []Code{ProjectEngineeringResourceRead}
			switch r.action {
			case "create":
				codes = append(codes, map[string]Code{"facility": ProjectEngineeringFacilityCreate, "component": ProjectEngineeringComponentCreate, "point": ProjectEngineeringPointCreate, "channel": ProjectEngineeringChannelCreate}[v.kind])
				if v.kind == "facility" || v.kind == "channel" {
					codes = append(codes, ProjectEngineeringCatalogRead)
				}
			case "update":
				codes = append(codes, ProjectEngineeringResourceUpdate)
			case "retire", "restore":
				codes = append(codes, ProjectEngineeringResourceLifecycle)
			}
			out = append(out, Policy{ProjectResourceOperation(v.kind, r.action), r.method, base + "/engineering/resources" + v.path + r.suffix, "project", codes, false, false})
		}
	}
	out = append(out, Policy{ProjectResourceOperation("facility", "changes"), "GET", base + "/engineering/resources/facilities/:facility_id/changes", "project", []Code{ProjectEngineeringResourceRead, ProjectEngineeringResourceHistoryRead}, false, false}, Policy{ProjectResourceOperation("request", "detail"), "GET", base + "/engineering/resources/requests/:request_id", "project", []Code{ProjectEngineeringResourceRead}, false, false})
	for _, kind := range []string{"type-definitions", "metrics", "units"} {
		for _, r := range []struct{ action, path string }{{"list", ""}, {"version", "/versions/:version_id"}} {
			out = append(out, Policy{ProjectResourceOperation(kind, r.action), "GET", base + "/engineering/catalog/" + kind + r.path, "project", []Code{ProjectEngineeringCatalogRead}, false, false})
		}
	}
	read := []Code{TenantEngineeringResourceRead, TenantProjectRead, TenantEngineeringResourceAuthorizationRead}
	out = append(out, Policy{ResourceAccessOperation("usage", "detail"), "GET", "/api/v1/tenants/:tenant_id/engineering/resources/facilities/:facility_id/project-usage", "tenant", read, false, false})
	write := append(append([]Code{}, read...), TenantEngineeringResourceAuthorizationManage, TenantEngineeringResourceManage)
	for _, r := range []struct{ kind, action, method, path string }{
		{"access", "detail", "GET", ""}, {"access", "changes", "GET", "/changes"},
		{"policy", "preview", "POST", "/policy/preview"}, {"policy", "change", "POST", "/policy"},
		{"facility", "detail", "GET", "/facilities/:facility_id"}, {"facility", "preview", "POST", "/facilities/:facility_id/preview"}, {"facility", "change", "POST", "/facilities/:facility_id/changes"},
		{"request", "detail", "GET", "/requests/:request_id"},
	} {
		codes := read
		if r.method != "GET" {
			codes = write
		}
		out = append(out, Policy{ResourceAccessOperation(r.kind, r.action), r.method, base + "/resource-access" + r.path, "tenant", codes, false, false})
	}
	return out
}
