import { Link2, Bell, MailPlus, ShieldCheck, UserRound, Users, KeyRound, BookOpen, ScrollText, Building2, FolderKanban, Blocks, Ruler, ClipboardList, Cpu, Factory, Activity, Upload, ListChecks, MapPinned, LayoutDashboard, Files } from 'lucide-react';
import type { NavigationGroup, NavigationItem } from '@infrapro/layouts';
import { Permissions as P, type PermissionCode } from '@infrapro/api-client';

export const workspacePaths = ['/', '/profile', '/security', '/invitations', '/access/accounts', '/access/roles', '/access/permissions', '/access/audit', '/tenants', '/platform/tenants', '/projects'] as const;
export type WorkspacePath = typeof workspacePaths[number];
const uuid = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';
export function safeReturn(value: unknown): string {
 if (typeof value !== 'string' || value.length > 2048) return '/';
 if(value==='/platform/parsing'||new RegExp(`^/t/${uuid}/parsing$`).test(value))return value;
 if (new RegExp(`^/t/${uuid}/bindings$`).test(value)) return value;
 if (new RegExp(`^/t/${uuid}/(?:p/${uuid}/)?files$`).test(value)) return value;
 if (new RegExp(`^/tasks(?:/${uuid})?$`).test(value)) return value;
 if (new RegExp(`^/notifications(?:/(?:preferences|invitations(?:/${uuid})?|${uuid}))?$`).test(value)) return value;
 if(value==='/display-launch'||new RegExp(`^/t/${uuid}/p/${uuid}/displays$`).test(value))return value;
 if(new RegExp(`^/t/${uuid}/(?:p/${uuid}/)?spatial$`).test(value)||new RegExp(`^/t/${uuid}/projects/${uuid}/spatial-access$`).test(value))return value;
 if (new RegExp(`^/t/${uuid}/(?:p/${uuid}/)?monitoring(?:/imports(?:/${uuid})?|/batches/${uuid}|/jobs|/derived(?:/${uuid})?|/derived-definitions|/derived-runs|/analysis(?:/${uuid})?)?$`).test(value) || new RegExp(`^/t/${uuid}/projects/${uuid}/monitoring-access$`).test(value)) return value;
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
  if (path.startsWith('/platform/parsing')) return can(P.ParsingMethodRead)||can(P.ParsingRunRead);
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
    { title: '通知中心', href: '/notifications', icon: Bell, active: path.startsWith('/notifications') },
    { title: '个人资料', href: '/profile', icon: UserRound, active: path === '/profile' },
    { title: '账号安全', href: '/security', icon: ShieldCheck, active: path === '/security' },
  ].filter((item) => allowedPath(item.href, can));
}

function navigationSection(title: string, icon: NavigationItem['icon'], items: NavigationItem[]): NavigationGroup['items'] {
  return items.length > 1 ? [{ title, icon, items }] : items;
}

export function platformNavigation(path: string, can: (permission: PermissionCode) => boolean): NavigationGroup[] {
  const groups: NavigationGroup[] = [
    { title: '组织管理', items: [
      { title: '租户管理', href: '/platform/tenants', icon: Building2, active: path.startsWith('/platform/tenants') },
    ] },
    { title: '监测业务', items: [{ title: '工程配置', icon: Blocks, items: [
      { title: '设施类型定义', href: '/platform/engineering/type-definitions', icon: Blocks, active: path === '/platform/engineering/type-definitions' },
      { title: '指标目录', href: '/platform/engineering/metrics', icon: Ruler, active: path === '/platform/engineering/metrics' },
      { title: '单位目录', href: '/platform/engineering/units', icon: Ruler, active: path === '/platform/engineering/units' },
      { title: '公共监测模板', href: '/platform/engineering/monitoring-templates', icon: ClipboardList, active: path === '/platform/engineering/monitoring-templates' },
    ] }, {title:'解析方法',href:'/platform/parsing',icon:Cpu,active:path.startsWith('/platform/parsing')}, {title:'公共设备产品',href:'/platform/device-products',icon:Factory,active:path.startsWith('/platform/device-products')}] },
    { title: '账号与权限', items: [
      { title: '账号管理', href: '/access/accounts', icon: Users, active: path === '/access/accounts' },
      { title: '账号开户邀请', href: '/invitations', icon: MailPlus, active: path === '/invitations' },
      { title: '角色管理', href: '/access/roles', icon: KeyRound, active: path === '/access/roles' },
    ] },
    { title: '审计记录', items: [
      { title: '平台审计', href: '/access/audit', icon: ScrollText, active: path === '/access/audit' },
    ] },
  ];
  return groups.map((group) => ({ ...group, items: group.items.flatMap((item): NavigationGroup['items'] => {
    if (!('items' in item)) return allowedPath(item.href, can) ? [item] : [];
    const visible = item.items.filter((child) => allowedPath(child.href, can));
    return visible.length > 1 ? [{ ...item, items: visible }] : visible;
  }) })).filter((group) => group.items.length > 0);
}

