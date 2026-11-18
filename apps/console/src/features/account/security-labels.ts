export const events: Record<string, string> = { mfa_enabled: '启用 MFA', mfa_replaced: '更换验证器', mfa_disabled: '关闭 MFA', mfa_recovery_codes_regenerated: '重建恢复码', mfa_recovery_code_used: '使用恢复码登录', mfa_reset: '重置 MFA', mfa_recovered: '完成 MFA 恢复', mfa_verification: 'MFA 验证', mfa_policy_changed: '修改 MFA 策略', account_authorization_changed: '本人角色权限变化', initial_role_bound: '开户角色绑定', account_roles_changed: '本人角色绑定变化', bootstrap: '首次账号创建', login: '登录', logout: '退出登录', logout_all: '退出全部设备', session_revoked: '会话撤销', password_changed: '修改密码', password_reset: '密码重置', reset_link_created: '生成重置链接', reauthentication: '密码确认', invitation_created: '创建邀请', invitation_revoked: '撤销邀请', invitation_accepted: '邀请已被接受', account_created: '账号创建', account_disabled: '账号停用', account_emergency_disabled: '应急停用', account_enabled: '账号恢复', profile_updated: '资料修改', avatar_updated: '更换账号头像', avatar_removed: '移除账号头像' };
export const outcomes: Record<string, string> = { success: '成功', failure: '失败', rate_limited: '尝试受限' };

export function clientLabel(agent: string) {
  if (!agent) return '未知客户端';
  const browsers: [RegExp, string][] = [[/Playwright/i, 'Playwright'], [/HeadlessChrome/i, 'Headless Chrome'], [/Edg(?:e|A|iOS)?\//, 'Edge'], [/Firefox|FxiOS/, 'Firefox'], [/Chrome|CriOS/, 'Chrome'], [/Safari/, 'Safari']];
  const systems: [RegExp, string][] = [[/iPhone|iPad/, 'iOS'], [/Android/, 'Android'], [/Macintosh|macOS/, 'macOS'], [/Windows/, 'Windows'], [/Linux/, 'Linux']];
  const browser = browsers.find(([pattern]) => pattern.test(agent))?.[1] ?? '其他客户端';
  const system = systems.find(([pattern]) => pattern.test(agent))?.[1];
  return system ? browser + ' · ' + system : browser;
}
