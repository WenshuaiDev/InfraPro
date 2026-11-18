import type { ReactNode } from 'react';
import { Link, useLocation } from '@tanstack/react-router';
import { useIdentity } from '@infrapro/api-client/react';
import { accountNavigation, relativePath } from '../../config/navigation';
import './account-center.css';

export function AccountCenterLayout({ title, children }: { title: string; children: ReactNode }) {
  const { can } = useIdentity();
  const path = relativePath(useLocation({ select: location => location.pathname }));
  return <div className="account-center">
    <h1 className="mb-3 text-[clamp(1.55rem,2.4vw,2rem)] font-semibold tracking-tight">{title}</h1>
    <nav className="account-center-nav" aria-label="账号设置">
      {accountNavigation(path, can).map(({ href, active, title: label, icon: Icon }) => <Link
        key={href} to={href} activeOptions={{ includeSearch: false }} aria-current={active ? 'page' : undefined}
      >{Icon && <Icon aria-hidden="true" />}{label}</Link>)}
    </nav>
    {children}
  </div>;
}
