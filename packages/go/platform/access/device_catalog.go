package access

import "strings"

const (
	DeviceDefinitionRead            Code = "device.definition.read"
	DeviceManufacturerManage        Code = "device.manufacturer.manage"
	DeviceProductEdit               Code = "device.product.edit"
	DeviceProductPublish            Code = "device.product.publish"
	DeviceProductState              Code = "device.product.state"
	TenantDeviceCatalogRead         Code = "tenant.device.catalog.read"
	TenantDeviceDefinitionRead      Code = "tenant.device.definition.read"
	TenantDeviceManufacturerManage  Code = "tenant.device.manufacturer.manage"
	TenantDeviceProductEdit         Code = "tenant.device.product.edit"
	TenantDeviceProductPublish      Code = "tenant.device.product.publish"
	TenantDeviceProductState        Code = "tenant.device.product.state"
	TenantDeviceRead                Code = "tenant.device.read"
	TenantDeviceManage              Code = "tenant.device.manage"
	TenantDeviceInstall             Code = "tenant.device.install"
	TenantDeviceState               Code = "tenant.device.state"
	TenantDeviceOutputManage        Code = "tenant.device.output.manage"
	TenantDeviceHistoryRead         Code = "tenant.device.history.read"
	TenantMonitoringSourceManage    Code = "tenant.monitoring.source.manage"
	TenantDeviceAuthorizationRead   Code = "tenant.device.authorization.read"
	TenantDeviceAuthorizationManage Code = "tenant.device.authorization.manage"
	ProjectDeviceRead               Code = "project.device.read"
	ProjectDeviceHistoryRead        Code = "project.device.history.read"
	ProjectDeviceOutputUse          Code = "project.device.output.use"
	ProjectMonitoringSourceManage   Code = "project.monitoring.source.manage"
)

