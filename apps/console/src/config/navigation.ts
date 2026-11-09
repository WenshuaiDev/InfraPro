import { House, MailPlus, ShieldCheck, UserRound, Users, KeyRound, BookOpen, ScrollText } from 'lucide-react';
import type { NavigationGroup } from '@infrapro/layouts';
import { Permissions as P, type PermissionCode } from '@infrapro/api-client';

export const workspacePaths = ['/', '/profile', '/security', '/invitations', '/access/accounts', '/access/roles', '/access/permissions', '/access/audit'] as const;
export type WorkspacePath = typeof workspacePaths[number];
export function safeReturn(value: unknown): WorkspacePath { return workspacePaths.find((path) => path === value) ?? '/'; }
export function relativePath(path: string) {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  return base && (path === base || path.startsWith(`${base}/`)) ? path.slice(base.length) || '/' : path;
}
export function allowedPath(path: string, can: (permission: PermissionCode) => boolean) {
  const permissions: Partial<Record<WorkspacePath, PermissionCode[]>> = {
    '/profile': [P.AccountProfileRead], '/security': [P.AccountPasswordUpdate, P.AccountSessionRead, P.AccountSessionRevoke, P.AccountSecurityEventRead],
    '/invitations': [P.PlatformInvitationRead], '/access/accounts': [P.PlatformAccountRead], '/access/roles': [P.PlatformRoleRead], '/access/permissions': [P.PlatformPermissionRead], '/access/audit': [P.PlatformAuditRead],
  };
  return permissions[safeReturn(path)]?.some(can) ?? true;
}
export function navigation(path: string, can: (permission: PermissionCode) => boolean = () => true): NavigationGroup[] {
  return [
    { title: '工作空间', items: [{ title: '首页', href: '/', icon: House, active: path === '/' }] },
    { title: '账号', items: [
      { title: '个人资料', href: '/profile', icon: UserRound, active: path === '/profile' },
      { title: '账号安全', href: '/security', icon: ShieldCheck, active: path === '/security' },
      { title: '邀请管理', href: '/invitations', icon: MailPlus, active: path === '/invitations' },
    ] },
    { title: '平台管理', items: [
      { title: '账号管理', href: '/access/accounts', icon: Users, active: path === '/access/accounts' },
      { title: '角色管理', href: '/access/roles', icon: KeyRound, active: path === '/access/roles' },
      { title: '权限目录', href: '/access/permissions', icon: BookOpen, active: path === '/access/permissions' },
      { title: '平台审计', href: '/access/audit', icon: ScrollText, active: path === '/access/audit' },
    ] },
  ].map((group) => ({ ...group, items: group.items.filter((item) => allowedPath(item.href, can)) })).filter((group) => group.items.length > 0);
}
