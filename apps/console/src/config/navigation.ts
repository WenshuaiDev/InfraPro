import { MailPlus, ShieldCheck, UserRound, Users, KeyRound, BookOpen, ScrollText, Building2, FolderKanban, Blocks, Ruler, ClipboardList, Cpu, Factory } from 'lucide-react';
import type { NavigationGroup, NavigationItem } from '@infrapro/layouts';
import { Permissions as P, type PermissionCode } from '@infrapro/api-client';

export const workspacePaths = ['/', '/profile', '/security', '/invitations', '/access/accounts', '/access/roles', '/access/permissions', '/access/audit', '/tenants', '/platform/tenants', '/projects'] as const;
export type WorkspacePath = typeof workspacePaths[number];
const uuid = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';
export function safeReturn(value: unknown): string {
 if (typeof value !== 'string') return '/';
 if (new RegExp(`^/platform/device-products(?:/${uuid})?$`).test(value) || new RegExp(`^/t/${uuid}/(?:device-products(?:/${uuid})?|devices(?:/${uuid})?|projects/${uuid}/device-access|p/${uuid}/(?:devices(?:/${uuid})?|device-outputs/${uuid}))$`).test(value)) return value;
 if (new RegExp(`^/t/${uuid}/(?:p/${uuid}/)?resources/facilities(?:/${uuid})?$`).test(value) || new RegExp(`^/t/${uuid}/resource-access(?:/${uuid}(?:/facilities/${uuid})?)?$`).test(value)) return value;
 if (/^\/platform\/engineering\/(type-definitions|metrics|units|monitoring-templates)$/.test(value) || new RegExp(`^/t/${uuid}/engineering/(templates|catalog/(type-definitions|metrics|units|monitoring-templates))$`).test(value)) return value;
 if (workspacePaths.some((path) => path === value) || value === '/tenant-invite') return value;
 return new RegExp(`^/(?:platform/tenants/${uuid}|t/${uuid}/(?:projects(?:/new|/${uuid})?|p/${uuid}(?:/(?:profile|audit|permissions|roles(?:/${uuid})?|members(?:/${uuid})?))?|profile|audit|permissions|roles(?:/${uuid})?|members(?:/${uuid})?|invitations(?:/${uuid})?))$`).test(value) ? value : '/';
}
export function relativePath(path: string) {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  return base && (path === base || path.startsWith(`${base}/`)) ? path.slice(base.length) || '/' : path;
}
export function allowedPath(path: string, can: (permission: PermissionCode) => boolean) {
  if (path.startsWith('/platform/device-products')) return can(P.DeviceDefinitionRead);
  if (path.startsWith('/platform/engineering/')) { const kind = path.split('/').at(-1); return can(kind === 'type-definitions' ? P.EngineeringTypeRead : kind === 'monitoring-templates' ? P.EngineeringTemplateRead : P.EngineeringMetricRead); }
  if (path.startsWith('/platform/tenants')) return [P.PlatformTenantRead, P.PlatformTenantCreate, P.PlatformTenantUpdate, P.PlatformTenantTransferAdmin].some(can);
  const permissions: Partial<Record<WorkspacePath, PermissionCode[]>> = {
    '/profile': [P.AccountProfileRead], '/security': [P.AccountPasswordUpdate, P.AccountSessionRead, P.AccountSessionRevoke, P.AccountSecurityEventRead],
    '/invitations': [P.PlatformInvitationRead], '/access/accounts': [P.PlatformAccountRead], '/access/roles': [P.PlatformRoleRead], '/access/permissions': [P.PlatformPermissionRead], '/access/audit': [P.PlatformAuditRead],
  };
  return permissions[workspacePaths.find((p) => p === path) ?? '/']?.some(can) ?? true;
}

