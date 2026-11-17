package auditrecord

import (
	"slices"
	"strings"
)

// Definition is a finite disclosure contract, not a prefix-based allow list.
// ScopeRule "business" requires the source's actual tenant/project ownership.
type Definition struct {
	Event     string   `json:"event_type"`
	Module    string   `json:"module"`
	Category  string   `json:"category"`
	Phase     string   `json:"phase"`
	ScopeRule string   `json:"scope_rule"`
	Fields    []string `json:"fields"`
	Metadata  string   `json:"metadata"`
	Label     string   `json:"label"`
}

var definitions = buildCatalog()

func Lookup(event string) (Definition, bool) {
	d, ok := definitions[event]
	d.Fields = slices.Clone(d.Fields)
	return d, ok
}
func Catalog() []Definition {
	v := make([]Definition, 0, len(definitions))
	for _, d := range definitions {
		d.Fields = slices.Clone(d.Fields)
		v = append(v, d)
	}
	slices.SortFunc(v, func(a, b Definition) int { return strings.Compare(a.Event, b.Event) })
	return v
}

var governanceFields = strings.Fields("phase failure_category failed_attempts enable_mode recovery_grant_id recovery_codes_remaining require_mfa factor_version recovery_generation enforcement_generation expected_factor_version expected_recovery_generation state changed reason expected_version expected_membership_version role_versions preview_hash role account tenant_name management_version id account_id username display_name name code description status enabled emergency version revision role_ids roles permissions permission_codes configured_permissions effective_permissions membership_id membership_version membership_reused tenant_id project_id administrator_account_id administrator_id owner_id owner_membership_id member_id created_at updated_at assigned_at revoked_at expires_at closed_at accepted_at joined_at removed_at source_role_id source_role_version assignment_policy binding_scope builtin_key active inactive_reasons authorization_revision public message public_message tenant_notice tenant before after count available management_gap_reason replacement affected_count")
var businessFields = strings.Fields("method_id method_version source mapping_revision adopted sequence asset_id current_version_id base_version_id format point_count component_count feature_count grants policy link ended updated allow_facility_create source_download is_default mode viewers capabilities fields valid_until revoked current_release_id draft_config_id package_id grant id code name display_name title status state version revision changed_fields target_id definition_id definition_version_id facility_id component_id point_id channel_id device_id manufacturer_id product_id product_version_id output_id binding_id resource_id job_id run_id generation generation_id config_id source_version_id attachment_id attachment_revision_id revision_id file_id prepared_file_id upload_id row_count size_bytes media_type sha256 filename current_revision previous_revision reason filter field_profile epoch authorization_revision projection_version expected_generation expected_control_revision action from to actor_id object_type object_id event_type outcome category operation_id request_id q task_module task_kind task_id generation result_id version_id access_revision head_revision manifest_root stale output_range started_at finished_at error_code field unit value before after count enabled active summary added removed retained")

