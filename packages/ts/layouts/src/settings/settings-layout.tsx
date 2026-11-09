import type { ReactNode } from 'react';
import { Separator } from '@infrapro/ui/components/separator';
import { cn } from '@infrapro/ui/lib/utils';
import type { LinkRenderer, NavigationItem } from '../shared/types';

export function SettingsLayout({ navigation, renderLink, children }: { navigation: NavigationItem[]; renderLink: LinkRenderer; children: ReactNode }) {
  return <><Separator className="mb-6" /><div className="flex min-w-0 flex-col gap-6 lg:flex-row lg:gap-12">
    <nav aria-label="账号设置" className="flex gap-1 lg:w-48 lg:shrink-0 lg:flex-col">{navigation.map((item) => <div key={item.href}>{renderLink({ href: item.href, 'aria-current': item.active ? 'page' : undefined, className: cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent', item.active ? 'bg-muted' : 'text-muted-foreground'), children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</div>)}</nav>
    <div className="min-w-0 flex-1 space-y-6">{children}</div>
  </div></>;
}