export function workspaceScope(path: string) {
  const tenantId = path.match(new RegExp(`^/t/(${uuid})/`))?.[1];
  const projectId = path.match(new RegExp(`^/t/${uuid}/p/(${uuid})(?:/|$)`))?.[1];
  const kind = projectId ? 'project' : tenantId ? 'tenant' : path.startsWith('/platform/') || path.startsWith('/access/') || path === '/invitations' ? 'platform' : 'account';
  return { kind, tenantId, projectId } as const;
}

export function accountNavigation(path: string, can: (permission: PermissionCode) => boolean): NavigationItem[] {
  return [
    { title: '个人资料', href: '/profile', icon: UserRound, active: path === '/profile' },
    { title: '账号安全', href: '/security', icon: ShieldCheck, active: path === '/security' },
  ].filter((item) => allowedPath(item.href, can));
}

export function platformNavigation(path: string, can: (permission: PermissionCode) => boolean): NavigationGroup[] {
  const groups: NavigationGroup[] = [
    { title: '组织管理', items: [
      { title: '租户管理', href: '/platform/tenants', icon: Building2, active: path.startsWith('/platform/tenants') },
    ] },
    { title: '工程配置', items: [
      { title: '设施类型定义', href: '/platform/engineering/type-definitions', icon: Blocks, active: path === '/platform/engineering/type-definitions' },
      { title: '指标目录', href: '/platform/engineering/metrics', icon: Ruler, active: path === '/platform/engineering/metrics' },
      { title: '单位目录', href: '/platform/engineering/units', icon: Ruler, active: path === '/platform/engineering/units' },
      { title: '公共监测模板', href: '/platform/engineering/monitoring-templates', icon: ClipboardList, active: path === '/platform/engineering/monitoring-templates' },
    ] },
    { title: '设备管理', items: [{title:'公共设备产品',href:'/platform/device-products',icon:Factory,active:path.startsWith('/platform/device-products')}] },
    { title: '账号与权限', items: [
      { title: '账号管理', href: '/access/accounts', icon: Users, active: path === '/access/accounts' },
      { title: '账号开户邀请', href: '/invitations', icon: MailPlus, active: path === '/invitations' },
      { title: '角色管理', href: '/access/roles', icon: KeyRound, active: path === '/access/roles' },
    ] },
    { title: '审计记录', items: [
      { title: '平台审计', href: '/access/audit', icon: ScrollText, active: path === '/access/audit' },
    ] },
  ];
  return groups.map((group) => ({ ...group, items: group.items.filter((item) => allowedPath(item.href, can)) })).filter((group) => group.items.length > 0);
}

