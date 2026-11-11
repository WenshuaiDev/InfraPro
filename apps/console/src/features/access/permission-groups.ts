import type { Permission } from '@infrapro/api-client';

export const tenantPermissionSections = [
  { title: '资料', part: 'profile' },
  { title: '项目', part: 'project' },
  { title: '成员', part: 'member' },
  { title: '邀请', part: 'invitation' },
  { title: '审计', part: 'audit' },
];

export function tenantPermissionGroup(permission: Pick<Permission, 'code' | 'group' | 'tenant_custom_role_assignable'>) {
  if (!permission.tenant_custom_role_assignable) return '管理员保留能力';
  return tenantPermissionSections.find((group) => group.part === permission.code.split('.')[1])?.title ?? permission.group;
}

export function groupPermissions<T extends { group: string }>(permissions: T[], name: (permission: T) => string = (permission) => permission.group, order: string[] = []) {
  const groups = new Map<string, T[]>();
  for (const permission of permissions) {
    const group = name(permission) || '其他权限';
    groups.set(group, [...(groups.get(group) ?? []), permission]);
  }
  return [...new Set([...order, ...groups.keys()])].flatMap((group) => groups.has(group) ? [{ name: group, items: groups.get(group)! }] : []);
}
