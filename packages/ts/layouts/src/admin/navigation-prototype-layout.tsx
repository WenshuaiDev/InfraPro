// THROWAWAY: three navigation structures on existing routes, selected by ?variant=A/B/C.
import { Fragment, useState, type ComponentProps, type ReactNode } from 'react';
import { ChevronDown, House, Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { SidebarProvider } from '@infrapro/ui/components/sidebar';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@infrapro/ui/components/sheet';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { AdminLayout } from './admin-layout';
import { CommandSearch } from './command-search';
import { ThemeSwitch } from './theme-switch';
import { UserMenu } from './user-menu';
import { PrototypeSwitcher, type PrototypeVariant } from '../shared/prototype-switcher';

type Props = ComponentProps<typeof AdminLayout> & { variant: PrototypeVariant; onVariantChange: (next: PrototypeVariant) => void; scope: string; path: string };

function BrandLink({ props, compact = false }: { props: Props; compact?: boolean }) {
  return props.renderLink({ href: '/', className: 'flex shrink-0 items-center gap-2.5 rounded-md focus-visible:outline-2 focus-visible:outline-ring', children: <><span className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground [&>svg]:size-5">{props.brand.logo}</span><span className={compact ? 'hidden text-base font-semibold sm:inline' : 'text-base font-semibold'}>{props.brand.name}</span><span className="sr-only">返回首页</span></> });
}

function Actions({ props }: { props: Props }) {
  const groups = [...props.groups, { title: '账号', items: props.userNavigation }];
  return <div className="ms-auto flex shrink-0 items-center gap-1 sm:gap-2"><span className="hidden lg:block"><CommandSearch groups={groups} onNavigate={props.onNavigate} /></span><span className="hidden sm:block"><ThemeSwitch /></span><UserMenu user={props.user} navigation={props.userNavigation} renderLink={props.renderLink} onLogout={props.onLogout} busy={props.logoutPending} /></div>;
}

function BusinessNavigation({ props, onSelect }: { props: Props; onSelect?: () => void }) {
  return <nav aria-label="工作空间业务导航" className="space-y-6 p-4">{props.groups.map((group) => <div key={group.title}><p className="mb-2 px-3 text-xs text-muted-foreground">{group.title}</p><div className="space-y-1">{group.items.map((item) => <Fragment key={item.href}>{props.renderLink({ href: item.href, onClick: onSelect, 'aria-current': item.active ? 'page' : undefined, className: `flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-ring ${item.active ? 'bg-accent font-medium text-accent-foreground' : 'hover:bg-accent/60'}`, children: <>{item.icon && <item.icon className="size-4 shrink-0" aria-hidden="true" />}<span>{item.title}</span></> })}</Fragment>)}</div></div>)}</nav>;
}

function NavigationDrawer({ props, children }: { props: Props; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return <Sheet open={open} onOpenChange={setOpen}><SheetTrigger asChild>{children}</SheetTrigger><SheetContent side="left" className="w-80 max-w-[85vw] gap-0 overflow-y-auto"><SheetHeader className="border-b"><SheetTitle>{props.scope}导航</SheetTitle><SheetDescription>选择当前工作空间中的功能</SheetDescription></SheetHeader><BusinessNavigation props={props} onSelect={() => setOpen(false)} /></SheetContent></Sheet>;
}

function HomeLink({ props }: { props: Props }) {
  return props.renderLink({ href: '/', className: 'flex min-h-11 shrink-0 items-center gap-2 rounded-lg px-3 text-sm hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring', 'aria-current': props.path === '/' ? 'page' : undefined, children: <><House className="size-4" aria-hidden="true" /><span className="hidden md:inline">首页</span></> });
}

export function NavigationVariantA(props: Props) {
  const [collapsed, setCollapsed] = useState(false);
  const business = props.groups.length > 0;
  return <div className="min-h-dvh w-full bg-background">
    <header className="sticky top-0 z-30 flex h-[72px] items-center gap-2 border-b bg-background px-4 sm:gap-4 sm:px-6"><BrandLink props={props} compact /><span className="hidden h-6 border-s sm:block" /><div className="min-w-0 w-64 max-w-[52vw] sm:max-w-80">{props.workspaceSwitcher}</div><span className="hidden sm:block"><HomeLink props={props} /></span><Actions props={props} /></header>
    <div className="flex min-h-[calc(100dvh-72px)]">
      {business && !collapsed && <aside className="sticky top-[72px] hidden h-[calc(100dvh-72px)] w-60 shrink-0 overflow-y-auto border-e bg-sidebar pb-32 lg:block"><div className="flex items-center justify-between px-6 pt-5 text-sm font-medium">{props.scope}<Button size="icon" variant="ghost" aria-label="收起业务导航" onClick={() => setCollapsed(true)}><PanelLeftClose className="size-4" /></Button></div><BusinessNavigation props={props} /></aside>}
      <div className="@container/content min-w-0 flex-1 pb-36"><div className={business ? 'flex items-center gap-2 px-4 pt-3' : 'hidden'}><span className="lg:hidden"><NavigationDrawer props={props}><Button variant="outline" size="sm"><Menu className="size-4" />业务导航</Button></NavigationDrawer></span>{collapsed && <Button className="hidden lg:flex" variant="outline" size="sm" onClick={() => setCollapsed(false)}><PanelLeftOpen className="size-4" />展开业务导航</Button>}</div>{props.children}</div>
    </div>
  </div>;
}

export function NavigationVariantB(props: Props) {
  return <div className="min-h-dvh w-full bg-background">
    <header className="sticky top-0 z-30 border-b bg-background"><div className="mx-auto flex h-20 max-w-[1600px] items-center gap-4 px-4 sm:px-8"><BrandLink props={props} compact /><div className="min-w-0 w-72 max-w-[54vw]">{props.workspaceSwitcher}</div><Actions props={props} /></div>
      <nav aria-label="顶部功能导航" className="flex min-h-12 flex-wrap items-center gap-x-3 border-t bg-muted/35 px-4 sm:px-8"><HomeLink props={props} /><span className="mx-1 h-5 border-s" />{props.groups.length ? props.groups.map((group) => <DropdownMenu key={group.title}><DropdownMenuTrigger asChild><Button variant="ghost" className={`h-12 rounded-none border-b-2 ${group.items.some((item) => item.active) ? 'border-primary font-semibold' : 'border-transparent'}`}>{group.title}<ChevronDown className="size-3" /></Button></DropdownMenuTrigger><DropdownMenuContent align="start">{group.items.map((item) => <DropdownMenuItem key={item.href} asChild>{props.renderLink({ href: item.href, 'aria-current': item.active ? 'page' : undefined, children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</DropdownMenuItem>)}</DropdownMenuContent></DropdownMenu>) : props.userNavigation.filter((item) => item.href !== '/').map((item) => <Fragment key={item.href}>{props.renderLink({ href: item.href, className: 'flex min-h-12 items-center gap-2 px-3 text-sm hover:bg-accent', 'aria-current': item.active ? 'page' : undefined, children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</Fragment>)}</nav>
    </header><div className="@container/content min-w-0 px-0 pb-36 pt-3 sm:px-4">{props.children}</div>
  </div>;
}

export function NavigationVariantC(props: Props) {
  const active = props.groups.flatMap((group) => group.items).find((item) => item.active);
  return <div className="min-h-dvh w-full bg-muted/25 px-3 pt-3 sm:px-6 sm:pt-5">
    <header className="sticky top-3 z-30 mx-auto flex min-h-16 max-w-[1440px] items-center gap-2 rounded-2xl border bg-background px-3 shadow-sm sm:gap-4 sm:px-5"><BrandLink props={props} compact /><div className="min-w-0 w-72 max-w-[52vw]">{props.workspaceSwitcher}</div><span className="hidden sm:block"><HomeLink props={props} /></span><Actions props={props} /></header>
    <div className="@container/content mx-auto mt-5 min-h-[calc(100dvh-120px)] max-w-[1440px] rounded-2xl border bg-background pb-36 sm:mt-7">
      {props.groups.length > 0 && <div className="flex flex-wrap items-center gap-3 border-b px-5 py-3"><NavigationDrawer props={props}><Button variant="outline"><Menu className="size-4" />功能导航</Button></NavigationDrawer><span className="text-sm text-muted-foreground">{props.scope}<span className="mx-2">/</span><span className="text-foreground">{active?.title ?? '当前页面'}</span></span></div>}
      {props.children}
    </div>
  </div>;
}

export function NavigationPrototypeLayout(props: Props) {
  const Component = props.variant === 'B' ? NavigationVariantB : props.variant === 'C' ? NavigationVariantC : NavigationVariantA;
  return <SidebarProvider className="block"><a href="#main" className="sr-only fixed start-4 top-4 z-50 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only">跳到主要内容</a><Component {...props} />{props.notice}<PrototypeSwitcher current={props.variant} onChange={props.onVariantChange} state={`当前：${props.scope} · ${props.groups.reduce((sum, group) => sum + group.items.length, 0)} 个业务入口 · ${props.variant === 'A' && props.groups.length ? '桌面侧栏' : props.variant === 'B' ? '顶部导航' : props.variant === 'C' && props.groups.length ? '导航抽屉' : '无侧栏'} · 业务只读`} /></SidebarProvider>;
}