export function navigation(path: string, can: (permission: PermissionCode) => boolean = () => true, tenantCan: (permission: PermissionCode) => boolean = () => false, projectCan: (permission: PermissionCode) => boolean = () => false): NavigationGroup[] {
  const { kind, tenantId: tenant, projectId: project } = workspaceScope(path);
  const projectBase = `/t/${tenant}/p/${project}`;
  if (kind === 'project') {
    const groups: NavigationGroup[] = [
      { title: '项目信息', items: [
        { title: projectCan(P.ProjectProfileRead) ? '项目资料' : '本人项目授权', href: `${projectBase}/profile`, icon: FolderKanban, active: path === `${projectBase}/profile` },
      ] },
      { title: '工程资源', items: projectCan(P.ProjectEngineeringResourceRead) ? [{ title: '项目设施', href: `${projectBase}/resources/facilities`, icon: Blocks, active: path.includes('/resources/facilities') }] : [] },
      { title: '设备管理', items: projectCan(P.ProjectDeviceRead)?[{title:'项目设备',href:`${projectBase}/devices`,icon:Cpu,active:path.includes('/devices')||path.includes('/device-outputs')}]:[] },
      { title: '成员与权限', items: [
        ...(projectCan(P.ProjectMemberRead) ? [{ title: '项目成员', href: `${projectBase}/members`, icon: Users, active: path.startsWith(`${projectBase}/members`) }] : []),
        ...(projectCan(P.ProjectRoleRead) ? [{ title: '项目角色', href: `${projectBase}/roles`, icon: KeyRound, active: path.startsWith(`${projectBase}/roles`) }] : []),
        ...(projectCan(P.ProjectPermissionRead) ? [{ title: '项目权限目录', href: `${projectBase}/permissions`, icon: BookOpen, active: path === `${projectBase}/permissions` }] : []),
      ] },
      { title: '审计记录', items: [
        ...(projectCan(P.ProjectAuditRead) ? [{ title: '项目操作记录', href: `${projectBase}/audit`, icon: ScrollText, active: path === `${projectBase}/audit` }] : []),
      ] },
    ];
    return groups.filter((group) => group.items.length > 0);
  }
  if (kind === 'tenant') {
    const groups: NavigationGroup[] = [
      { title: '租户与项目', items: [
        { title: tenantCan(P.TenantProfileRead) ? '租户资料' : '本人租户授权', href: `/t/${tenant}/profile`, icon: Building2, active: path === `/t/${tenant}/profile` },
        ...(tenantCan(P.TenantProjectRead) ? [{ title: '项目治理', href: `/t/${tenant}/projects`, icon: FolderKanban, active: path.startsWith(`/t/${tenant}/projects`) }] : []),
      ] },
      { title: '成员与权限', items: [
        ...(tenantCan(P.TenantMemberRead) ? [{ title: '成员管理', href: `/t/${tenant}/members`, icon: Users, active: path.startsWith(`/t/${tenant}/members`) }] : []),
        ...(tenantCan(P.TenantInvitationRead) ? [{ title: '成员邀请', href: `/t/${tenant}/invitations`, icon: MailPlus, active: path.startsWith(`/t/${tenant}/invitations`) }] : []),
        ...(tenantCan(P.TenantRoleRead) ? [{ title: '租户角色', href: `/t/${tenant}/roles`, icon: KeyRound, active: path.startsWith(`/t/${tenant}/roles`) }] : []),
      ] },
      { title: '审计记录', items: [
        ...(tenantCan(P.TenantAuditRead) ? [{ title: '租户操作记录', href: `/t/${tenant}/audit`, icon: ScrollText, active: path === `/t/${tenant}/audit` }] : []),
      ] },
    ];
    groups.splice(1, 0, { title: '工程资源', items: [
      ...(tenantCan(P.TenantEngineeringResourceRead) ? [{ title: '设施管理', href: `/t/${tenant}/resources/facilities`, icon: Blocks, active: path.includes('/resources/facilities') }] : []),
      ...([P.TenantEngineeringResourceRead,P.TenantProjectRead,P.TenantEngineeringResourceAuthorizationRead].every(tenantCan) ? [{title:'项目资源授权',href:`/t/${tenant}/resource-access`,icon:KeyRound,active:path.includes('/resource-access')}] : []),
    ] });
    groups.splice(2, 0, {title:'设备管理',items:[
      ...([P.TenantDeviceDefinitionRead,P.TenantDeviceCatalogRead].some(tenantCan)?[{title:'设备产品目录',href:`/t/${tenant}/device-products`,icon:Factory,active:path.includes('/device-products')}]:[]),
      ...(tenantCan(P.TenantDeviceRead)?[{title:'设备台账',href:`/t/${tenant}/devices`,icon:Cpu,active:path.includes('/devices')}]:[]),
    ]});
    groups.splice(3, 0, { title: '工程配置', items: [
      ...(tenantCan(P.TenantEngineeringTemplateRead) ? [{ title: '本租户监测模板', href: `/t/${tenant}/engineering/templates`, icon: ClipboardList, active: path.endsWith('/engineering/templates') }] : []),
    ] });
    return groups.filter((group) => group.items.length > 0);
  }
  if (kind === 'platform') return platformNavigation(path, can);
  return [];
}
