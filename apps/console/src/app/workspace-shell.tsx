import type { ReactNode } from 'react';
import { Link, Outlet, useLocation, useNavigate } from '@tanstack/react-router';
import { AdminLayout, PageContent, SettingsLayout, type LinkRenderer } from '@infrapro/layouts';
import { useIdentity } from '@infrapro/api-client/react';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { brand } from '../config/brand';
import { allowedPath, navigation, relativePath, safeReturn } from '../config/navigation';
import { useOperation } from '../features/account/use-operation';

const renderLink: LinkRenderer = ({ href, ...props }) => <Link to={safeReturn(href)} {...props} />;

export function WorkspaceShell({ children }: { children?: ReactNode }) {
  const { account, client, signedOut, can } = useIdentity();
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const navigate = useNavigate();
  const groups = navigation(path, can);
  const logout = useOperation({ mutationFn: () => client.logout(), onSuccess: signedOut });
  if (!account) return null;
  return <AdminLayout brand={brand} groups={groups} user={{ displayName: account.display_name, username: account.username }} userNavigation={groups.flatMap((group) => group.items).filter((item) => item.href === '/profile' || item.href === '/security')} renderLink={renderLink} onNavigate={(href) => { void navigate({ to: safeReturn(href) }); }} onLogout={() => logout.mutate()} logoutPending={logout.isPending} notice={logout.error ? <FormNotice error>{logout.error.message}</FormNotice> : undefined}>
    <PageContent>{allowedPath(path, can) ? (children ?? <Outlet />) : <section className="space-y-3"><h1 className="text-2xl font-semibold">403 · 无权限</h1><p className="text-muted-foreground">当前账号没有访问此页面的权限。请联系维护人员配置角色。</p><Link to="/" className="underline">返回首页</Link></section>}</PageContent>
  </AdminLayout>;
}

export function AccountSettings({ children }: { children: ReactNode }) {
  const path = relativePath(useLocation({ select: (value) => value.pathname }));
  const { can } = useIdentity();
  const items = navigation(path, can).flatMap((group) => group.items).filter((item) => item.href === '/profile' || item.href === '/security');
  return <SettingsLayout navigation={items} renderLink={renderLink}>{children}</SettingsLayout>;
}
