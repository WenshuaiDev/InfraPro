import { projectContextKey } from '../features/projects/scope';
import { useCallback, useEffect, useState, useSyncExternalStore, type ReactNode } from 'react';
import { Link, Outlet, useLocation, useNavigate } from '@tanstack/react-router';
import { AdminLayout, PageContent, SettingsLayout, type LinkRenderer } from '@infrapro/layouts';
import { useIdentity } from '@infrapro/api-client/react';
import type { ProjectContext, TenantContext } from '@infrapro/api-client';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { brand } from '../config/brand';
import { accountNavigation, allowedPath, navigation, relativePath, safeReturn, workspaceScope } from '../config/navigation';
import { useOperation } from '../features/account/use-operation';
import { notifyManager, useQueryClient } from '@tanstack/react-query';
import { House } from 'lucide-react';
import { WorkspaceSwitcher } from './workspace-switcher';
import { WorkspacePickerProvider, useWorkspacePicker } from './workspace-picker-context';

const renderLink: LinkRenderer = ({ href, ...props }) => <Link to={safeReturn(href)} {...props} />;

function useScopeState<T>(key: readonly unknown[]) {
  const cache = useQueryClient();
  const subscribe = useCallback((onChange: () => void) => cache.getQueryCache().subscribe(notifyManager.batchCalls(onChange)), [cache]);
  // Route boundaries own context requests and disposal. Subscribe to the cache
  // key so navigation follows a replaced query after scope cleanup as well.
  return useSyncExternalStore(subscribe, () => cache.getQueryState<T>(key));
}

export function WorkspaceShell({ children }: { children?: ReactNode }) {
  const { account } = useIdentity();
  if (!account) return null;
  return <WorkspacePickerProvider key={account.id}><WorkspaceContent>{children}</WorkspaceContent></WorkspacePickerProvider>;
}

function WorkspaceContent({ children }: { children?: ReactNode }) {
  const { account, client, signedOut, can } = useIdentity();
  const picker = useWorkspacePicker();
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const navigate = useNavigate();
  const { kind, tenantId, projectId } = workspaceScope(path);
  const [restrictedTenant, setRestrictedTenant] = useState<string>();
  useEffect(() => client.onTenantRestriction((id, code) => { if (code === 'tenant_disabled' || code === 'tenant_not_found') setRestrictedTenant(id); else if (code === 'active') setRestrictedTenant((old) => old === id ? undefined : old); }), [client]);
  const tenant = useScopeState<TenantContext>(['tenant', account?.id, tenantId, 'context']);
  const project = useScopeState<ProjectContext>(projectContextKey(account?.id, tenantId ?? '', projectId ?? ''));
  const groups = navigation(path, can, (p) => restrictedTenant !== tenantId && !tenant?.error && (tenant?.data?.permissions.includes(p) ?? false), (p) => restrictedTenant !== tenantId && !project?.error && (project?.data?.authorization.permissions.includes(p) ?? false));
  const logout = useOperation({ mutationFn: () => client.logout(), onSuccess: signedOut });
  if (!account) return null;
  return <AdminLayout mode={kind === 'account' ? 'plain' : 'workspace'} brand={brand} groups={groups} workspaceSwitcherOpen={picker.state.open} renderWorkspaceSwitcher={(host) => <WorkspaceSwitcher host={host} path={path} tenant={!tenant?.error && restrictedTenant !== tenantId ? tenant?.data : undefined} project={!project?.error && restrictedTenant !== tenantId ? project?.data : undefined} />} user={{ displayName: account.display_name, username: account.username }} userNavigation={[{ title: '账号首页', href: '/', icon: House, active: path === '/' }, ...accountNavigation(path, can)]} renderLink={renderLink} onNavigate={(href) => { void navigate({ to: safeReturn(href) }); }} onLogout={() => logout.mutate()} logoutPending={logout.isPending} notice={logout.error ? <FormNotice error>{logout.error.message}</FormNotice> : undefined}>
    <PageContent>{allowedPath(path, can) ? (children ?? <Outlet />) : <section className="space-y-3"><h1 className="text-2xl font-semibold">403 · 无权限</h1><p className="text-muted-foreground">当前账号没有访问此页面的权限。请联系维护人员配置角色。</p><Link to="/" className="underline">返回首页</Link></section>}</PageContent>
  </AdminLayout>;
}

export function AccountSettings({ children }: { children: ReactNode }) {
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const { can } = useIdentity();
  const items = accountNavigation(path, can);
  return <SettingsLayout navigation={items} renderLink={renderLink}>{children}</SettingsLayout>;
}
