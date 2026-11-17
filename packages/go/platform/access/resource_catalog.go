package access

import "strings"

const (
	TenantEngineeringResourceRead   Code = "tenant.engineering.resource.read"
	TenantEngineeringTypeAdopt      Code = "tenant.engineering.type.adopt"
	TenantEngineeringResourceManage Code = "tenant.engineering.resource.manage"
)

func resourceCatalog() []Permission {
	return append([]Permission{
		{Code: TenantEngineeringTypeAdopt, Name: "采用兼容设施类型版本", Group: "工程资源", Scope: "tenant", Description: "同类型定义内预览并采用兼容发布版本，影响所有共享项目。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true},
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
	out = append(out, Policy{ResourceOperation("tree", "list"), "GET", base + "/tree", "tenant", []Code{TenantEngineeringResourceRead}, false, false})
	for _, v := range []struct{ kind, path string }{
		{"facility", base + "/facilities"}, {"component", base + "/facilities/:facility_id/components"}, {"monitoring_section", base + "/facilities/:facility_id/monitoring-sections"}, {"borehole", base + "/facilities/:facility_id/boreholes"}, {"point", base + "/facilities/:facility_id/points"}, {"channel", base + "/facilities/:facility_id/points/:point_id/channels"},
	} {
		param := map[string]string{"facility": "facility_id", "component": "component_id", "monitoring_section": "monitoring_section_id", "borehole": "borehole_id", "point": "point_id", "channel": "channel_id"}[v.kind]
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
	for _, action := range []string{"adopt-type-preview", "adopt-type"} {
		out = append(out, Policy{ResourceOperation("facility", action), "POST", base + "/facilities/:facility_id/" + action, "tenant", []Code{TenantEngineeringResourceRead, TenantEngineeringResourceManage, TenantEngineeringCatalogRead, TenantEngineeringTypeAdopt}, false, false})
	}

	for _, kind := range []string{"facility", "component", "monitoring_section", "borehole", "point"} {
		path := map[string]string{"facility": "/facilities/:facility_id", "component": "/facilities/:facility_id/components/:component_id", "monitoring_section": "/facilities/:facility_id/monitoring-sections/:monitoring_section_id", "borehole": "/facilities/:facility_id/boreholes/:borehole_id", "point": "/facilities/:facility_id/points/:point_id"}[kind]
		for _, action := range []string{"attachment-content", "attachment-preview"} {
			out = append(out, Policy{ResourceOperation(kind, action), "GET", base + path + "/attachments/:attachment_ref_id/" + action, "tenant", []Code{TenantEngineeringResourceRead}, false, false})
		}
		for _, action := range []string{"structure-details", "structure-records-preview", "structure-records"} {
			codes := []Code{TenantEngineeringResourceRead}
			method := "GET"
			if action != "structure-details" {
				method = "POST"
				codes = append(codes, TenantEngineeringResourceManage)
			}
			out = append(out, Policy{ResourceOperation(kind, action), method, base + path + "/" + action, "tenant", codes, false, false})
		}
	}
	for _, kind := range []string{"component", "monitoring_section", "borehole", "point"} {
		path := map[string]string{"component": "components", "monitoring_section": "monitoring-sections", "borehole": "boreholes", "point": "points"}[kind]
		for _, action := range []string{"move-preview", "move"} {
			out = append(out, Policy{ResourceOperation(kind, action), "POST", base + "/facilities/:facility_id/" + path + "/:" + kind + "_id/" + action, "tenant", []Code{TenantEngineeringResourceRead, TenantEngineeringResourceManage}, false, false})
		}
	}

	// Existing point paths remain stable. Resource lifecycle for a channel also
	// has an owner-neutral path for facility and component ownership.
	for _, policy := range append([]Policy(nil), out...) {
		if strings.HasPrefix(string(policy.Operation), "Resource:channel:") && policy.Operation != ResourceOperation("channel", "create") {
			policy.Operation = Operation(strings.Replace(string(policy.Operation), ":channel:", ":channel-object:", 1))
			policy.Path = strings.Replace(policy.Path, "/points/:point_id/channels", "/channels", 1)
			out = append(out, policy)
		}
	}
	for _, kind := range []string{"type-definitions", "metrics", "units"} {
		out = append(out, Policy{ResourceOperation(kind, "version"), "GET", "/api/v1/tenants/:tenant_id/engineering/catalog/" + kind + "/versions/:version_id", "tenant", []Code{TenantEngineeringCatalogRead}, false, false})
	}

	return out
}
