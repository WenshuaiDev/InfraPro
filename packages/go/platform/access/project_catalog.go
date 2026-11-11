package access

const (
	TenantProjectRead            Code = "tenant.project.read"
	TenantProjectCreate          Code = "tenant.project.create"
	TenantProjectRecoverAdmin    Code = "tenant.project.recover_admin"
	ProjectProfileRead           Code = "project.profile.read"
	ProjectProfileUpdate         Code = "project.profile.update"
	ProjectMemberRead            Code = "project.member.read"
	ProjectMemberAdd             Code = "project.member.add"
	ProjectMemberSuspend         Code = "project.member.suspend"
	ProjectMemberResume          Code = "project.member.resume"
	ProjectMemberRemove          Code = "project.member.remove"
	ProjectRoleRead              Code = "project.role.read"
	ProjectPermissionRead        Code = "project.permission.read"
	ProjectAuthorizationRead     Code = "project.authorization.read"
	ProjectAuditRead             Code = "project.audit.read"
	ProjectRoleCreate            Code = "project.role.create"
	ProjectRoleUpdate            Code = "project.role.update"
	ProjectRoleSetPermissions    Code = "project.role.set_permissions"
	ProjectRoleSetStatus         Code = "project.role.set_status"
	ProjectRoleDelete            Code = "project.role.delete"
	ProjectMemberAssignRoles     Code = "project.member.assign_roles"
	ProjectAdministratorTransfer Code = "project.administrator.transfer"
	ProjectOwnerChange           Code = "project.owner.change"
	ProjectArchive               Code = "project.archive"
	ProjectRestore               Code = "project.restore"
)

func projectCatalog() []Permission {
	return append(baseCatalog(), []Permission{
		{Code: TenantProjectRead, Name: "查看租户项目治理摘要", Group: "项目管理", Scope: "tenant", Description: "查看租户项目治理摘要；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: true, ProjectCustomRoleAssignable: false},
		{Code: TenantProjectCreate, Name: "创建项目", Group: "项目管理", Scope: "tenant", Description: "创建项目；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: true, ProjectCustomRoleAssignable: false},
		{Code: TenantProjectRecoverAdmin, Name: "修复项目管理员", Group: "项目管理", Scope: "tenant", Description: "修复项目管理员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectProfileRead, Name: "查看项目资料", Group: "项目管理", Scope: "project", Description: "查看项目资料；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectProfileUpdate, Name: "编辑项目资料", Group: "项目管理", Scope: "project", Description: "编辑项目资料；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectMemberRead, Name: "查看项目成员", Group: "项目管理", Scope: "project", Description: "查看项目成员；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectMemberAdd, Name: "添加项目成员", Group: "项目管理", Scope: "project", Description: "添加项目成员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectMemberSuspend, Name: "暂停项目成员", Group: "项目管理", Scope: "project", Description: "暂停项目成员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectMemberResume, Name: "恢复项目成员", Group: "项目管理", Scope: "project", Description: "恢复项目成员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectMemberRemove, Name: "移除项目成员", Group: "项目管理", Scope: "project", Description: "移除项目成员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectRoleRead, Name: "查看项目角色", Group: "项目管理", Scope: "project", Description: "查看项目角色；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectPermissionRead, Name: "查看项目权限目录", Group: "项目管理", Scope: "project", Description: "查看项目权限目录；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectAuthorizationRead, Name: "查看项目授权解释", Group: "项目管理", Scope: "project", Description: "查看项目授权解释；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectAuditRead, Name: "查看项目操作记录", Group: "项目管理", Scope: "project", Description: "查看项目操作记录；仍需满足范围、配套读取权限及当前状态。", Risk: "normal", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: true},
		{Code: ProjectRoleCreate, Name: "创建项目角色", Group: "项目管理", Scope: "project", Description: "创建项目角色；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectRoleUpdate, Name: "编辑项目角色", Group: "项目管理", Scope: "project", Description: "编辑项目角色；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectRoleSetPermissions, Name: "配置项目角色权限", Group: "项目管理", Scope: "project", Description: "配置项目角色权限；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectRoleSetStatus, Name: "启停项目角色", Group: "项目管理", Scope: "project", Description: "启停项目角色；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectRoleDelete, Name: "删除项目角色", Group: "项目管理", Scope: "project", Description: "删除项目角色；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectMemberAssignRoles, Name: "配置项目成员角色", Group: "项目管理", Scope: "project", Description: "配置项目成员角色；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectAdministratorTransfer, Name: "交接项目管理员", Group: "项目管理", Scope: "project", Description: "交接项目管理员；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectOwnerChange, Name: "变更项目负责人", Group: "项目管理", Scope: "project", Description: "变更项目负责人；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectArchive, Name: "归档项目", Group: "项目管理", Scope: "project", Description: "归档项目；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
		{Code: ProjectRestore, Name: "恢复项目", Group: "项目管理", Scope: "project", Description: "恢复项目；仍需满足范围、配套读取权限及当前状态。", Risk: "high", Lifecycle: "active", TenantCustomRoleAssignable: false, ProjectCustomRoleAssignable: false},
	}...)
}
