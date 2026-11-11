// Package access owns the only permission catalog and authorization graph.
package access

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Code string

const (
	TenantPermissionRead     Code = "tenant.permission.read"
	TenantRoleRead           Code = "tenant.role.read"
	TenantRoleCreate         Code = "tenant.role.create"
	TenantRoleUpdate         Code = "tenant.role.update"
	TenantRoleSetPermissions Code = "tenant.role.set_permissions"
	TenantRoleSetStatus      Code = "tenant.role.set_status"
	TenantRoleDelete         Code = "tenant.role.delete"
	TenantMemberAssignRoles  Code = "tenant.member.assign_roles"

	TenantMemberRead           Code = "tenant.member.read"
	TenantMemberSuspend        Code = "tenant.member.suspend"
	TenantMemberResume         Code = "tenant.member.resume"
	TenantMemberRemove         Code = "tenant.member.remove"
	TenantInvitationRead       Code = "tenant.invitation.read"
	TenantInvitationCreate     Code = "tenant.invitation.create"
	TenantInvitationRevoke     Code = "tenant.invitation.revoke"
	TenantInvitationRegenerate Code = "tenant.invitation.regenerate"
	ProfileRead                Code = "account.profile.read"
	ProfileUpdate              Code = "account.profile.update"
	PasswordUpdate             Code = "account.password.update"
	SessionRead                Code = "account.session.read"
	SessionRevoke              Code = "account.session.revoke"
	SecurityEventRead          Code = "account.security_event.read"
	InvitationRead             Code = "platform.invitation.read"
	InvitationCreate           Code = "platform.invitation.create"
	InvitationRevoke           Code = "platform.invitation.revoke"
	InvitationRegenerate       Code = "platform.invitation.regenerate"
	AccountRead                Code = "platform.account.read"
	AccountAssignRoles         Code = "platform.account.assign_roles"
	AccountDisable             Code = "platform.account.disable"
	AccountEnable              Code = "platform.account.enable"
	RoleRead                   Code = "platform.role.read"
	RoleCreate                 Code = "platform.role.create"
	RoleUpdate                 Code = "platform.role.update"
	RoleSetPermissions         Code = "platform.role.set_permissions"
	RoleSetStatus              Code = "platform.role.set_status"
	RoleDelete                 Code = "platform.role.delete"
	PermissionRead             Code = "platform.permission.read"
	AuditRead                  Code = "platform.audit.read"
	TenantDisable              Code = "platform.tenant.disable"
	TenantEnable               Code = "platform.tenant.enable"
	TenantRead                 Code = "platform.tenant.read"
	TenantCreate               Code = "platform.tenant.create"
	TenantUpdate               Code = "platform.tenant.update"
	TenantTransfer             Code = "platform.tenant.transfer_admin"
	TenantProfileRead          Code = "tenant.profile.read"
	TenantProfileUpdate        Code = "tenant.profile.update"
	TenantAuditRead            Code = "tenant.audit.read"
)

type Permission struct {
	Code                        Code   `json:"code"`
	Name                        string `json:"name"`
	Group                       string `json:"group"`
	Scope                       string `json:"scope"`
	Description                 string `json:"description"`
	Risk                        string `json:"risk"`
	Lifecycle                   string `json:"lifecycle"`
	ProjectCustomRoleAssignable bool   `json:"project_custom_role_assignable"`
	TenantCustomRoleAssignable  bool   `json:"tenant_custom_role_assignable"`
}

