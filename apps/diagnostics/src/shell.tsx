import { useEffect, useRef } from 'react';
import { Outlet, Link, useLocation, useNavigate } from '@tanstack/react-router';
import { Activity, Boxes, Clock3, HeartPulse, Server, ShieldCheck } from 'lucide-react';
import { AdminLayout, PageContent, type NavigationGroup } from '@infrapro/layouts';
import { useSession } from './hooks';
import { Login } from './login';
import { RegionBoundary } from './patterns';
export function Shell() {
  const { session, loading, logout } = useSession(); const location = useLocation(); const navigate = useNavigate();
  const authenticated = !!session;
  const content = useRef<HTMLDivElement>(null);
  useEffect(() => { if (authenticated) content.current?.focus({ preventScroll: true }); }, [location.pathname, authenticated]);
  if (loading) return <main className="grid min-h-svh place-items-center"><p>正在确认独立诊断会话…</p></main>;
  if (!session) return <Login />;
  const groups: NavigationGroup[] = [{ title: '独立运行诊断', items: [
    { title: '运行概览', href: '/', icon: Activity }, { title: '运行组件', href: '/components', icon: Boxes }, { title: '节点与资源', href: '/nodes', icon: Server }, { title: '有限历史', href: '/history', icon: Clock3 }, { title: '诊断自身', href: '/self', icon: HeartPulse },
  ].map((item) => ({ ...item, active: item.href === '/' ? location.pathname === '/' : location.pathname.startsWith(item.href) })) }];
  return <AdminLayout mode="workspace" brand={{ name: 'InfraPro 诊断', description: '独立运维 · 只读证据', logo: <ShieldCheck aria-hidden="true" /> }} groups={groups} user={{ displayName: session.user.username, username: session.user.username }} userNavigation={[]} renderLink={({ href, ...props }) => <Link to={href} {...props} />} onNavigate={(href) => { void navigate({ to: href }); }} onLogout={() => { void logout(); }}><PageContent><div id="diagnostics-content" ref={content} tabIndex={-1} className="focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"><RegionBoundary key={location.pathname}><Outlet /></RegionBoundary></div></PageContent></AdminLayout>;
}
