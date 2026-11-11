package access

const (
	EngineeringTypeRead              Code = "engineering.type.read"
	EngineeringTypeEdit              Code = "engineering.type.edit"
	EngineeringTypePublish           Code = "engineering.type.publish"
	EngineeringTypeState             Code = "engineering.type.state"
	EngineeringMetricRead            Code = "engineering.metric.read"
	EngineeringMetricEdit            Code = "engineering.metric.edit"
	EngineeringMetricPublish         Code = "engineering.metric.publish"
	EngineeringMetricState           Code = "engineering.metric.state"
	EngineeringTemplateRead          Code = "engineering.template.read"
	EngineeringTemplateEdit          Code = "engineering.template.edit"
	EngineeringTemplatePublish       Code = "engineering.template.publish"
	EngineeringTemplateState         Code = "engineering.template.state"
	TenantEngineeringTemplateRead    Code = "tenant.engineering.template.read"
	TenantEngineeringTemplateEdit    Code = "tenant.engineering.template.edit"
	TenantEngineeringTemplatePublish Code = "tenant.engineering.template.publish"
	TenantEngineeringTemplateState   Code = "tenant.engineering.template.state"
	TenantEngineeringCatalogRead     Code = "tenant.engineering.catalog.read"
)

func engineeringCatalog() []Permission {
	return []Permission{
		{Code: EngineeringTypeRead, Name: "查看设施类型定义", Group: "工程配置", Scope: "platform", Description: "查看设施类型定义；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTypeEdit, Name: "编辑设施类型定义", Group: "工程配置", Scope: "platform", Description: "编辑设施类型定义；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTypePublish, Name: "发布设施类型定义", Group: "工程配置", Scope: "platform", Description: "发布设施类型定义；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTypeState, Name: "停用恢复设施类型定义", Group: "工程配置", Scope: "platform", Description: "停用恢复设施类型定义；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringMetricRead, Name: "查看指标与单位", Group: "工程配置", Scope: "platform", Description: "查看指标与单位；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringMetricEdit, Name: "编辑指标与单位", Group: "工程配置", Scope: "platform", Description: "编辑指标与单位；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringMetricPublish, Name: "发布指标与单位", Group: "工程配置", Scope: "platform", Description: "发布指标与单位；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringMetricState, Name: "停用恢复指标与单位", Group: "工程配置", Scope: "platform", Description: "停用恢复指标与单位；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTemplateRead, Name: "查看公共监测模板", Group: "工程配置", Scope: "platform", Description: "查看公共监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTemplateEdit, Name: "编辑公共监测模板", Group: "工程配置", Scope: "platform", Description: "编辑公共监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTemplatePublish, Name: "发布公共监测模板", Group: "工程配置", Scope: "platform", Description: "发布公共监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: EngineeringTemplateState, Name: "停用恢复公共监测模板", Group: "工程配置", Scope: "platform", Description: "停用恢复公共监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false},
		{Code: TenantEngineeringTemplateRead, Name: "查看本租户监测模板", Group: "工程配置", Scope: "tenant", Description: "查看本租户监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: true},
		{Code: TenantEngineeringTemplateEdit, Name: "编辑本租户监测模板", Group: "工程配置", Scope: "tenant", Description: "编辑本租户监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true},
		{Code: TenantEngineeringTemplatePublish, Name: "发布本租户监测模板", Group: "工程配置", Scope: "tenant", Description: "发布本租户监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true},
		{Code: TenantEngineeringTemplateState, Name: "停用恢复本租户监测模板", Group: "工程配置", Scope: "tenant", Description: "停用恢复本租户监测模板；变更还需查看权限，按当前作用范围与依赖状态校验。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true},
		{Code: TenantEngineeringCatalogRead, Name: "查看公共工程目录", Group: "工程配置", Scope: "tenant", Description: "在当前有效租户内选用可用公共工程定义。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: true},
	}
}

func Catalog() []Permission {
	return append(append(projectCatalog(), engineeringCatalog()...), resourceCatalog()...)
}
