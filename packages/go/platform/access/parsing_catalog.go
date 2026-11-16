package access

import "strings"

const (
	ParsingMethodRead                Code = "parsing.method.read"
	ParsingMethodManage              Code = "parsing.method.manage"
	DeviceProductParsingManage       Code = "device.product.parsing.manage"
	ParsingTestExecute               Code = "parsing.test.execute"
	ParsingRunRead                   Code = "parsing.run.read"
	ParsingSampleRead                Code = "parsing.sample.read"
	TenantParsingMethodRead          Code = "tenant.parsing.method.read"
	TenantDeviceProductParsingManage Code = "tenant.device.product.parsing.manage"
	TenantDeviceParsingManage        Code = "tenant.device.parsing.manage"
	TenantParsingTestExecute         Code = "tenant.parsing.test.execute"
	TenantParsingRunRead             Code = "tenant.parsing.run.read"
	TenantParsingSampleRead          Code = "tenant.parsing.sample.read"
)

func parsingCatalog() []Permission {
	out := []Permission{}
	for _, v := range []struct {
		code Code
		name string
	}{{ParsingMethodRead, "查看解析方法"}, {ParsingMethodManage, "管理解析方法版本"}, {DeviceProductParsingManage, "维护公共产品解析配置"}, {ParsingTestExecute, "执行平台试解析"}, {ParsingRunRead, "查看平台解析运行"}, {ParsingSampleRead, "读取平台解析原文"}, {TenantParsingMethodRead, "查看可用解析方法"}, {TenantDeviceProductParsingManage, "维护私有产品解析配置"}, {TenantDeviceParsingManage, "管理设备解析采用"}, {TenantParsingTestExecute, "执行租户试解析"}, {TenantParsingRunRead, "查看租户解析运行"}, {TenantParsingSampleRead, "读取租户解析原文"}} {
		scope := "platform"
		if strings.HasPrefix(string(v.code), "tenant.") {
			scope = "tenant"
		}
		risk := "normal"
		if !strings.HasSuffix(string(v.code), ".read") || strings.Contains(string(v.code), "sample") {
			risk = "high"
		}
		out = append(out, Permission{Code: v.code, Name: v.name, Group: "解析服务", Scope: scope, Description: "仅在当前授权范围操作固定版本、配置及证据；原文读取单独授权，试解析不产生正式观测。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant"})
	}
	return out
}
func parsingCodes(scope string) []Code {
	out := []Code{}
	for _, p := range parsingCatalog() {
		if p.Scope == scope {
			out = append(out, p.Code)
		}
	}
	return out
}
func ParsingOperation(scope, kind, action string) Operation {
	return Operation("Parsing:" + scope + ":" + kind + ":" + action)
}
func parsingPolicies() []Policy {
	out := []Policy{}
	add := func(scope, kind, action, method, path string, codes ...Code) {
		class := scope
		if scope == "platform" {
			class = "permission"
		}
		out = append(out, Policy{Operation: ParsingOperation(scope, kind, action), Method: method, Path: path, Class: class, Permissions: codes})
	}
	for _, entry := range []string{"platform", "tenant"} {
		base := "/api/v1/platform"
		method, execute, run, sample, product, manage, edit := ParsingMethodRead, ParsingTestExecute, ParsingRunRead, ParsingSampleRead, DeviceDefinitionRead, DeviceProductParsingManage, DeviceProductEdit
		if entry == "tenant" {
			base = "/api/v1/tenants/:tenant_id"
			method, execute, run, sample, product, manage, edit = TenantParsingMethodRead, TenantParsingTestExecute, TenantParsingRunRead, TenantParsingSampleRead, TenantDeviceDefinitionRead, TenantDeviceProductParsingManage, TenantDeviceProductEdit
		}
		add(entry, "methods", "list", "GET", base+"/parsing/methods", method)
		if entry == "platform" {
			add(entry, "catalog", "status", "GET", base+"/parsing/catalog-status", method)
			add(entry, "catalog", "sync", "POST", base+"/parsing/catalog-syncs", method, ParsingMethodManage)
			add(entry, "catalog", "detail", "GET", base+"/parsing/catalog-syncs/:sync_id", method)
			add(entry, "methods", "state", "POST", base+"/parsing/methods/:method_id/versions/:version/state", method, ParsingMethodManage)
		}
		p := base + "/device-definitions/products/:id/versions/:version_id"
		for _, route := range []struct{ kind, action, verb, suffix string }{{"configs", "list", "GET", "/parsing-configs"}, {"configs", "create", "POST", "/parsing-configs"}, {"configs", "revision", "POST", "/parsing-configs/:config_id/revisions"}, {"configs", "state", "POST", "/parsing-configs/:config_id/state"}, {"defaults", "detail", "GET", "/parsing-default"}, {"defaults", "set", "PUT", "/parsing-default"}} {
			codes := []Code{product}
			if route.verb != "GET" {
				codes = append(codes, method, manage, edit)
			}
			add(entry, route.kind, route.action, route.verb, p+route.suffix, codes...)
		}
		for _, route := range []struct {
			kind, action, verb, path string
			codes                    []Code
		}{
			{"samples", "create", "POST", "/samples", []Code{method, execute}},
			{"samples", "content", "PUT", "/samples/:sample_id/content", []Code{method, execute}},
			{"samples", "confirm", "POST", "/samples/:sample_id/confirm", []Code{method, execute}},
			{"samples", "list", "GET", "/samples", []Code{run}},
			{"samples", "detail", "GET", "/samples/:sample_id", []Code{run}},
			{"samples", "download", "GET", "/samples/:sample_id/content", []Code{run, sample}},
			{"runs", "create", "POST", "/runs", []Code{method, execute}},
			{"runs", "list", "GET", "/runs", []Code{run}},
			{"runs", "detail", "GET", "/runs/:run_id", []Code{run}},
			{"runs", "execution", "GET", "/runs/:run_id/executions/:generation", []Code{run}},
			{"runs", "result", "GET", "/runs/:run_id/executions/:generation/result", []Code{run}},
			{"runs", "response", "GET", "/runs/:run_id/executions/:generation/response", []Code{run, sample}},
			{"runs", "compare", "POST", "/runs/compare", []Code{method, execute, run}},
		} {
			add(entry, route.kind, route.action, route.verb, base+"/parsing"+route.path, route.codes...)
		}
	}
	t := "/api/v1/tenants/:tenant_id"
	d := t + "/devices/:device_id/parsing"
	add("tenant", "public-configs", "list", "GET", t+"/device-catalog/products/:id/versions/:version_id/parsing-configs", TenantDeviceCatalogRead)
	add("tenant", "public-defaults", "detail", "GET", t+"/device-catalog/products/:id/versions/:version_id/parsing-default", TenantDeviceCatalogRead)
	for _, route := range []struct{ kind, action, verb, path string }{{"device", "detail", "GET", ""}, {"device", "history", "GET", "/history"}, {"device-configs", "list", "GET", "/configs"}, {"device-configs", "create", "POST", "/configs"}, {"device-configs", "revision", "POST", "/configs/:config_id/revisions"}, {"device", "preview", "POST", "/preview"}, {"device", "commands", "POST", "/commands"}} {
		codes := []Code{TenantDeviceRead}
		if route.action == "history" {
			codes = append(codes, TenantDeviceHistoryRead)
		}
		if route.verb == "POST" {
			codes = append(codes, TenantDeviceParsingManage, TenantParsingMethodRead)
		}
		add("tenant", route.kind, route.action, route.verb, d+route.path, codes...)
	}
	for _, action := range []string{"preview", "commands"} {
		add("tenant", "batch", action, "POST", t+"/parsing/device-adoptions/"+action, TenantDeviceRead, TenantDeviceParsingManage, TenantParsingMethodRead)
	}
	for _, action := range []string{"detail", "history"} {
		suffix := ""
		codes := []Code{ProjectDeviceRead}
		if action == "history" {
			suffix = "/history"
			codes = append(codes, ProjectDeviceHistoryRead)
		}
		add("project", "device", action, "GET", t+"/projects/:project_id/devices/:device_id/parsing"+suffix, codes...)
	}
	return out
}
