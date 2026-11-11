import { projectContextKey } from '../features/projects/scope';
import { useCallback, useEffect, useState, useSyncExternalStore, type ReactNode } from 'react';
import { Link, Outlet, useLocation, useNavigate } from '@tanstack/react-router';
import { AdminLayout, NavigationPrototypeLayout, PageContent, SettingsLayout, type LinkRenderer } from '@infrapro/layouts';
import { useIdentity } from '@infrapro/api-client/react';
import type { ProjectContext, TenantContext } from '@infrapro/api-client';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { brand } from '../config/brand';
import { accountNavigation, allowedPath, navigation, relativePath, safeReturn, workspaceScope } from '../config/navigation';
import { useOperation } from '../features/account/use-operation';
import { notifyManager, useQueryClient } from '@tanstack/react-query';
import { House } from 'lucide-react';
import { WorkspaceSwitcher } from './workspace-switcher';
import { WorkspaceSwitcherPrototype } from './workspace-switcher-prototype';

const renderLink: LinkRenderer = ({ href, ...props }) => <Link to={safeReturn(href)} {...props} />;

function useScopeState<T>(key: readonly unknown[]) {
  const cache = useQueryClient();
  const subscribe = useCallback((onChange: () => void) => cache.getQueryCache().subscribe(notifyManager.batchCalls(onChange)), [cache]);
  // Route boundaries own context requests and disposal. Subscribe to the cache
  // key so navigation follows a replaced query after scope cleanup as well.
  return useSyncExternalStore(subscribe, () => cache.getQueryState<T>(key));
}

export function WorkspaceShell({ children }: { children?: ReactNode }) {
  const { account, client, signedOut, can } = useIdentity();
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const prototypeSearch = useLocation({ select: (value) => value.search });
  const navigate = useNavigate();
  const { tenantId, projectId } = workspaceScope(path);
  const [restrictedTenant, setRestrictedTenant] = useState<string>();
  useEffect(() => client.onTenantRestriction((id, code) => { if (code === 'tenant_disabled' || code === 'tenant_not_found') setRestrictedTenant(id); else if (code === 'active') setRestrictedTenant((old) => old === id ? undefined : old); }), [client]);
  const tenant = useScopeState<TenantContext>(['tenant', account?.id, tenantId, 'context']);
  const project = useScopeState<ProjectContext>(projectContextKey(account?.id, tenantId ?? '', projectId ?? ''));
  const groups = navigation(path, can, (p) => restrictedTenant !== tenantId && !tenant?.error && (tenant?.data?.permissions.includes(p) ?? false), (p) => restrictedTenant !== tenantId && !project?.error && (project?.data?.authorization.permissions.includes(p) ?? false));
  const logout = useOperation({ mutationFn: () => client.logout(), onSuccess: signedOut });
  if (!account) return null;
  if (import.meta.env.DEV && import.meta.env.VITE_NAVIGATION_PROTOTYPE === '1') {
    const kind = workspaceScope(path).kind;
    const scope = { account: '个人空间', tenant: '租户工作空间', project: '项目工作空间', platform: '平台工作空间' }[kind];
    return <NavigationPrototypeLayout brand={brand} path={path} scope={scope} variant={prototypeSearch.variant ?? 'A'} onVariantChange={(variant) => { void navigate({ to: path, search: { ...prototypeSearch, variant }, replace: true }); }} groups={kind === 'account' ? [] : groups} workspaceSwitcher={<WorkspaceSwitcherPrototype key={account.id} path={path} tenant={!tenant?.error && restrictedTenant !== tenantId ? tenant?.data : undefined} project={!project?.error && restrictedTenant !== tenantId ? project?.data : undefined} />} user={{ displayName: account.display_name, username: account.username }} userNavigation={[{ title: '首页', href: '/', icon: House, active: path === '/' }, ...accountNavigation(path, can)]} renderLink={renderLink} onNavigate={(href) => { void navigate({ to: safeReturn(href) }); }} onLogout={() => { window.alert('导航原型仅供浏览，请在原应用中退出登录。'); }}><PageContent>{allowedPath(path, can) ? (children ?? <Outlet />) : <p>当前账号没有访问此页面的权限。</p>}</PageContent></NavigationPrototypeLayout>;
  }
  return <AdminLayout brand={brand} groups={groups} workspaceSwitcher={<WorkspaceSwitcher key={account.id} path={path} tenant={!tenant?.error && restrictedTenant !== tenantId ? tenant?.data : undefined} project={!project?.error && restrictedTenant !== tenantId ? project?.data : undefined} />} user={{ displayName: account.display_name, username: account.username }} userNavigation={[{ title: '账号首页', href: '/', icon: House, active: path === '/' }, ...accountNavigation(path, can)]} renderLink={renderLink} onNavigate={(href) => { void navigate({ to: safeReturn(href) }); }} onLogout={() => logout.mutate()} logoutPending={logout.isPending} notice={logout.error ? <FormNotice error>{logout.error.message}</FormNotice> : undefined}>
    <PageContent>{allowedPath(path, can) ? (children ?? <Outlet />) : <section className="space-y-3"><h1 className="text-2xl font-semibold">403 · 无权限</h1><p className="text-muted-foreground">当前账号没有访问此页面的权限。请联系维护人员配置角色。</p><Link to="/" className="underline">返回首页</Link></section>}</PageContent>
  </AdminLayout>;
}

export function AccountSettings({ children }: { children: ReactNode }) {
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const { can } = useIdentity();
  const items = accountNavigation(path, can);
  return <SettingsLayout navigation={items} renderLink={renderLink}>{children}</SettingsLayout>;
}
