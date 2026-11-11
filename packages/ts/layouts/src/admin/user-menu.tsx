import { LogOut } from 'lucide-react';
import { Avatar, AvatarFallback } from '@infrapro/ui/components/avatar';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import type { LayoutUser, LinkRenderer, NavigationItem } from '../shared/types';

export function UserMenu({ user, navigation, renderLink, onLogout, busy }: { user: LayoutUser; navigation: NavigationItem[]; renderLink: LinkRenderer; onLogout: () => void; busy?: boolean }) {
  const initials = Array.from(user.displayName).slice(0, 2).join('');
  const avatar = <Avatar className="size-8 rounded-lg"><AvatarFallback className="rounded-lg text-xs">{initials}</AvatarFallback></Avatar>;
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" className="size-9 rounded-full p-0" aria-label="账号菜单">{avatar}</Button></DropdownMenuTrigger>
    <DropdownMenuContent className="w-56" side="bottom" align="end" sideOffset={4}>
      <DropdownMenuLabel><p className="truncate font-medium">{user.displayName}</p><p className="truncate text-xs font-normal text-muted-foreground">{user.username}</p></DropdownMenuLabel><DropdownMenuSeparator />
      {navigation.map((item) => <DropdownMenuItem key={item.href} asChild>{renderLink({ href: item.href, children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</DropdownMenuItem>)}
      <DropdownMenuSeparator /><DropdownMenuItem variant="destructive" disabled={busy} onSelect={onLogout}><LogOut className="size-4" />{busy ? '正在退出…' : '退出登录'}</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
}
