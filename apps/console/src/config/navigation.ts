import { House, MailPlus, ShieldCheck, UserRound, Users, KeyRound, BookOpen, ScrollText, Building2 } from 'lucide-react';
import type { NavigationGroup } from '@infrapro/layouts';
import { Permissions as P, type PermissionCode } from '@infrapro/api-client';

export const workspacePaths = ['/', '/profile', '/security', '/invitations', '/access/accounts', '/access/roles', '/access/permissions', '/access/audit', '/tenants', '/platform/tenants'] as const;
export type WorkspacePath = typeof workspacePaths[number];
const uuid = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';
export function safeReturn(value: unknown): string {
 if (typeof value !== 'string') return '/';
 if (workspacePaths.some((path) => path === value)) return value;
 return new RegExp(`^/(?:platform/tenants/${uuid}|t/${uuid}/(?:profile|audit))$`).test(value) ? value : '/';
}
export function relativePath(path: string) {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  return base && (path === base || path.startsWith(`${base}/`)) ? path.slice(base.length) || '/' : path;
}
export function allowedPath(path: string, can: (permission: PermissionCode) => boolean) {
  if (path.startsWith('/platform/tenants')) return [P.PlatformTenantRead, P.PlatformTenantCreate, P.PlatformTenantUpdate, P.PlatformTenantTransferAdmin].some(can);
  const permissions: Partial<Record<WorkspacePath, PermissionCode[]>> = {
    '/profile': [P.AccountProfileRead], '/security': [P.AccountPasswordUpdate, P.AccountSessionRead, P.AccountSessionRevoke, P.AccountSecurityEventRead],
    '/invitations': [P.PlatformInvitationRead], '/access/accounts': [P.PlatformAccountRead], '/access/roles': [P.PlatformRoleRead], '/access/permissions': [P.PlatformPermissionRead], '/access/audit': [P.PlatformAuditRead],
  };
  return permissions[workspacePaths.find((p) => p === path) ?? '/']?.some(can) ?? true;
}
export function navigation(path: string, can: (permission: PermissionCode) => boolean = () => true): NavigationGroup[] {
  const tenant = path.match(new RegExp(`^/t/(${uuid})/(?:profile|audit)$`))?.[1];
  return [
    ...(tenant ? [{ title: '当前租户', items: [
      { title: '租户资料', href: `/t/${tenant}/profile`, icon: Building2, active: path.endsWith('/profile') },
      { title: '租户操作记录', href: `/t/${tenant}/audit`, icon: ScrollText, active: path.endsWith('/audit') },
      { title: '切换租户', href: '/tenants', icon: Building2, active: false },
    ] }] : []),
    { title: '工作空间', items: [{ title: '首页', href: '/', icon: House, active: path === '/' }, { title: '我的租户', href: '/tenants', icon: Building2, active: path === '/tenants' }] },
    { title: '账号', items: [
      { title: '个人资料', href: '/profile', icon: UserRound, active: path === '/profile' },
      { title: '账号安全', href: '/security', icon: ShieldCheck, active: path === '/security' },
      { title: '邀请管理', href: '/invitations', icon: MailPlus, active: path === '/invitations' },
    ] },
    { title: '平台管理', items: [
      { title: '租户管理', href: '/platform/tenants', icon: Building2, active: path.startsWith('/platform/tenants') },
      { title: '账号管理', href: '/access/accounts', icon: Users, active: path === '/access/accounts' },
      { title: '角色管理', href: '/access/roles', icon: KeyRound, active: path === '/access/roles' },
      { title: '权限目录', href: '/access/permissions', icon: BookOpen, active: path === '/access/permissions' },
      { title: '平台审计', href: '/access/audit', icon: ScrollText, active: path === '/access/audit' },
    ] },
  ].map((group) => ({ ...group, items: group.items.filter((item) => allowedPath(item.href, can)) })).filter((group) => group.items.length > 0);
}
