package access

import "strings"

const (
	IngestionSourceRead             Code = "ingestion.source.read"
	IngestionSourceManage           Code = "ingestion.source.manage"
	IngestionQuarantineRead         Code = "ingestion.quarantine.read"
	IngestionQuarantineManage       Code = "ingestion.quarantine.manage"
	TenantIngestionSourceRead       Code = "tenant.ingestion.source.read"
	TenantIngestionSourceConfirm    Code = "tenant.ingestion.source.confirm"
	TenantIngestionTerminalRead     Code = "tenant.ingestion.terminal.read"
	TenantIngestionTerminalManage   Code = "tenant.ingestion.terminal.manage"
	TenantIngestionExecutionManage  Code = "tenant.ingestion.execution.manage"
	TenantIngestionUnmatchedRead    Code = "tenant.ingestion.unmatched.read"
	TenantIngestionUnmatchedProcess Code = "tenant.ingestion.unmatched.process"
	TenantIngestionMessageRead      Code = "tenant.ingestion.message.read"
	TenantIngestionRawRead          Code = "tenant.ingestion.raw.read"
	TenantIngestionReprocess        Code = "tenant.ingestion.reprocess"
)

func ingestionCatalog() []Permission {
	out := []Permission{}
	for _, v := range []struct {
		code Code
		name string
	}{{IngestionSourceRead, "查看接入入口"}, {IngestionSourceManage, "维护接入入口"}, {IngestionQuarantineRead, "查看接入隔离元数据"}, {IngestionQuarantineManage, "接入传输对账"}, {TenantIngestionSourceRead, "查看获配接入入口"}, {TenantIngestionSourceConfirm, "确认接入入口修订"}, {TenantIngestionTerminalRead, "查看接入终端"}, {TenantIngestionTerminalManage, "维护终端与设备关联"}, {TenantIngestionExecutionManage, "管理接入机器执行授权"}, {TenantIngestionUnmatchedRead, "查看未匹配报文元数据"}, {TenantIngestionUnmatchedProcess, "处理未匹配报文"}, {TenantIngestionMessageRead, "查看已核验报文"}, {TenantIngestionRawRead, "读取已核验单元原文"}, {TenantIngestionReprocess, "重新处理已核验报文"}} {
		scope := "platform"
		if strings.HasPrefix(string(v.code), "tenant.") {
			scope = "tenant"
		}
		risk := "high"
		if strings.HasSuffix(string(v.code), ".read") && v.code != TenantIngestionRawRead {
			risk = "normal"
		}
		out = append(out, Permission{Code: v.code, Name: v.name, Group: "数据接入", Scope: scope, Description: "限当前入口、来源项目和设备资格；管理入口不授予正文读取或设备解析采用权限。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant"})
	}
	return out
}
func ingestionCodes(scope string) []Code {
	out := []Code{}
	for _, p := range ingestionCatalog() {
		if p.Scope == scope {
			out = append(out, p.Code)
		}
	}
	return out
}
func IngestionOperation(scope, kind, action string) Operation {
	return Operation("Ingestion:" + scope + ":" + kind + ":" + action)
}
func ingestionPolicies() []Policy {
	out := []Policy{}
	add := func(scope, kind, action, verb, path string, codes ...Code) {
		class := scope
		if scope == "platform" {
			class = "permission"
		}
		out = append(out, Policy{Operation: IngestionOperation(scope, kind, action), Method: verb, Path: path, Class: class, Permissions: codes})
	}
	p := "/api/v1/platform/ingestion"
	for _, route := range []struct {
		kind, action, verb, path string
		codes                    []Code
	}{
		{"source-options", "list", "GET", "/source-options", []Code{IngestionSourceRead, IngestionSourceManage}},
		{"relays", "list", "GET", "/relays", []Code{IngestionSourceRead}},
		{"relays", "create", "POST", "/relays", []Code{IngestionSourceRead, IngestionSourceManage}},
		{"sources", "list", "GET", "/sources", []Code{IngestionSourceRead}},
		{"sources", "create", "POST", "/sources", []Code{IngestionSourceRead, IngestionSourceManage}},
		{"sources", "detail", "GET", "/sources/:source_id", []Code{IngestionSourceRead}},
		{"sources", "revision", "POST", "/sources/:source_id/revisions", []Code{IngestionSourceRead, IngestionSourceManage}},
		{"sources", "commands", "POST", "/sources/:source_id/commands", []Code{IngestionSourceRead, IngestionSourceManage}},
		{"integrity-events", "list", "GET", "/integrity-events", []Code{IngestionQuarantineRead}},
		{"quarantine", "list", "GET", "/quarantine", []Code{IngestionQuarantineRead}},
		{"recovery", "detail", "GET", "/recovery-operations/:recovery_id/generations/:generation", []Code{IngestionQuarantineRead}},
		{"recovery", "list", "GET", "/recovery-operations", []Code{IngestionQuarantineRead}},
		{"recovery", "preview", "POST", "/recovery-operations/preview", []Code{IngestionQuarantineRead, IngestionQuarantineManage}},
		{"recovery", "create", "POST", "/recovery-operations", []Code{IngestionQuarantineRead, IngestionQuarantineManage}},
	} {
		add("platform", route.kind, route.action, route.verb, p+route.path, route.codes...)
	}
	t := "/api/v1/tenants/:tenant_id/ingestion"
	for _, route := range []struct {
		kind, action, verb, path string
		codes                    []Code
	}{
		{"sources", "list", "GET", "/sources", []Code{TenantIngestionSourceRead, TenantProjectRead}},
		{"sources", "detail", "GET", "/sources/:source_id", []Code{TenantIngestionSourceRead, TenantProjectRead}},
		{"sources", "confirm", "POST", "/sources/:source_id/confirm", []Code{TenantIngestionSourceRead, TenantIngestionSourceConfirm, TenantProjectRead}},
		{"terminals", "list", "GET", "/terminals", []Code{TenantIngestionTerminalRead, TenantDeviceRead}},
		{"terminals", "create", "POST", "/terminals", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage, TenantDeviceRead, TenantIngestionSourceRead}},
		{"sources", "planning", "POST", "/sources/:source_id/planning", []Code{TenantIngestionSourceRead, TenantIngestionExecutionManage}},
		{"terminals", "revision", "POST", "/terminals/:terminal_id/revisions", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage, TenantDeviceRead}},
		{"terminals", "history", "GET", "/terminals/:terminal_id/revisions", []Code{TenantIngestionTerminalRead, TenantDeviceRead}},
		{"terminals", "detail", "GET", "/terminals/:terminal_id", []Code{TenantIngestionTerminalRead, TenantDeviceRead}},
		{"associations", "list", "GET", "/terminals/:terminal_id/associations", []Code{TenantIngestionTerminalRead, TenantDeviceRead}},
		{"associations", "preview", "POST", "/terminals/:terminal_id/associations/preview", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage, TenantDeviceRead}},
		{"associations", "create", "POST", "/terminals/:terminal_id/associations", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage, TenantDeviceRead}},
		{"associations", "state", "POST", "/associations/:association_id/state", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage, TenantDeviceRead}},
		{"changes", "cancel", "POST", "/changes/:change_id/cancel", []Code{TenantIngestionTerminalRead, TenantIngestionTerminalManage}},
		{"execution-grants", "list", "GET", "/execution-grants", []Code{TenantIngestionSourceRead}},
		{"execution-grants", "save", "POST", "/execution-grants", []Code{TenantIngestionSourceRead, TenantIngestionExecutionManage}},
		{"unmatched", "list", "GET", "/unmatched", []Code{TenantIngestionUnmatchedRead, TenantIngestionSourceRead}},
		{"unmatched", "close", "POST", "/unmatched/:receipt_id/close", []Code{TenantIngestionUnmatchedRead, TenantIngestionUnmatchedProcess}},
		{"messages", "list", "GET", "/messages", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead}},
		{"messages", "detail", "GET", "/messages/:message_id", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead}},
		{"messages", "content", "GET", "/messages/:message_id/content", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead, TenantIngestionRawRead}},
		{"messages", "decisions", "GET", "/messages/:message_id/decisions", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead}},
		{"messages", "results", "GET", "/messages/:message_id/results", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead, TenantParsingRunRead}},
		{"messages", "response", "GET", "/messages/:message_id/results/:result_id/content", []Code{TenantIngestionMessageRead, TenantDeviceRead, TenantIngestionTerminalRead, TenantParsingRunRead, TenantIngestionRawRead}},
		{"recovery", "detail", "GET", "/recovery-operations/:recovery_id/generations/:generation", []Code{TenantIngestionSourceRead}},
		{"recovery", "list", "GET", "/recovery-operations", []Code{TenantIngestionSourceRead}},
		{"recovery", "preview", "POST", "/recovery-operations/preview", []Code{TenantIngestionSourceRead}},
		{"recovery", "create", "POST", "/recovery-operations", []Code{TenantIngestionSourceRead}},
		{"operations", "list", "GET", "/operations", []Code{TenantIngestionSourceRead}},
	} {
		add("tenant", route.kind, route.action, route.verb, t+route.path, route.codes...)
	}
	add("project", "device", "detail", "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/devices/:device_id/ingestion", ProjectDeviceRead)
	add("tenant", "device", "detail", "GET", "/api/v1/tenants/:tenant_id/devices/:device_id/ingestion", TenantDeviceRead)
	return out
}