func buildCatalog() map[string]Definition {
	out := map[string]Definition{}
	add := func(module, category, phase, scope, metadata, events string, fields []string) {
		for _, event := range strings.Fields(events) {
			out[event] = Definition{event, module, category, phase, scope, slices.Clone(fields), metadata, eventLabel(module, event)}
		}
	}
	add("identity", "security", "command", "platform", "governance", `mfa_policy_changed mfa_enabled mfa_replaced mfa_disabled mfa_recovery_codes_regenerated mfa_recovery_code_used mfa_reset mfa_recovered mfa_verification login logout logout_all session_revoked reauthentication password_changed password_reset reset_link_created backup_access_revoked profile_updated invitation_created invitation_revoked invitation_accepted account_created account_disabled account_enabled account_emergency_disabled`, governanceFields)
	add("identity", "maintenance", "maintenance", "platform", "governance", "bootstrap", governanceFields)
	add("access", "management", "command", "platform", "governance", `initial_role_bound admin_recovered catalog_synced role_created account_roles_changed account_authorization_changed RoleUpdate RoleSetPermissions RoleSetStatus RoleDelete AccountDisable AccountEnable`, governanceFields)
	add("tenancy", "management", "command", "platform_tenant", "governance", `tenant.created tenant.administrator.transferred tenant.administrator.unavailable tenant.administrator.available`, governanceFields)
	add("tenancy", "management", "command", "notice", "governance", `tenant.disabled tenant.enabled`, governanceFields)
	add("tenancy", "management", "command", "entry", "governance", `tenant.profile.updated`, governanceFields)
	add("tenancy", "management", "command", "tenant", "governance", `tenant.role.created tenant.role.copied tenant.role.updated tenant.role.permissions_changed tenant.role.status_changed tenant.role.deleted tenant.member.joined tenant.member.suspended tenant.member.resumed tenant.member.removed tenant.member.resolved tenant.member.roles_changed tenant.member.authorization_changed tenant.authorization.preset_synced tenant.invitation.invalidated tenant.invitation.created tenant.invitation.revoked tenant.invitation.regenerated tenant.invitation.accepted tenant.invitation.expired tenant.invitation.inviter_unavailable tenant.invitation.tenant_disabled tenant.invitation.authorization_changed`, governanceFields)
	add("projects", "management", "command", "governance", "governance", `tenant.project.governance`, governanceFields)
	add("projects", "management", "command", "project", "governance", `project.ProjectCreate project.ProjectUpdate project.ProjectMemberAdd project.ProjectMemberSuspend project.ProjectMemberResume project.ProjectMemberRemove project.ProjectOwnerChange project.ProjectTransfer project.ProjectRecoverAdmin project.ProjectArchive project.ProjectRestore project.ProjectRolePermissions project.ProjectRoleStatus project.ProjectRoleDelete project.ProjectMemberRoles project.ProjectRoleCreate project.ProjectRoleCopy project.ProjectRoleUpdate project.member.parent_ended project.administrator.gap project.administrator.unavailable project.administrator.available project.authorization.preset_synced project.resource_access.changed`, governanceFields)
	add("notifications", "maintenance", "maintenance", "platform", "governance", `notification.delivery.replayed notification.handoff.replayed`, governanceFields)
	add("files", "maintenance", "maintenance", "platform", "governance", `files.restore_interrupted`, businessFields)
	add("files", "maintenance", "maintenance", "platform", "governance", `files.maintenance.integrity files.maintenance.cleanup.planned files.maintenance.cleanup.item files.maintenance.cleanup.completed files.maintenance.backup.prepared files.maintenance.backup.sealed files.maintenance.backup.released`, businessFields)
	add("tasks", "task", "command", "business", "task", `task.cancel task.retry`, businessFields)
	add("monitoring", "business", "execution", "business", "monitoring", `monitoring.retention.completed`, businessFields)
	add("monitoring", "management", "command", "tenant", "monitoring", `monitoring.access.grant monitoring.access.revoke monitoring.access.replace monitoring.access.request monitoring.access.approve monitoring.access.reject monitoring.access.create monitoring.access.update monitoring.access.delete`, businessFields)
	add("derived", "business", "command", "business", "derived", `derived.channels.create derived.channels.trials derived.channels.control derived.generations.replay derived.runs.cancel derived.runs.retry`, businessFields)
	add("derived", "business", "execution", "business", "derived", `derived.results.published derived.analysis.published derived.results.integrity`, businessFields)
	add("spatial", "maintenance", "maintenance", "tenant", "spatial", `spatial.maintenance.integrity`, businessFields)
	add("spatial", "file", "transfer", "business", "spatial", `spatial.source.download`, businessFields)
	add("audit", "task", "command", "business_or_platform", "audit", `audit.export.requested audit.export.cancel.accepted audit.export.retry`, businessFields)
	add("audit", "task", "execution", "business_or_platform", "audit", `audit.export.succeeded audit.export.failed audit.export.cancelled audit.export.restore_interrupted`, businessFields)
	add("audit", "maintenance", "maintenance", "platform", "governance", `audit.maintenance.planned audit.maintenance.applied audit.maintenance.hold_created audit.maintenance.hold_released audit.maintenance.mode audit.maintenance.handoff_replayed`, strings.Fields("kind mode plan_id hold_id deleted protected failed classified unclassified candidates active_holds policy_applied source count platform_seconds tenant_seconds project_seconds"))
	add("audit", "security", "command", "platform", "governance", `audit.command.denied audit.command.conflict audit.command.failure audit.command.unknown`, []string{"action", "error_code"})
	for _, module := range []string{"files", "monitoring", "derived", "spatial", "audit"} {
		for _, action := range []string{"authorized", "finished", "interrupted_unknown"} {
			add(module, "file", "transfer", "business_or_platform", module, module+".download."+action, []string{"transfer_id", "file_id", "revision_id", "generation", "sent_bytes", "transfer_outcome", "method", "range"})
		}
	}
	for _, module := range []string{"monitoring", "derived", "spatial", "display", "files", "parsing", "ingestion"} {
		for _, action := range []string{"accepted", "succeeded", "failed", "cancelled", "needs_action"} {
			scope := "business"
			if module == "parsing" || module == "ingestion" {
				scope = "business_or_platform"
			}
			add(module, "task", "execution", scope, module, module+".execution."+action, businessFields)
		}
	}
	registerDomainEvents(add)
	return out
}

