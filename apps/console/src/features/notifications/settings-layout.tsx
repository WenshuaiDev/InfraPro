import type { ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { AccountCenterLayout } from '../account/account-center-layout';

export function NotificationSettingsLayout({ title, description, invitation, children }: {
  title: string; description: string; invitation?: boolean; children: ReactNode;
}) {
  return <AccountCenterLayout title="通知中心">
    <section className="account-surface max-w-3xl space-y-5">
      <Link to={invitation ? '/notifications/invitations' : '/notifications'} className="inline-flex min-h-10 items-center gap-2 rounded-sm text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft aria-hidden="true" className="size-4" />{invitation ? '返回收到的邀请' : '返回通知中心'}
      </Link>
      <div><h2 className="text-xl font-semibold">{title}</h2><p className="mt-1 text-sm text-muted-foreground">{description}</p></div>
      {children}
    </section>
  </AccountCenterLayout>;
}
