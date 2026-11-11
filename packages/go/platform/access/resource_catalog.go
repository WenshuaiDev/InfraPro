package access

const (
	TenantEngineeringResourceRead   Code = "tenant.engineering.resource.read"
	TenantEngineeringResourceManage Code = "tenant.engineering.resource.manage"
)

func resourceCatalog() []Permission {
	return append([]Permission{
		{Code: TenantEngineeringResourceRead, Name: "查看工程资源", Group: "工程资源", Scope: "tenant", Description: "查看本租户设施、构件、测点、业务通道及变更历史。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: true},
		{Code: TenantEngineeringResourceManage, Name: "维护工程资源", Group: "工程资源", Scope: "tenant", Description: "须同时具有查看权限；新建设施及通道还需公共工程目录读取权限。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true},
	}, projectResourceCatalog()...)
}
func ResourceOperation(kind, action string) Operation {
	return Operation("Resource:" + kind + ":" + action)
}
func resourcePolicies() []Policy {
	out := []Policy{}
	base := "/api/v1/tenants/:tenant_id/engineering/resources"
	for _, v := range []struct{ kind, path string }{
		{"facility", base + "/facilities"}, {"component", base + "/facilities/:facility_id/components"}, {"point", base + "/facilities/:facility_id/points"}, {"channel", base + "/facilities/:facility_id/points/:point_id/channels"},
	} {
		param := map[string]string{"facility": "facility_id", "component": "component_id", "point": "point_id", "channel": "channel_id"}[v.kind]
		for _, r := range []struct{ action, method, suffix string }{
			{"list", "GET", ""}, {"create", "POST", ""}, {"detail", "GET", "/:" + param}, {"update", "PATCH", "/:" + param}, {"retire", "POST", "/:" + param + "/retire"}, {"restore", "POST", "/:" + param + "/restore"}, {"delete", "POST", "/:" + param + "/delete"}, {"preview", "POST", "/:" + param + "/mutation-preview"},
		} {
			codes := []Code{TenantEngineeringResourceRead}
			if r.method != "GET" {
				codes = append(codes, TenantEngineeringResourceManage)
			}
			if r.action == "create" && (v.kind == "facility" || v.kind == "channel") {
				codes = append(codes, TenantEngineeringCatalogRead)
			}
			out = append(out, Policy{ResourceOperation(v.kind, r.action), r.method, v.path + r.suffix, "tenant", codes, false, false})
		}
	}
	out = append(out, Policy{ResourceOperation("facility", "changes"), "GET", base + "/facilities/:facility_id/changes", "tenant", []Code{TenantEngineeringResourceRead}, false, false}, Policy{ResourceOperation("request", "detail"), "GET", base + "/requests/:request_id", "tenant", []Code{TenantEngineeringResourceRead}, false, false})
	return out
}
