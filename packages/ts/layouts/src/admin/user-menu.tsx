import { LogOut } from 'lucide-react';
import { AccountAvatar } from '@infrapro/ui/patterns/account-avatar';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import type { LayoutUser, LinkRenderer, NavigationItem } from '../shared/types';

export function UserMenu({ user, navigation, renderLink, onLogout, busy }: { user: LayoutUser; navigation: NavigationItem[]; renderLink: LinkRenderer; onLogout: () => void; busy?: boolean }) {
  const avatar = <AccountAvatar name={user.displayName} src={user.avatarUrl} className="size-8" />;
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" className="size-11 rounded-lg p-0" aria-label="账号菜单">{avatar}</Button></DropdownMenuTrigger>
    <DropdownMenuContent className="w-56" side="bottom" align="end" sideOffset={4}>
      <DropdownMenuLabel className="flex items-center gap-3 py-3">{avatar}<div className="min-w-0 flex-1"><p className="truncate font-medium">{user.displayName}</p><p className="truncate text-xs font-normal text-muted-foreground">{user.username}</p></div></DropdownMenuLabel><DropdownMenuSeparator />
      {navigation.map((item) => <DropdownMenuItem key={item.href} asChild>{renderLink({ href: item.href, children: <>{item.icon && <item.icon className="size-4" />}{item.title}</> })}</DropdownMenuItem>)}
      <DropdownMenuSeparator /><DropdownMenuItem variant="destructive" disabled={busy} onSelect={onLogout}><LogOut className="size-4" />{busy ? '正在退出…' : '退出登录'}</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
}