func baseCatalog() []Permission {
	return []Permission{
		{TenantDisable, "停用租户", "租户治理", "platform", "关闭租户业务访问，保留身份与历史；同时需要租户查看权限。", "high", "active", false, false},
		{TenantEnable, "恢复租户", "租户治理", "platform", "核验当前管理员后恢复租户；同时需要租户查看权限。", "high", "active", false, false},
		{TenantPermissionRead, "查看租户权限目录", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleRead, "查看租户角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleCreate, "创建租户角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleUpdate, "编辑租户角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleSetPermissions, "配置租户角色权限", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleSetStatus, "启停租户角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantRoleDelete, "删除未绑定租户角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},
		{TenantMemberAssignRoles, "配置租户成员角色", "租户授权管理", "tenant", "仅当前租户管理员持有，不可下放。", "high", "active", false, false},

		{TenantMemberRead, "查看租户成员", "当前租户", "tenant", "查看本租户成员和关系历史。", "normal", "active", false, true},
		{TenantMemberSuspend, "暂停租户成员", "当前租户", "tenant", "暂停本租户普通成员访问。", "high", "active", false, true},
		{TenantMemberResume, "恢复租户成员", "当前租户", "tenant", "恢复本租户暂停的普通成员。", "high", "active", false, true},
		{TenantMemberRemove, "移除租户成员", "当前租户", "tenant", "结束本租户普通成员关系。", "high", "active", false, true},
		{TenantInvitationRead, "查看成员邀请", "当前租户", "tenant", "查看本租户成员邀请及历史。", "normal", "active", false, true},
		{TenantInvitationCreate, "邀请租户成员", "当前租户", "tenant", "精确定位已有账号并邀请其成为普通成员。", "high", "active", false, true},
		{TenantInvitationRevoke, "撤销成员邀请", "当前租户", "tenant", "撤销本租户待处理成员邀请。", "high", "active", false, true},
		{TenantInvitationRegenerate, "重新生成成员邀请", "当前租户", "tenant", "替换成员邀请链接，同时需要创建权限。", "high", "active", false, true},
		{TenantRead, "查看租户档案", "租户治理", "platform", "查看平台租户档案及管理员任职。", "normal", "active", false, false},
		{TenantCreate, "创建租户", "租户治理", "platform", "创建租户并指定固定初始管理员。", "high", "active", false, false},
		{TenantUpdate, "修改租户档案", "租户治理", "platform", "修改租户名称、编号和说明。", "high", "active", false, false},
		{TenantTransfer, "交接租户管理员", "租户治理", "platform", "指定新管理员并立即结束原成员关系。", "high", "active", false, false},
		{TenantProfileRead, "查看租户资料", "当前租户", "tenant", "查看本人有效成员关系所属租户的资料。", "normal", "active", false, true},
		{TenantProfileUpdate, "修改租户资料", "当前租户", "tenant", "修改该租户的名称、编号和说明。", "high", "active", false, true},
		{TenantAuditRead, "查看租户操作记录", "当前租户", "tenant", "查看该租户档案、成员与邀请治理记录。", "normal", "active", false, true},
		{ProfileRead, "查看本人资料", "个人账号", "self", "查看本人完整资料、角色及权限来源。", "normal", "active", false, false},
		{ProfileUpdate, "修改本人资料", "个人账号", "self", "修改本人显示名称。", "normal", "active", false, false},
		{PasswordUpdate, "修改本人密码", "个人账号", "self", "验证当前密码后修改本人密码。", "high", "active", false, false},
		{SessionRead, "查看本人会话", "个人账号", "self", "查看本人有效会话。", "normal", "active", false, false},
		{SessionRevoke, "撤销本人会话", "个人账号", "self", "撤销本人指定会话或退出全部设备。", "high", "active", false, false},
		{SecurityEventRead, "查看本人安全记录", "个人账号", "self", "查看与本人相关的安全记录摘要。", "normal", "active", false, false},
		{InvitationRead, "查看本人邀请", "邀请", "platform", "仅查看本人创建的邀请。", "normal", "active", false, false},
		{InvitationCreate, "创建基础成员邀请", "邀请", "platform", "创建仅授予基础成员角色的邀请。", "high", "active", false, false},
		{InvitationRevoke, "撤销本人邀请", "邀请", "platform", "撤销本人未使用的邀请。", "normal", "active", false, false},
		{InvitationRegenerate, "重新生成本人邀请", "邀请", "platform", "重新生成本人邀请，旧链接失效；同时需要创建邀请权限。", "high", "active", false, false},
		{AccountRead, "查看平台账号", "账号管理", "platform", "查看账号列表、详情、角色及有效权限来源；不含私人会话。", "normal", "active", false, false},
		{AccountAssignRoles, "配置账号角色", "账号管理", "platform", "为平台账号绑定或解绑角色，受本人权限范围约束。", "high", "active", false, false},
		{AccountDisable, "停用平台账号", "账号管理", "platform", "常规停用账号并撤销凭证，保护最后管理账号。", "high", "active", false, false},
		{AccountEnable, "恢复平台账号", "账号管理", "platform", "恢复账号登录资格，不恢复历史凭证。", "high", "active", false, false},
		{RoleRead, "查看角色", "角色管理", "platform", "查看角色配置、状态和绑定数量。", "normal", "active", false, false},
		{RoleCreate, "创建自定义角色", "角色管理", "platform", "创建空权限集合的自定义角色。", "normal", "active", false, false},
		{RoleUpdate, "编辑角色资料", "角色管理", "platform", "修改自定义角色的名称及说明。", "normal", "active", false, false},
		{RoleSetPermissions, "配置角色权限", "角色管理", "platform", "替换自定义角色权限，影响所有绑定者，受本人权限范围约束。", "high", "active", false, false},
		{RoleSetStatus, "启停角色", "角色管理", "platform", "启停自定义角色，影响所有绑定者的有效权限。", "high", "active", false, false},
		{RoleDelete, "删除未绑定角色", "角色管理", "platform", "软删除没有任何账号绑定的自定义角色。", "high", "active", false, false},
		{PermissionRead, "查看权限目录", "平台审计", "platform", "查看完整的只读权限目录、范围及风险说明。", "normal", "active", false, false},
		{AuditRead, "查看平台审计", "平台审计", "platform", "查看平台级账号和授权变更的操作者、原因与差异。", "high", "active", false, false},
	}
}
func activeCodes() []Code {
	v := []Code{}
	for _, p := range Catalog() {
		if p.Lifecycle == "active" {
			v = append(v, p.Code)
		}
	}
	return v
}
func known(code Code) bool {
	for _, p := range Catalog() {
		if p.Code == code && p.Lifecycle == "active" {
			return true
		}
	}
	return false
}
func Checksum() string {
	b, _ := json.Marshal(struct {
		Permissions []Permission
		Roles       []Builtin
		Policies    []Policy
	}{Catalog(), Builtins(), Policies()})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
