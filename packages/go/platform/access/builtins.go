package access

const AdminKey = "platform_admin"
const MemberKey = "member"
const TenantAdminKey = "tenant_admin"
const TenantMemberKey = "tenant_member"

type Builtin struct {
	Key, Name, Description, BindingScope string
	Permissions                          []Code
	AssignmentPolicy                     string
}

func baseBuiltins() []Builtin {
	// Scope is an explicit catalog property; future business permissions are not included.
	admin := []Code{}
	for _, p := range Catalog() {
		if p.Lifecycle == "active" && (p.Scope == "self" || p.Scope == "platform") {
			admin = append(admin, p.Code)
		}
	}
	return []Builtin{
		{AdminKey, "平台管理员", "平台管理及本人账号能力，由系统维护。", "platform", admin, "manual"},
		{MemberKey, "基础成员", "本人资料、密码、会话及安全记录，由系统维护。", "platform", []Code{ProfileRead, ProfileUpdate, PasswordUpdate, SessionRead, SessionRevoke, SecurityEventRead}, "manual"},
		{TenantAdminKey, "租户管理员", "管理所属租户资料、成员、邀请和操作记录。", "tenant", []Code{TenantEngineeringResourceAuthorizationRead, TenantEngineeringResourceAuthorizationManage, TenantEngineeringResourceRead, TenantEngineeringResourceManage, TenantEngineeringCatalogRead, TenantEngineeringTemplateRead, TenantEngineeringTemplateEdit, TenantEngineeringTemplatePublish, TenantEngineeringTemplateState, TenantProjectRead, TenantProjectCreate, TenantProjectRecoverAdmin, TenantProfileRead, TenantProfileUpdate, TenantAuditRead, TenantMemberRead, TenantMemberSuspend, TenantMemberResume, TenantMemberRemove, TenantInvitationRead, TenantInvitationCreate, TenantInvitationRevoke, TenantInvitationRegenerate, TenantPermissionRead, TenantRoleRead, TenantRoleCreate, TenantRoleUpdate, TenantRoleSetPermissions, TenantRoleSetStatus, TenantRoleDelete, TenantMemberAssignRoles}, "administrator_transfer_only"},
		{TenantMemberKey, "租户普通成员", "可查看所属租户基本资料。", "tenant", []Code{TenantProfileRead}, "manual"},
		{"tenant_profile_maintainer", "租户资料维护员", "查看及修改本租户资料。", "tenant", []Code{TenantProfileRead, TenantProfileUpdate}, "manual"},
		{"tenant_member_manager", "租户成员管理人员", "查看和管理权限覆盖范围内的普通成员。", "tenant", []Code{TenantProfileRead, TenantMemberRead, TenantMemberSuspend, TenantMemberResume, TenantMemberRemove}, "manual"},
		{"tenant_invitation_coordinator", "租户邀请协作员", "管理本租户普通成员邀请。", "tenant", []Code{TenantProfileRead, TenantInvitationRead, TenantInvitationCreate, TenantInvitationRevoke, TenantInvitationRegenerate}, "manual"},
		{"tenant_auditor", "租户审计查看员", "查看本租户资料及操作记录。", "tenant", []Code{TenantProfileRead, TenantAuditRead}, "manual"},
	}
}
func ManagementCodes() []Code { return Builtins()[0].Permissions }

const ProjectAdminKey = "project_admin"
const ProjectMemberKey = "project_member"

func Builtins() []Builtin {
	return append(baseBuiltins(), []Builtin{
		{ProjectAdminKey, "项目管理员", "仅通过创建、交接和失管恢复任命。", "project", append(projectResourceCodes(), []Code{ProjectProfileRead, ProjectProfileUpdate, ProjectMemberRead, ProjectMemberAdd, ProjectMemberSuspend, ProjectMemberResume, ProjectMemberRemove, ProjectRoleRead, ProjectPermissionRead, ProjectAuthorizationRead, ProjectAuditRead, ProjectRoleCreate, ProjectRoleUpdate, ProjectRoleSetPermissions, ProjectRoleSetStatus, ProjectRoleDelete, ProjectMemberAssignRoles, ProjectAdministratorTransfer, ProjectOwnerChange, ProjectArchive, ProjectRestore}...), "administrator_transfer_only"},
		{"project_resource_reader", "项目资源查看员", "查看获准资源及公共历史。", "project", []Code{ProjectProfileRead, ProjectEngineeringResourceRead, ProjectEngineeringResourceHistoryRead}, "manual"},
		{"project_resource_maintainer", "项目资源维护员", "在租户明确授权范围内创建和维护统一资源。", "project", append(projectResourceCodes(), ProjectProfileRead), "manual"},
		{ProjectMemberKey, "项目普通成员", "查看项目资料。", "project", []Code{ProjectProfileRead}, "manual"},
		{"project_profile_maintainer", "项目资料维护员", "查看并维护项目资料。", "project", []Code{ProjectProfileRead, ProjectProfileUpdate}, "manual"},
		{"project_member_manager", "项目成员管理人员", "在授予范围内管理普通成员。", "project", []Code{ProjectProfileRead, ProjectMemberRead, ProjectMemberAdd, ProjectMemberSuspend, ProjectMemberResume, ProjectMemberRemove, ProjectAuthorizationRead}, "manual"},
		{"project_auditor", "项目审计查看员", "查看项目授权及操作记录。", "project", []Code{ProjectProfileRead, ProjectMemberRead, ProjectRoleRead, ProjectPermissionRead, ProjectAuthorizationRead, ProjectAuditRead}, "manual"},
	}...)
}
