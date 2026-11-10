package access

type Operation string
type Policy struct {
	Operation           Operation
	Method, Path, Class string
	Permissions         []Code
	Recent              bool
	ChangesGraph        bool
}

const (
	OpTenantList           Operation = "TenantList"
	OpTenantCreate         Operation = "TenantCreate"
	OpTenantRead           Operation = "TenantRead"
	OpTenantUpdate         Operation = "TenantUpdate"
	OpTenantTransfer       Operation = "TenantTransfer"
	OpTenantCandidates     Operation = "TenantCandidates"
	OpMyTenants            Operation = "MyTenants"
	OpTenantContext        Operation = "TenantContext"
	OpTenantProfileRead    Operation = "TenantProfileRead"
	OpTenantProfileUpdate  Operation = "TenantProfileUpdate"
	OpTenantAudit          Operation = "TenantAudit"
	OpAccountTenants       Operation = "AccountTenants"
	OpHealth               Operation = "Health"
	OpReady                Operation = "Ready"
	OpHelp                 Operation = "Help"
	OpCSRF                 Operation = "CSRF"
	OpLogin                Operation = "Login"
	OpLogout               Operation = "Logout"
	OpInspectInvitation    Operation = "InspectInvitation"
	OpAcceptInvitation     Operation = "AcceptInvitation"
	OpInspectReset         Operation = "InspectReset"
	OpCompleteReset        Operation = "CompleteReset"
	OpMe                   Operation = "Me"
	OpReauthenticate       Operation = "Reauthenticate"
	OpActivity             Operation = "Activity"
	OpProfileRead          Operation = "ProfileRead"
	OpProfileUpdate        Operation = "ProfileUpdate"
	OpPasswordUpdate       Operation = "PasswordUpdate"
	OpSessionRead          Operation = "SessionRead"
	OpSessionRevoke        Operation = "SessionRevoke"
	OpLogoutAll            Operation = "LogoutAll"
	OpSecurityEventRead    Operation = "SecurityEventRead"
	OpInvitationRead       Operation = "InvitationRead"
	OpInvitationCreate     Operation = "InvitationCreate"
	OpInvitationRevoke     Operation = "InvitationRevoke"
	OpInvitationRegenerate Operation = "InvitationRegenerate"
	OpPermissionRead       Operation = "PermissionRead"
	OpRoleList             Operation = "RoleList"
	OpRoleRead             Operation = "RoleRead"
	OpRoleCreate           Operation = "RoleCreate"
	OpRoleUpdate           Operation = "RoleUpdate"
	OpRoleSetPermissions   Operation = "RoleSetPermissions"
	OpRoleSetStatus        Operation = "RoleSetStatus"
	OpRoleDelete           Operation = "RoleDelete"
	OpAccountList          Operation = "AccountList"
	OpAccountRead          Operation = "AccountRead"
	OpAccountAuthorization Operation = "AccountAuthorization"
	OpAccountAssignRoles   Operation = "AccountAssignRoles"
	OpAccountDisable       Operation = "AccountDisable"
	OpAccountEnable        Operation = "AccountEnable"
	OpAuditRead            Operation = "AuditRead"
)

