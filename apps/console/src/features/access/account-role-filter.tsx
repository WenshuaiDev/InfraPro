import { PlusCircle } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { useAccountRoleOptions } from './use-account-role-options';

export function AccountRoleFilter({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const query = useAccountRoleOptions();
  const roles = query.data?.pages.flatMap((page) => page.items) ?? [];
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="h-9 max-w-full border-dashed max-sm:h-11" aria-label="绑定角色"><PlusCircle aria-hidden="true" />角色{value && <><span className="mx-1 h-4 border-l" /><Badge variant="secondary" className="min-w-0 max-w-40"><span className="truncate">{roles.find((role) => role.id === value)?.name ?? '已选角色'}</span></Badge></>}</Button></DropdownMenuTrigger>
    <DropdownMenuContent align="start" className="max-h-80 w-72 max-w-[calc(100vw-2rem)] overflow-y-auto">
      <DropdownMenuLabel>绑定的平台角色</DropdownMenuLabel><DropdownMenuSeparator />
      <DropdownMenuRadioGroup value={value} onValueChange={onChange}><DropdownMenuRadioItem value="">全部角色</DropdownMenuRadioItem>{roles.map((role) => <DropdownMenuRadioItem key={role.id} value={role.id} className="items-start whitespace-normal break-words">{role.name}{role.status === 'disabled' ? '（已停用）' : ''}</DropdownMenuRadioItem>)}</DropdownMenuRadioGroup>
      {query.isPending && <p role="status" className="px-2 py-3 text-sm text-muted-foreground">正在读取角色…</p>}
      {query.error && <><p role="alert" className="px-2 py-3 text-sm text-destructive">角色读取失败，请重试。</p><DropdownMenuItem onSelect={(event) => { event.preventDefault(); void query.refetch(); }}>重新加载角色</DropdownMenuItem></>}
      {query.hasNextPage && <><DropdownMenuSeparator /><DropdownMenuItem disabled={query.isFetching} onSelect={(event) => { event.preventDefault(); void query.fetchNextPage(); }}>{query.isFetchingNextPage ? '正在加载…' : '加载更多角色'}</DropdownMenuItem></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
