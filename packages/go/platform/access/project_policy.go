package access

const (
	OpMyProjects                     Operation = "MyProjects"
	OpProjectGovernanceList          Operation = "ProjectGovernanceList"
	OpProjectGovernanceDetail        Operation = "ProjectGovernanceDetail"
	OpProjectCreate                  Operation = "ProjectCreate"
	OpProjectCreateCandidates        Operation = "ProjectCreateCandidates"
	OpProjectRecoveryCandidates      Operation = "ProjectRecoveryCandidates"
	OpProjectRecoverAdminPreview     Operation = "ProjectRecoverAdminPreview"
	OpProjectRecoverAdmin            Operation = "ProjectRecoverAdmin"
	OpProjectContext                 Operation = "ProjectContext"
	OpProjectAvailability            Operation = "ProjectAvailability"
	OpProjectProfile                 Operation = "ProjectProfile"
	OpProjectMembers                 Operation = "ProjectMembers"
	OpProjectMember                  Operation = "ProjectMember"
	OpProjectAuthorization           Operation = "ProjectAuthorization"
	OpProjectRoles                   Operation = "ProjectRoles"
	OpProjectRole                    Operation = "ProjectRole"
	OpProjectRoleMembers             Operation = "ProjectRoleMembers"
	OpProjectPermissions             Operation = "ProjectPermissions"
	OpProjectAudit                   Operation = "ProjectAudit"
	OpProjectUpdate                  Operation = "ProjectUpdate"
	OpProjectMemberCandidates        Operation = "ProjectMemberCandidates"
	OpProjectOwnerCandidates         Operation = "ProjectOwnerCandidates"
	OpProjectAdministratorCandidates Operation = "ProjectAdministratorCandidates"
	OpProjectMemberAdd               Operation = "ProjectMemberAdd"
	OpProjectMemberSuspendPreview    Operation = "ProjectMemberSuspendPreview"
	OpProjectMemberSuspend           Operation = "ProjectMemberSuspend"
	OpProjectMemberResumePreview     Operation = "ProjectMemberResumePreview"
	OpProjectMemberResume            Operation = "ProjectMemberResume"
	OpProjectMemberRemovePreview     Operation = "ProjectMemberRemovePreview"
	OpProjectMemberRemove            Operation = "ProjectMemberRemove"
	OpProjectOwnerChangePreview      Operation = "ProjectOwnerChangePreview"
	OpProjectOwnerChange             Operation = "ProjectOwnerChange"
	OpProjectTransferPreview         Operation = "ProjectTransferPreview"
	OpProjectTransfer                Operation = "ProjectTransfer"
	OpProjectArchivePreview          Operation = "ProjectArchivePreview"
	OpProjectArchive                 Operation = "ProjectArchive"
	OpProjectRestorePreview          Operation = "ProjectRestorePreview"
	OpProjectRestore                 Operation = "ProjectRestore"
	OpProjectRolePermissionsPreview  Operation = "ProjectRolePermissionsPreview"
	OpProjectRolePermissions         Operation = "ProjectRolePermissions"
	OpProjectRoleStatusPreview       Operation = "ProjectRoleStatusPreview"
	OpProjectRoleStatus              Operation = "ProjectRoleStatus"
	OpProjectRoleDeletePreview       Operation = "ProjectRoleDeletePreview"
	OpProjectRoleDelete              Operation = "ProjectRoleDelete"
	OpProjectMemberRolesPreview      Operation = "ProjectMemberRolesPreview"
	OpProjectMemberRoles             Operation = "ProjectMemberRoles"
	OpProjectRoleCreate              Operation = "ProjectRoleCreate"
	OpProjectRoleCopy                Operation = "ProjectRoleCopy"
	OpProjectRoleUpdate              Operation = "ProjectRoleUpdate"
	OpProjectPresetList              Operation = "ProjectPresetList"
	OpProjectPresetDetail            Operation = "ProjectPresetDetail"
)