export function navigation(path: string, can: (permission: PermissionCode) => boolean = () => true, tenantCan: (permission: PermissionCode) => boolean = () => false, projectCan: (permission: PermissionCode) => boolean = () => false): NavigationGroup[] {
  const { kind, tenantId: tenant, projectId: project } = workspaceScope(path);
  const projectBase = `/t/${tenant}/p/${project}`;
  if (kind === 'project') {
    const groups: NavigationGroup[] = [
      { title: '项目信息', items: [
        ...([P.ProjectProfileRead,P.ProjectAttachmentsRead].every(projectCan)?[{title:'文件与附件',href:`${projectBase}/files`,icon:Files,active:path===`${projectBase}/files`}]:[]),
        { title: projectCan(P.ProjectProfileRead) ? '项目资料' : '本人项目授权', href: `${projectBase}/profile`, icon: FolderKanban, active: path === `${projectBase}/profile` },
      ] },
      { title: '监测业务', items: [
        ...(projectCan(P.ProjectDisplayView)?[{title:'项目大屏',href:`${projectBase}/displays`,icon:LayoutDashboard,active:path===`${projectBase}/displays`}]:[]),
        ...(projectCan(P.ProjectSpatialView)?[{title:'工程空间',href:`${projectBase}/spatial`,icon:MapPinned,active:path===`${projectBase}/spatial`}]:[]),
        ...(projectCan(P.ProjectEngineeringResourceRead) ? [{ title: '项目设施', href: `${projectBase}/resources/facilities`, icon: Blocks, active: path.startsWith(`${projectBase}/resources/facilities`) }] : []),
        ...(projectCan(P.ProjectDeviceRead) ? [{ title: '项目设备', href: `${projectBase}/devices`, icon: Cpu, active: path.startsWith(`${projectBase}/devices`) || path.startsWith(`${projectBase}/device-outputs/`) }] : []),
        ...navigationSection('监测数据', Activity, monitoringNavigation(projectBase, path, projectCan, false)),
      ] },
      { title: '成员与权限', items: [
        ...(projectCan(P.ProjectMemberRead) ? [{ title: '项目成员', href: `${projectBase}/members`, icon: Users, active: path.startsWith(`${projectBase}/members`) }] : []),
        ...(projectCan(P.ProjectRoleRead) ? [{ title: '项目角色', href: `${projectBase}/roles`, icon: KeyRound, active: path.startsWith(`${projectBase}/roles`) }] : []),
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
        ...([P.TenantProfileRead,P.TenantAttachmentsRead].every(tenantCan)?[{title:'文件与附件',href:`/t/${tenant}/files`,icon:Files,active:path===`/t/${tenant}/files`}]:[]),
        { title: tenantCan(P.TenantProfileRead) ? '租户资料' : '本人租户授权', href: `/t/${tenant}/profile`, icon: Building2, active: path === `/t/${tenant}/profile` },
        ...(tenantCan(P.TenantProjectRead) ? [{ title: '项目治理', href: `/t/${tenant}/projects`, icon: FolderKanban, active: path.startsWith(`/t/${tenant}/projects`) }] : []),
      ] },
      { title: '监测业务', items: [
        ...(tenantCan(P.TenantEngineeringResourceRead) ? [{ title: '测点与设备绑定', href: `/t/${tenant}/bindings`, icon: Link2, active: path === `/t/${tenant}/bindings` }] : []),
        ...navigationSection('工程资源', Blocks, [
          ...(tenantCan(P.TenantSpatialRead)?[{title:'工程空间',href:`/t/${tenant}/spatial`,icon:MapPinned,active:path===`/t/${tenant}/spatial`}]:[]),
        ...(tenantCan(P.TenantEngineeringResourceRead) ? [{ title: '设施管理', href: `/t/${tenant}/resources/facilities`, icon: Blocks, active: path.startsWith(`/t/${tenant}/resources/facilities`) }] : []),
          ...([P.TenantEngineeringResourceRead, P.TenantProjectRead, P.TenantEngineeringResourceAuthorizationRead].every(tenantCan) ? [{ title: '项目资源授权', href: `/t/${tenant}/resource-access`, icon: KeyRound, active: path.startsWith(`/t/${tenant}/resource-access`) }] : []),
        ]),
        ...navigationSection('设备管理', Cpu, [
          ...([P.TenantParsingMethodRead,P.TenantParsingRunRead].some(tenantCan)?[{title:'试解析与运行',href:`/t/${tenant}/parsing`,icon:Cpu,active:path.startsWith(`/t/${tenant}/parsing`)}]:[]),
          ...([P.TenantDeviceDefinitionRead, P.TenantDeviceCatalogRead].some(tenantCan) ? [{ title: '设备产品目录', href: `/t/${tenant}/device-products`, icon: Factory, active: path.startsWith(`/t/${tenant}/device-products`) }] : []),
          ...(tenantCan(P.TenantDeviceRead) ? [{ title: '设备台账', href: `/t/${tenant}/devices`, icon: Cpu, active: path.startsWith(`/t/${tenant}/devices`) }] : []),
        ]),
        ...navigationSection('监测数据', Activity, monitoringNavigation(`/t/${tenant}`, path, tenantCan, true)),
        ...(tenantCan(P.TenantEngineeringTemplateRead) ? [{ title: '本租户监测模板', href: `/t/${tenant}/engineering/templates`, icon: ClipboardList, active: path === `/t/${tenant}/engineering/templates` }] : []),
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
    return groups.filter((group) => group.items.length > 0);
  }
  if (kind === 'platform') return platformNavigation(path, can);
  return [];
}

function monitoringNavigation(base: string, path: string, can: (permission: PermissionCode) => boolean, tenant: boolean): NavigationItem[] {
 const read=can(tenant?P.TenantMonitoringDataRead:P.ProjectMonitoringDataRead), imports=can(tenant?P.TenantMonitoringImportsRead:P.ProjectMonitoringImportsRead);
 return [
 ...(read?[{title:"监测浏览",href:`${base}/monitoring`,icon:Activity,active:path===`${base}/monitoring`||path===`${base}/monitoring/`}]:[]),
 ...(imports?[{title:"导入与批次",href:`${base}/monitoring/imports`,icon:Upload,active:path.startsWith(`${base}/monitoring/imports`)||path.startsWith(`${base}/monitoring/batches`)}]:[]),
 ...(read||imports?[{title:"监测任务",href:`${base}/monitoring/jobs`,icon:ListChecks,active:path===`${base}/monitoring/jobs`}]:[]),
 ...(tenant&&can(P.TenantDerivedMetricsRead)?[{title:"派生指标定义",href:`${base}/monitoring/derived-definitions`,icon:BookOpen,active:path===`${base}/monitoring/derived-definitions`}]:[]),
 ...(can(tenant?P.TenantDerivedChannelsRead:P.ProjectDerivedChannelsRead)?[{title:"派生通道",href:`${base}/monitoring/derived`,icon:Activity,active:path===`${base}/monitoring/derived`||path.startsWith(`${base}/monitoring/derived/`)}]:[]),
 ...(can(tenant?P.TenantDerivedAnalysisRead:P.ProjectDerivedAnalysisRead)||can(tenant?P.TenantDerivedAnalysisResultsRead:P.ProjectDerivedAnalysisResultsRead)?[{title:"历史分析",href:`${base}/monitoring/analysis`,icon:ClipboardList,active:path.startsWith(`${base}/monitoring/analysis`)}]:[]),
 ...((can(tenant?P.TenantDerivedRunsRead:P.ProjectDerivedRunsRead)||can(tenant?P.TenantDerivedAnalysisExport:P.ProjectDerivedAnalysisExport))?[{title:"计算运行",href:`${base}/monitoring/derived-runs`,icon:ListChecks,active:path===`${base}/monitoring/derived-runs`}]:[]),
 ];
}
