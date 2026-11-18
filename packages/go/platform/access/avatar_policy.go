package access

func avatarPolicies() []Policy {
	return []Policy{
		{"AvatarRead", "GET", "/api/v1/account/avatar", "authenticated", nil, false, false},
		{"AvatarUpdate", "PUT", "/api/v1/account/avatar", "permission", []Code{ProfileUpdate}, false, false},
		{"AvatarRemove", "DELETE", "/api/v1/account/avatar", "permission", []Code{ProfileUpdate}, false, false},
		{"AccountAvatarRead", "GET", "/api/v1/access/accounts/:id/avatar", "permission", []Code{AccountRead}, false, false},
		{"TenantMemberAvatarRead", "GET", "/api/v1/tenants/:tenant_id/members/:membership_id/avatar", "tenant", []Code{TenantMemberRead}, false, false},
		{"ProjectMemberAvatarRead", "GET", "/api/v1/tenants/:tenant_id/projects/:project_id/members/:membership_id/avatar", "project", []Code{ProjectMemberRead}, false, false},
	}
}