// Labels are generated only while registering this finite catalog. They never
// authorize an unknown event or infer its scope from a name prefix.
func eventLabel(module, event string) string {
	exact := map[string]string{
		"mfa_policy_changed": "修改多因素认证策略", "mfa_enabled": "启用多因素认证", "mfa_replaced": "更换验证器", "mfa_disabled": "关闭多因素认证", "mfa_recovery_codes_regenerated": "重建恢复码", "mfa_recovery_code_used": "使用恢复码登录", "mfa_reset": "重置多因素认证", "mfa_recovered": "恢复多因素认证", "mfa_verification": "验证多因素认证",
		"login": "登录", "logout": "退出登录", "logout_all": "撤销所有会话", "session_revoked": "撤销会话", "reauthentication": "重新验证身份", "password_changed": "修改密码", "password_reset": "重置密码", "reset_link_created": "创建密码重置凭据", "backup_access_revoked": "撤销备用访问", "profile_updated": "修改个人资料", "invitation_created": "创建开户邀请", "invitation_revoked": "撤销开户邀请", "invitation_accepted": "接受开户邀请", "account_created": "创建账号", "account_disabled": "停用账号", "account_enabled": "恢复账号", "account_emergency_disabled": "紧急停用账号", "bootstrap": "初始化平台", "initial_role_bound": "绑定初始角色", "admin_recovered": "恢复管理员", "catalog_synced": "同步权限目录", "role_created": "创建角色", "account_roles_changed": "配置账号角色", "account_authorization_changed": "更新账号授权", "RoleUpdate": "修改角色", "RoleSetPermissions": "修改角色权限", "RoleSetStatus": "启停角色", "RoleDelete": "删除角色", "AccountDisable": "停用账号", "AccountEnable": "恢复账号",
		"tenant.disabled": "平台停用租户", "tenant.enabled": "平台恢复租户", "tenant.project.governance": "项目治理变化", "files.attachment.bound": "文件检查完成并绑定修订", "audit.export.cancel.accepted": "取消导出请求已接受", "audit.export.restore_interrupted": "恢复中断审计导出",
		"project.ProjectCreate": "创建项目", "project.ProjectUpdate": "修改项目资料", "project.ProjectMemberAdd": "添加项目成员", "project.ProjectMemberSuspend": "暂停项目成员", "project.ProjectMemberResume": "恢复项目成员", "project.ProjectMemberRemove": "移除项目成员", "project.ProjectOwnerChange": "更换项目负责人", "project.ProjectTransfer": "移交项目管理", "project.ProjectRecoverAdmin": "恢复项目管理员", "project.ProjectArchive": "归档项目", "project.ProjectRestore": "恢复项目", "project.ProjectRolePermissions": "修改项目角色权限", "project.ProjectRoleStatus": "启停项目角色", "project.ProjectRoleDelete": "删除项目角色", "project.ProjectMemberRoles": "配置项目成员角色", "project.ProjectRoleCreate": "创建项目角色", "project.ProjectRoleCopy": "复制项目角色", "project.ProjectRoleUpdate": "修改项目角色",
	}
	if label := exact[event]; label != "" {
		return label
	}
	parts := strings.FieldsFunc(event, func(r rune) bool { return r == '.' || r == ':' })
	nouns := map[string]string{"tenant": "租户", "project": "项目", "role": "角色", "member": "成员", "invitation": "邀请", "administrator": "管理员", "authorization": "授权", "profile": "资料", "manufacturer": "厂商", "product": "产品", "device": "设备", "output": "输出", "installation": "安装记录", "source": "来源", "access": "授权", "policy": "授权策略", "link": "设施关联", "grants": "授权集合", "assets": "空间资料", "versions": "版本", "views": "空间视图", "jobs": "任务", "uploads": "上传", "upload": "上传", "instance": "大屏实例", "package": "大屏包", "terminal": "终端", "config": "配置", "attachment": "附件", "task": "任务", "execution": "任务执行", "channels": "派生通道", "runs": "执行", "generations": "计算成果", "results": "计算成果", "analysis": "分析成果", "artifacts": "分析成果", "configs": "配置", "metrics": "指标", "imports": "导入批次", "batches": "批次", "exports": "导出", "export": "导出", "selections": "筛选集", "snapshots": "快照", "retention": "保留清理", "routes": "存储路由", "summaries": "数据汇总", "restore": "恢复", "download": "文件传输", "cleanup": "清理", "backup": "备份", "delivery": "通知投递", "handoff": "持久交接", "command": "命令"}
	verbs := map[string]string{"create": "创建", "created": "已创建", "update": "更新", "updated": "已更新", "save": "保存", "copy": "复制", "copied": "已复制", "delete": "删除", "deleted": "已删除", "state": "修改状态", "status_changed": "状态已变更", "permissions_changed": "权限已变更", "roles_changed": "角色已变更", "authorization_changed": "授权已变更", "preset_synced": "同步预置权限", "new-draft": "创建草稿", "delete-draft": "删除草稿", "draft-save": "保存草稿", "draft-create": "创建草稿", "draft-delete": "删除草稿", "category-save": "保存分类", "category-delete": "删除分类", "publish": "发布", "published": "已发布", "withdraw": "撤销发布", "rollback": "回退发布", "revert": "还原修订", "restore": "恢复", "retire": "退役", "archive": "归档", "bound": "绑定修订", "bind": "绑定", "switch": "切换", "switched": "已切换", "unbind": "解除绑定", "change": "变更", "changed": "已变更", "end": "结束", "relocate": "迁移安装位置", "correct": "更正", "replace": "替换", "joined": "已加入", "suspended": "已暂停", "resumed": "已恢复", "removed": "已移除", "resolved": "成员身份已核实", "transferred": "已移交", "unavailable": "当前不可用", "available": "恢复可用", "gap": "管理职责空缺", "parent_ended": "上级成员关系已结束", "invalidated": "已失效", "revoked": "已撤销", "regenerated": "重新生成", "accepted": "请求已接受", "expired": "已到期", "inviter_unavailable": "邀请人不可用", "tenant_disabled": "租户已停用", "cancel": "取消请求已接受", "cancelled": "实际已取消", "retry": "人工重试", "succeeded": "执行成功", "failed": "执行失败", "needs_action": "等待处理", "complete": "完成提交", "completed": "已完成", "current": "设为当前版本", "edit": "编辑", "process": "请求处理", "assign": "分配", "grant": "授权", "revoke": "撤销授权", "request": "请求授权", "approve": "批准", "reject": "拒绝", "config": "保存配置", "settings": "修改设置", "terminal": "管理终端", "upload": "接受上传", "reconfirm": "重新确认", "trash": "移入回收站", "trials": "请求试算", "trial": "试算", "control": "修改执行状态", "apply-config": "应用配置", "assignments": "配置分段安排", "backfill": "请求回算", "replay": "请求回放", "comparisons": "请求比较", "runs": "请求执行", "exports": "请求导出", "integrity": "核对完整性", "download": "请求下载", "mapping": "修改映射", "validate": "请求校验", "resolve": "解决校验问题", "readopt": "重新采用", "renew": "续期", "queued": "已排队", "rebuild": "请求重建", "rebuilt": "重建完成", "pause": "暂停", "open": "开放写入", "authorized": "已授权开始读取", "finished": "读取已结束", "interrupted_unknown": "读取中断，结果未知", "restore_interrupted": "恢复中断", "requested": "请求已接受", "planned": "生成维护计划", "applied": "执行维护计划", "item": "记录清理结果", "prepared": "准备备份", "sealed": "封存备份", "released": "释放保护", "hold_created": "创建保留保护", "hold_released": "释放保留保护", "mode": "切换维护状态", "handoff_replayed": "重试交接翻译", "replayed": "维护重放", "denied": "已拒绝", "conflict": "修改冲突", "failure": "执行失败", "unknown": "结果待核实"}
	moduleName := map[string]string{"identity": "身份", "access": "权限", "tenancy": "租户", "projects": "项目", "engineering": "工程定义", "resources": "工程资源", "devicecatalog": "设备目录", "devices": "设备台账", "sources": "监测来源", "monitoring": "监测数据", "derived": "派生分析", "spatial": "空间模型", "display": "项目大屏", "files": "文件", "tasks": "任务", "notifications": "通知", "audit": "审计", "parsing": "解析", "ingestion": "设备数据接入"}[module]
	noun := ""
	for _, part := range parts[:len(parts)-1] {
		if value := nouns[part]; value != "" {
			noun = value
		}
	}
	verb := verbs[parts[len(parts)-1]]
	if verb == "" {
		verb = "操作已记录"
	}
	if noun != "" {
		return moduleName + " · " + noun + "：" + verb
	}
	return moduleName + "：" + verb
}