func Policies() []Policy {
	return []Policy{
		{OpTenantList, "GET", "/api/v1/platform/tenants", "permission", []Code{TenantRead}, false, false},
		{OpTenantCreate, "POST", "/api/v1/platform/tenants", "permission", []Code{TenantCreate}, true, true},
		{OpTenantRead, "GET", "/api/v1/platform/tenants/:tenant_id", "permission", []Code{TenantRead}, false, false},
		{OpTenantUpdate, "PATCH", "/api/v1/platform/tenants/:tenant_id", "permission", []Code{TenantUpdate}, true, false},
		{OpTenantTransfer, "POST", "/api/v1/platform/tenants/:tenant_id/administrator-transfer", "permission", []Code{TenantTransfer}, true, true},
		{OpTenantCandidates, "GET", "/api/v1/platform/tenant-admin-candidates", "permission_any", []Code{TenantCreate, TenantTransfer}, false, false},
		{OpMyTenants, "GET", "/api/v1/account/tenants", "authenticated", nil, false, false},
		{OpTenantContext, "GET", "/api/v1/tenants/:tenant_id/context", "tenant", nil, false, false},
		{OpTenantProfileRead, "GET", "/api/v1/tenants/:tenant_id/profile", "tenant", []Code{TenantProfileRead}, false, false},
		{OpTenantProfileUpdate, "PATCH", "/api/v1/tenants/:tenant_id/profile", "tenant", []Code{TenantProfileUpdate}, true, false},
		{OpTenantAudit, "GET", "/api/v1/tenants/:tenant_id/audit-events", "tenant", []Code{TenantAuditRead}, false, false},
		{OpAccountTenants, "GET", "/api/v1/access/accounts/:id/tenant-memberships", "permission", []Code{AccountRead, TenantRead}, false, false},
		{OpHealth, "GET", "/healthz", "public", nil, false, false},
		{OpReady, "GET", "/readyz", "public", nil, false, false},
		{OpHelp, "GET", "/api/v1/public/account-help", "public", nil, false, false},
		{OpCSRF, "GET", "/api/v1/auth/csrf", "public", nil, false, false},
		{OpLogin, "POST", "/api/v1/auth/login", "public", nil, false, false},
		{OpLogout, "POST", "/api/v1/auth/logout", "public", nil, false, false},
		{OpInspectInvitation, "POST", "/api/v1/public/invitations/inspect", "credential", nil, false, false},
		{OpAcceptInvitation, "POST", "/api/v1/public/invitations/accept", "credential", nil, false, false},
		{OpInspectReset, "POST", "/api/v1/public/password-resets/inspect", "credential", nil, false, false},
		{OpCompleteReset, "POST", "/api/v1/public/password-resets/complete", "credential", nil, false, false},
		{OpMe, "GET", "/api/v1/auth/me", "authenticated", nil, false, false},
		{OpReauthenticate, "POST", "/api/v1/auth/reauthenticate", "authenticated", nil, false, false},
		{OpActivity, "POST", "/api/v1/auth/activity", "authenticated", nil, false, false},
		{OpProfileRead, "GET", "/api/v1/account/profile", "permission", []Code{ProfileRead}, false, false},
		{OpProfileUpdate, "PATCH", "/api/v1/account/profile", "permission", []Code{ProfileUpdate}, false, false},
		{OpPasswordUpdate, "POST", "/api/v1/account/password", "permission", []Code{PasswordUpdate}, false, false},
		{OpSessionRead, "GET", "/api/v1/account/sessions", "permission", []Code{SessionRead}, false, false},
		{OpSessionRevoke, "DELETE", "/api/v1/account/sessions/:id", "permission", []Code{SessionRevoke}, false, false},
		{OpLogoutAll, "POST", "/api/v1/auth/logout-all", "permission", []Code{SessionRevoke}, false, false},
		{OpSecurityEventRead, "GET", "/api/v1/account/security-events", "permission", []Code{SecurityEventRead}, false, false},
		{OpInvitationRead, "GET", "/api/v1/invitations", "permission", []Code{InvitationRead}, false, false},
		{OpInvitationCreate, "POST", "/api/v1/invitations", "permission", []Code{InvitationCreate}, true, false},
		{OpInvitationRevoke, "POST", "/api/v1/invitations/:id/revoke", "permission", []Code{InvitationRevoke}, false, false},
		{OpInvitationRegenerate, "POST", "/api/v1/invitations/:id/regenerate", "permission", []Code{InvitationRegenerate, InvitationCreate}, true, false},
		{OpPermissionRead, "GET", "/api/v1/access/permissions", "permission", []Code{PermissionRead}, false, false},
		{OpRoleList, "GET", "/api/v1/access/roles", "permission", []Code{RoleRead}, false, false},
		{OpRoleRead, "GET", "/api/v1/access/roles/:id", "permission", []Code{RoleRead}, false, false},
		{OpRoleCreate, "POST", "/api/v1/access/roles", "permission", []Code{RoleCreate}, true, true},
		{OpRoleUpdate, "PATCH", "/api/v1/access/roles/:id", "permission", []Code{RoleUpdate}, true, true},
		{OpRoleSetPermissions, "PUT", "/api/v1/access/roles/:id/permissions", "permission", []Code{RoleSetPermissions}, true, true},
		{OpRoleSetStatus, "PUT", "/api/v1/access/roles/:id/status", "permission", []Code{RoleSetStatus}, true, true},
		{OpRoleDelete, "POST", "/api/v1/access/roles/:id/archive", "permission", []Code{RoleDelete}, true, true},
		{OpAccountList, "GET", "/api/v1/access/accounts", "permission", []Code{AccountRead}, false, false},
		{OpAccountRead, "GET", "/api/v1/access/accounts/:id", "permission", []Code{AccountRead}, false, false},
		{OpAccountAuthorization, "GET", "/api/v1/access/accounts/:id/authorization", "permission", []Code{AccountRead}, false, false},
		{OpAccountAssignRoles, "PUT", "/api/v1/access/accounts/:id/roles", "permission", []Code{AccountAssignRoles}, true, true},
		{OpAccountDisable, "POST", "/api/v1/access/accounts/:id/disable", "permission", []Code{AccountDisable}, true, true},
		{OpAccountEnable, "POST", "/api/v1/access/accounts/:id/enable", "permission", []Code{AccountEnable}, true, true},
		{OpAuditRead, "GET", "/api/v1/access/audit-events", "permission", []Code{AuditRead}, false, false},
	}
}
func PolicyFor(op Operation) (Policy, bool) {
	for _, p := range Policies() {
		if p.Operation == op {
			return p, true
		}
	}
	return Policy{}, false
}
func RoutePolicy(method, path string) (Policy, bool) {
	for _, p := range Policies() {
		if p.Method == method && p.Path == path {
			return p, true
		}
	}
	return Policy{}, false
}
