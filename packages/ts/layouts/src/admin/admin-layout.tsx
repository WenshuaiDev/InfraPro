import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Separator } from '@infrapro/ui/components/separator';
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger, useSidebar } from '@infrapro/ui/components/sidebar';
import { useLayoutStore } from '../state/layout-store';
import type { Brand, LayoutUser, LinkRenderer, NavigationGroup, NavigationItem } from '../shared/types';
import { CommandSearch } from './command-search';
import { ThemeSwitch } from './theme-switch';
import { UserMenu } from './user-menu';

type Props = { sidebarControl?: { open: boolean; onOpenChange: (open: boolean) => void }; brand: Brand; groups: NavigationGroup[]; user: LayoutUser; userNavigation: NavigationItem[]; renderLink: LinkRenderer; onNavigate: (href: string) => void; onLogout: () => void; logoutPending?: boolean; notice?: ReactNode; workspaceSwitcher?: ReactNode; children: ReactNode };

function AppSidebar({ brand, groups, user, userNavigation, renderLink, onLogout, logoutPending, workspaceSwitcher }: Props) {
  const { isMobile, setOpenMobile } = useSidebar();
  return <Sidebar variant="inset" collapsible="icon">
    <SidebarHeader><SidebarMenu><SidebarMenuItem><div className="flex min-w-0 items-center">{workspaceSwitcher ?? <SidebarMenuButton size="lg" asChild>{renderLink({ href: '/', onClick: () => setOpenMobile(false), children: <><div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground [&>svg]:size-5">{brand.logo}</div><div className="grid flex-1 text-start text-sm leading-tight"><span className="truncate font-semibold">{brand.name}</span><span className="truncate text-xs">{brand.description}</span></div></> })}</SidebarMenuButton>}{isMobile && <Button size="icon" variant="ghost" aria-label="关闭导航" onClick={() => setOpenMobile(false)}><X className="size-4" /></Button>}</div></SidebarMenuItem></SidebarMenu></SidebarHeader>
    <SidebarContent className="group-data-[collapsible=icon]:overflow-y-auto">
      <nav aria-label="主导航" className="flex flex-col gap-5 px-2 py-4 group-data-[collapsible=icon]:gap-4 group-data-[collapsible=icon]:py-2">
        {groups.map((group) => <SidebarGroup key={group.title} role="group" aria-label={group.title} className="p-0">
          <SidebarGroupLabel className="mb-1 px-3 group-data-[collapsible=icon]:mb-0">{group.title}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu className="gap-2 md:gap-1">
              {group.items.map((item) => <SidebarMenuItem key={item.href}>
                <SidebarMenuButton asChild isActive={item.active} tooltip={item.title} className="h-11 gap-3 px-3 text-sidebar-foreground md:h-10">
                  {renderLink({ href: item.href, 'aria-current': item.active ? 'page' : undefined, onClick: () => setOpenMobile(false), children: <>{item.icon && <item.icon className="size-4" aria-hidden="true" />}<span>{item.title}</span></> })}
                </SidebarMenuButton>
              </SidebarMenuItem>)}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>)}
      </nav>
    </SidebarContent>
    <SidebarFooter><SidebarMenu><SidebarMenuItem><UserMenu user={user} navigation={userNavigation} renderLink={renderLink} onLogout={onLogout} busy={logoutPending} sidebar /></SidebarMenuItem></SidebarMenu></SidebarFooter><SidebarRail />
  </Sidebar>;
}

export function AdminLayout(props: Props) {
  const sidebarOpen = useLayoutStore((state) => state.sidebarOpen);
  const setSidebarOpen = useLayoutStore((state) => state.setSidebarOpen);
  const accountItems = props.userNavigation.filter((item) => !props.groups.some((group) => group.items.some((entry) => entry.href === item.href)));
  const searchGroups = accountItems.length ? [...props.groups, { title: '账号快捷入口', items: accountItems }] : props.groups;
  return <SidebarProvider open={props.sidebarControl?.open ?? sidebarOpen} onOpenChange={props.sidebarControl?.onOpenChange ?? setSidebarOpen}>
    <a href="#main" className="sr-only fixed start-4 top-4 z-100 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed">跳到主要内容</a>
    <AppSidebar {...props} />
    <SidebarInset className="@container/content min-w-0">
      <header className="sticky top-0 z-30 h-16 border-b border-transparent bg-background/95 backdrop-blur-sm"><div className="flex h-full items-center gap-3 px-4 sm:gap-4"><SidebarTrigger variant="outline" /><Separator orientation="vertical" className="h-6" /><div className="ms-auto flex items-center gap-1 sm:gap-2"><CommandSearch groups={searchGroups} onNavigate={props.onNavigate} /><ThemeSwitch /><UserMenu user={props.user} navigation={props.userNavigation} renderLink={props.renderLink} onLogout={props.onLogout} busy={props.logoutPending} /></div></div></header>
      {props.notice && <div className="px-4 pt-4">{props.notice}</div>}{props.children}
    </SidebarInset>
  </SidebarProvider>;
}