func projectDeviceCodes() []Code {
	return []Code{ProjectDeviceRead, ProjectDeviceHistoryRead, ProjectDeviceOutputUse, ProjectMonitoringSourceManage}
}
func tenantDeviceCodes() []Code {
	out := []Code{}
	for _, p := range deviceCatalog() {
		if p.Scope == "tenant" {
			out = append(out, p.Code)
		}
	}
	return out
}
func deviceCatalog() []Permission {
	out := []Permission{}
	for _, v := range []struct {
		code Code
		name string
	}{
		{DeviceDefinitionRead, "查看公共设备产品定义"}, {DeviceManufacturerManage, "维护公共设备厂家"}, {DeviceProductEdit, "编制公共设备产品"}, {DeviceProductPublish, "发布公共设备产品"}, {DeviceProductState, "启停公共产品版本"},
		{TenantDeviceCatalogRead, "读取公共设备产品目录"}, {TenantDeviceDefinitionRead, "查看私有设备产品定义"}, {TenantDeviceManufacturerManage, "维护私有设备厂家"}, {TenantDeviceProductEdit, "编制私有设备产品"}, {TenantDeviceProductPublish, "发布私有设备产品"}, {TenantDeviceProductState, "启停私有产品版本"},
		{TenantDeviceRead, "查看租户设备"}, {TenantDeviceManage, "维护设备资料"}, {TenantDeviceInstall, "维护设备安装"}, {TenantDeviceState, "启停设备"}, {TenantDeviceOutputManage, "配置设备输出"}, {TenantDeviceHistoryRead, "查看设备历史"}, {TenantMonitoringSourceManage, "维护监测通道来源"}, {TenantDeviceAuthorizationRead, "查看项目设备授权"}, {TenantDeviceAuthorizationManage, "管理项目设备授权"},
		{ProjectDeviceRead, "查看获准设备和输出"}, {ProjectDeviceHistoryRead, "查看获准设备历史"}, {ProjectDeviceOutputUse, "使用获准设备输出"}, {ProjectMonitoringSourceManage, "维护获准通道来源"},
	} {
		scope := "platform"
		if strings.HasPrefix(string(v.code), "tenant.") {
			scope = "tenant"
		}
		if strings.HasPrefix(string(v.code), "project.") {
			scope = "project"
		}
		risk := "high"
		if strings.HasSuffix(string(v.code), ".read") {
			risk = "normal"
		}
		out = append(out, Permission{Code: v.code, Name: v.name, Description: "按当前作用范围、读取资格与明确对象授权执行；状态变化不删除业务履历。", Group: "设备管理", Scope: scope, Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
	}
	return out
}
func DeviceOperation(scope, kind, action string) Operation {
	return Operation("Device:" + scope + ":" + kind + ":" + action)
}
func devicePolicies() []Policy {
	out := []Policy{}
	add := func(scope, kind, action, method, path string, codes ...Code) {
		policyScope := scope
		if scope == "platform" {
			policyScope = "permission"
			if kind == "request" {
				policyScope = "authenticated"
			}
		}
		out = append(out, Policy{DeviceOperation(scope, kind, action), method, path, policyScope, codes, false, false})
	}
	for _, scope := range []string{"platform", "tenant"} {
		base := "/api/v1/platform/device-definitions"
		read, manufacturer, edit, publish, state := DeviceDefinitionRead, DeviceManufacturerManage, DeviceProductEdit, DeviceProductPublish, DeviceProductState
		if scope == "tenant" {
			base = "/api/v1/tenants/:tenant_id/device-definitions"
			read, manufacturer, edit, publish, state = TenantDeviceDefinitionRead, TenantDeviceManufacturerManage, TenantDeviceProductEdit, TenantDeviceProductPublish, TenantDeviceProductState
		}
		for _, v := range []struct {
			kind, action, method, path string
			write                      Code
		}{
			{"manufacturer", "list", "GET", "/manufacturers", ""}, {"manufacturer", "create", "POST", "/manufacturers", manufacturer}, {"manufacturer", "detail", "GET", "/manufacturers/:id", ""}, {"manufacturer", "update", "PATCH", "/manufacturers/:id", manufacturer}, {"manufacturer", "state", "POST", "/manufacturers/:id/state", manufacturer}, {"manufacturer", "delete", "POST", "/manufacturers/:id/delete", manufacturer}, {"manufacturer", "preview", "POST", "/manufacturers/:id/mutation-preview", manufacturer},
			{"product", "list", "GET", "/products", ""}, {"product", "create", "POST", "/products", edit}, {"product", "detail", "GET", "/products/:id", ""}, {"product", "draft-save", "PUT", "/products/:id/draft", edit}, {"product", "draft-create", "POST", "/products/:id/drafts", edit}, {"product", "draft-delete", "POST", "/products/:id/draft/delete", edit}, {"product", "validate", "POST", "/products/:id/validate", edit}, {"product", "publish", "POST", "/products/:id/publish", publish}, {"product", "version", "GET", "/products/:id/versions/:version_id", ""}, {"product", "state", "POST", "/products/:id/versions/:version_id/state", state}, {"product", "preview", "POST", "/products/:id/mutation-preview", ""}, {"product", "delete", "POST", "/products/:id/delete", edit}, {"product", "copy", "POST", "/products/copy", edit}, {"catalog", "changes", "GET", "/changes", ""},
		} {
			codes := []Code{read}
			if v.write != "" {
				codes = append(codes, v.write)
			}
			add(scope, v.kind, v.action, v.method, base+v.path, codes...)
		}
	}
	t := "/api/v1/tenants/:tenant_id"
	j := t + "/projects/:project_id"
	for _, v := range []struct{ kind, action, path string }{
		{"catalog-manufacturer", "list", "/manufacturers"}, {"catalog-product", "list", "/products"}, {"catalog-product", "versions", "/products/:id/versions"}, {"catalog-product", "version", "/products/:id/versions/:version_id"},
	} {
		add("tenant", v.kind, v.action, "GET", t+"/device-catalog"+v.path, TenantDeviceCatalogRead)
	}
	for _, v := range []struct {
		kind, action, method, path string
		write                      Code
	}{
		{"device", "list", "GET", "/devices", ""}, {"device", "create", "POST", "/devices", TenantDeviceManage}, {"device", "detail", "GET", "/devices/:device_id", ""}, {"device", "update", "PATCH", "/devices/:device_id", TenantDeviceManage}, {"device", "state", "POST", "/devices/:device_id/state", TenantDeviceState}, {"device", "delete", "POST", "/devices/:device_id/delete", TenantDeviceManage}, {"device", "preview", "POST", "/devices/:device_id/mutation-preview", ""},
		{"output", "slots", "GET", "/devices/:device_id/outputs", ""}, {"output", "generations", "GET", "/devices/:device_id/output-generations", TenantDeviceHistoryRead}, {"output", "create", "POST", "/devices/:device_id/outputs", TenantDeviceOutputManage}, {"output", "detail", "GET", "/devices/:device_id/outputs/:output_id", ""}, {"output", "update", "PATCH", "/devices/:device_id/outputs/:output_id", TenantDeviceOutputManage}, {"output", "replace", "POST", "/devices/:device_id/outputs/:output_id/replace", TenantDeviceOutputManage}, {"output", "end", "POST", "/devices/:device_id/outputs/:output_id/end", TenantDeviceOutputManage},
		{"installation", "list", "GET", "/devices/:device_id/installations", ""}, {"installation", "create", "POST", "/devices/:device_id/installations", TenantDeviceInstall}, {"installation", "end", "POST", "/devices/:device_id/installations/:installation_id/end", TenantDeviceInstall}, {"installation", "relocate", "POST", "/devices/:device_id/installations/:installation_id/relocate", TenantDeviceInstall}, {"installation", "correct", "PATCH", "/devices/:device_id/installations/:installation_id", TenantDeviceInstall}, {"installation", "targets", "GET", "/devices/:device_id/installation-targets", TenantDeviceInstall},
		{"device", "changes", "GET", "/devices/:device_id/changes", TenantDeviceHistoryRead},
	} {
		codes := []Code{TenantDeviceRead}
		if v.write != "" {
			codes = append(codes, v.write)
		}
		add("tenant", v.kind, v.action, v.method, t+v.path, codes...)
	}
	add("tenant", "device", "product-candidates", "GET", t+"/devices/product-candidates", TenantDeviceRead, TenantDeviceManage)
	add("tenant", "tree", "list", "GET", t+"/device-tree") // service independently checks each readable branch
	add("project", "tree", "list", "GET", j+"/device-tree", ProjectDeviceRead)
	for _, v := range []struct {
		kind, action, path string
		history            bool
	}{
		{"device", "list", "/devices", false}, {"device", "detail", "/devices/:device_id", false}, {"installation", "list", "/devices/:device_id/installations", true}, {"device", "changes", "/devices/:device_id/changes", true}, {"output", "list", "/device-outputs", false}, {"output", "detail", "/device-outputs/:output_id", false}, {"output", "changes", "/device-outputs/:output_id/changes", true},
	} {
		codes := []Code{ProjectDeviceRead}
		if v.history {
			codes = append(codes, ProjectDeviceHistoryRead)
		}
		add("project", v.kind, v.action, "GET", j+v.path, codes...)
	}
	for _, scope := range []string{"tenant", "project"} {
		base := t
		read, history, deviceRead, deviceHistory, manage := TenantEngineeringResourceRead, TenantEngineeringResourceRead, TenantDeviceRead, TenantDeviceHistoryRead, TenantMonitoringSourceManage
		if scope == "project" {
			base = j
			read, history, deviceRead, deviceHistory, manage = ProjectEngineeringResourceRead, ProjectEngineeringResourceHistoryRead, ProjectDeviceRead, ProjectDeviceHistoryRead, ProjectMonitoringSourceManage
		}
		c := base + "/engineering/resources/facilities/:facility_id/points/:point_id/channels/:channel_id/source"
		for _, v := range []struct{ action, method, suffix string }{{"detail", "GET", ""}, {"history", "GET", "/history"}, {"candidates", "GET", "/candidates"}, {"preview", "POST", "/preview"}, {"bind", "POST", "/bind"}, {"switch", "POST", "/switch"}, {"unbind", "POST", "/unbind"}} {
			codes := []Code{read}
			if v.action == "history" && history != read {
				codes = append(codes, history)
			}
			if v.action == "candidates" || v.method == "POST" {
				codes = append(codes, deviceRead, manage)
				if scope == "project" {
					codes = append(codes, ProjectDeviceOutputUse)
				}
			}
			add(scope, "source", v.action, v.method, c+v.suffix, codes...)
		}
		codes := []Code{deviceRead, read, manage}
		if scope == "project" {
			codes = append(codes, ProjectDeviceOutputUse)
		}
		add(scope, "output", "channel-candidates", "GET", base+"/device-outputs/:output_id/channel-candidates", codes...)
		historyCodes := []Code{deviceRead, deviceHistory, read}
		if history != read {
			historyCodes = append(historyCodes, history)
		}
		add(scope, "output", "source-history", "GET", base+"/device-outputs/:output_id/source-history", historyCodes...)
	}
	for _, v := range []struct{ action, method, path string }{{"detail", "GET", ""}, {"candidates", "GET", "/candidates"}, {"history", "GET", "/changes"}, {"preview", "POST", "/preview"}, {"change", "POST", "/changes"}} {
		codes := []Code{TenantDeviceAuthorizationRead, TenantDeviceRead, TenantProjectRead}
		if v.method == "POST" {
			codes = append(codes, TenantDeviceAuthorizationManage)
		}
		add("tenant", "access", v.action, v.method, j+"/device-access"+v.path, codes...)
	}
	for _, v := range []struct{ scope, base string }{{"platform", "/api/v1/platform"}, {"tenant", t}, {"project", j}} {
		add(v.scope, "request", "detail", "GET", v.base+"/device-requests/:request_id")
	}
	return out
}
