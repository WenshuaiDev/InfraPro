import { ChevronsUpDown, LogOut } from 'lucide-react';
import { Avatar, AvatarFallback } from '@infrapro/ui/components/avatar';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { SidebarMenuButton, useSidebar } from '@infrapro/ui/components/sidebar';
import type { LayoutUser, LinkRenderer, NavigationItem } from '../shared/types';

export function UserMenu({ user, navigation, renderLink, onLogout, busy, sidebar = false }: { user: LayoutUser; navigation: NavigationItem[]; renderLink: LinkRenderer; onLogout: () => void; busy?: boolean; sidebar?: boolean }) {
  const { isMobile } = useSidebar();
  const initials = Array.from(user.displayName).slice(0, 2).join('');
  const avatar = <Avatar className="size-8 rounded-lg"><AvatarFallback className="rounded-lg text-xs">{initials}</AvatarFallback></Avatar>;
  return <DropdownMenu><DropdownMenuTrigger asChild>{sidebar ? <SidebarMenuButton size="lg" aria-label="侧栏账号菜单" className="data-[state=open]:bg-sidebar-accent">{avatar}<div className="grid flex-1 text-start text-sm leading-tight"><span className="truncate font-semibold">{user.displayName}</span><span className="truncate text-xs text-muted-foreground">{user.username}</span></div><ChevronsUpDown className="ms-auto size-4" /></SidebarMenuButton> : <Button variant="ghost" className="size-9 rounded-full p-0" aria-label="账号菜单">{avatar}</Button>}</DropdownMenuTrigger>
    <DropdownMenuContent className="w-56" side={sidebar && !isMobile ? 'right' : 'bottom'} align="end" sideOffset={4}>
      <DropdownMenuLabel><p className="truncate font-medium">{user.displayName}</p><p className="truncate text-xs font-normal text-muted-foreground">{user.username}</p></DropdownMenuLabel><DropdownMenuSeparator />
      {navigation.map((item) => <DropdownMenuItem key={item.href} asChild>{renderLink({ href: item.href, children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</DropdownMenuItem>)}
      <DropdownMenuSeparator /><DropdownMenuItem variant="destructive" disabled={busy} onSelect={onLogout}><LogOut className="size-4" />{busy ? '正在退出…' : '退出登录'}</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
}