func projectPolicies() []Policy {
	return []Policy{
		{OpMyProjects, "GET", "/api/v1/me/projects", "authenticated", []Code{}, false, false},
		{OpProjectGovernanceList, "GET", "/api/v1/tenants/:tenant_id/project-governance", "tenant", []Code{TenantProjectRead}, false, false},
		{OpProjectGovernanceDetail, "GET", "/api/v1/tenants/:tenant_id/project-governance/:project_id", "tenant", []Code{TenantProjectRead}, false, false},
		{OpProjectCreate, "POST", "/api/v1/tenants/:tenant_id/projects", "tenant", []Code{TenantProjectRead, TenantProjectCreate}, false, true},
		{OpProjectCreateCandidates, "GET", "/api/v1/tenants/:tenant_id/project-candidates", "tenant", []Code{TenantProjectRead, TenantProjectCreate}, false, false},
		{OpProjectRecoveryCandidates, "GET", "/api/v1/tenants/:tenant_id/project-governance/:project_id/administrator-candidates", "tenant", []Code{TenantProjectRead, TenantProjectRecoverAdmin}, false, false},
		{OpProjectRecoverAdminPreview, "POST", "/api/v1/tenants/:tenant_id/project-governance/:project_id/administrator-recovery/preview", "tenant", []Code{TenantProjectRead, TenantProjectRecoverAdmin}, false, false},
		{OpProjectRecoverAdmin, "POST", "/api/v1/tenants/:tenant_id/project-governance/:project_id/administrator-recovery", "tenant", []Code{TenantProjectRead, TenantProjectRecoverAdmin}, true, true},
		{OpProjectContext, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/context", "project_context", []Code{}, false, false},
		{OpProjectAvailability, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/availability", "project_context", []Code{}, false, false},
		{OpProjectProfile, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/profile", "project", []Code{ProjectProfileRead}, false, false},
		{OpProjectMembers, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/members", "project", []Code{ProjectMemberRead}, false, false},
		{OpProjectMember, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id", "project", []Code{ProjectMemberRead}, false, false},
		{OpProjectAuthorization, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/authorization", "project", []Code{ProjectMemberRead, ProjectAuthorizationRead}, false, false},
		{OpProjectRoles, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/roles", "project", []Code{ProjectRoleRead}, false, false},
		{OpProjectRole, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id", "project", []Code{ProjectRoleRead}, false, false},
		{OpProjectRoleMembers, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/members", "project", []Code{ProjectRoleRead, ProjectMemberRead}, false, false},
		{OpProjectPermissions, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/permissions", "project", []Code{ProjectPermissionRead}, false, false},
		{OpProjectAudit, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/audit", "project", []Code{ProjectAuditRead}, false, false},
		{OpProjectUpdate, "PATCH", "/api/v1/tenants/:tenant_id/projects/:project_id/profile", "project", []Code{ProjectProfileRead, ProjectProfileUpdate}, false, false},
		{OpProjectMemberCandidates, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/member-candidates", "project", []Code{ProjectMemberRead, ProjectMemberAdd}, false, false},
		{OpProjectOwnerCandidates, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/owner-candidates", "project", []Code{ProjectProfileRead, ProjectOwnerChange}, false, false},
		{OpProjectAdministratorCandidates, "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/administrator-candidates", "project", []Code{ProjectProfileRead, ProjectMemberRead, ProjectAdministratorTransfer}, false, false},
		{OpProjectMemberAdd, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members", "project", []Code{ProjectMemberRead, ProjectMemberAdd}, false, true},
		{OpProjectMemberSuspendPreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/suspend/preview", "project", []Code{ProjectMemberRead, ProjectMemberSuspend}, false, false},
		{OpProjectMemberSuspend, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/suspend", "project", []Code{ProjectMemberRead, ProjectMemberSuspend}, false, true},
		{OpProjectMemberResumePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/resume/preview", "project", []Code{ProjectMemberRead, ProjectMemberResume}, false, false},
		{OpProjectMemberResume, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/resume", "project", []Code{ProjectMemberRead, ProjectMemberResume}, false, true},
		{OpProjectMemberRemovePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/remove/preview", "project", []Code{ProjectMemberRead, ProjectMemberRemove}, false, false},
		{OpProjectMemberRemove, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/remove", "project", []Code{ProjectMemberRead, ProjectMemberRemove}, true, true},
		{OpProjectOwnerChangePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/owner/preview", "project", []Code{ProjectProfileRead, ProjectOwnerChange}, false, false},
		{OpProjectOwnerChange, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/owner", "project", []Code{ProjectProfileRead, ProjectOwnerChange}, true, true},
		{OpProjectTransferPreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/administrator-transfer/preview", "project", []Code{ProjectProfileRead, ProjectMemberRead, ProjectAdministratorTransfer}, false, false},
		{OpProjectTransfer, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/administrator-transfer", "project", []Code{ProjectProfileRead, ProjectMemberRead, ProjectAdministratorTransfer}, true, true},
		{OpProjectArchivePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/archive/preview", "project", []Code{ProjectProfileRead, ProjectArchive}, false, false},
		{OpProjectArchive, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/archive", "project", []Code{ProjectProfileRead, ProjectArchive}, true, true},
		{OpProjectRestorePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/restore/preview", "project", []Code{ProjectProfileRead, ProjectRestore}, false, false},
		{OpProjectRestore, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/restore", "project", []Code{ProjectProfileRead, ProjectRestore}, true, true},
		{OpProjectRolePermissionsPreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/permissions/preview", "project", []Code{ProjectRoleRead, ProjectPermissionRead, ProjectRoleSetPermissions}, false, false},
		{OpProjectRolePermissions, "PUT", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/permissions", "project", []Code{ProjectRoleRead, ProjectPermissionRead, ProjectRoleSetPermissions}, true, true},
		{OpProjectRoleStatusPreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/status/preview", "project", []Code{ProjectRoleRead, ProjectRoleSetStatus}, false, false},
		{OpProjectRoleStatus, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/status", "project", []Code{ProjectRoleRead, ProjectRoleSetStatus}, true, true},
		{OpProjectRoleDeletePreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/delete/preview", "project", []Code{ProjectRoleRead, ProjectRoleDelete}, false, false},
		{OpProjectRoleDelete, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/delete", "project", []Code{ProjectRoleRead, ProjectRoleDelete}, true, true},
		{OpProjectMemberRolesPreview, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/roles/preview", "project", []Code{ProjectMemberRead, ProjectRoleRead, ProjectMemberAssignRoles}, false, false},
		{OpProjectMemberRoles, "PUT", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/roles", "project", []Code{ProjectMemberRead, ProjectRoleRead, ProjectMemberAssignRoles}, true, true},
		{OpProjectRoleCreate, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles", "project", []Code{ProjectRoleRead, ProjectPermissionRead, ProjectRoleCreate}, true, true},
		{OpProjectRoleCopy, "POST", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id/copy", "project", []Code{ProjectRoleRead, ProjectPermissionRead, ProjectRoleCreate}, true, true},
		{OpProjectRoleUpdate, "PATCH", "/api/v1/tenants/:tenant_id/projects/:project_id/roles/:role_id", "project", []Code{ProjectRoleRead, ProjectRoleUpdate}, false, true},
		{OpProjectPresetList, "GET", "/api/v1/access/project-role-presets", "permission", []Code{RoleRead}, false, false},
		{OpProjectPresetDetail, "GET", "/api/v1/access/project-role-presets/:role_id", "permission", []Code{RoleRead}, false, false},
	}
}
