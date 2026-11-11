import { useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef, type VisibilityState } from '@tanstack/react-table';
import { CheckCircle2, CircleDashed, Eye, MoreHorizontal, ShieldCheck, SlidersHorizontal, UserCheck, UserRoundX } from 'lucide-react';
import { Permissions as P, type ManagedAccount } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Avatar, AvatarFallback } from '@infrapro/ui/components/avatar';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import type { AccountMode } from './account-panel';

type OpenAccount = (account: ManagedAccount, mode: AccountMode, trigger: HTMLElement | null) => void;

export function AccountStatus({ status }: { status: string }) {
  const Icon = status === 'active' ? CheckCircle2 : CircleDashed;
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-sm"><Icon aria-hidden="true" className="size-4 text-muted-foreground" />{status === 'active' ? '正常' : '停用'}</span>;
}

export function AccountAvatar({ name }: { name: string }) {
  return <Avatar aria-hidden="true" className="size-9 rounded-lg"><AvatarFallback className="rounded-lg text-xs font-medium">{Array.from(name).slice(0, 2).join('').toUpperCase()}</AvatarFallback></Avatar>;
}

function AccountActions({ account, open }: { account: ManagedAccount; open: OpenAccount }) {
  const { can, canAll } = useIdentity();
  const trigger = useRef<HTMLButtonElement>(null), opening = useRef(false);
  const select = (mode: AccountMode) => { opening.current = true; open(account, mode, trigger.current); };
  const canAssign = canAll(P.PlatformRoleRead, P.PlatformAccountAssignRoles);
  const canStatus = can(account.status === 'active' ? P.PlatformAccountDisable : P.PlatformAccountEnable);
  const StatusIcon = account.status === 'active' ? UserRoundX : UserCheck;
  return <DropdownMenu><DropdownMenuTrigger asChild><Button ref={trigger} id={`account-${account.id}-actions`} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${account.display_name}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" onCloseAutoFocus={(event) => { if (opening.current) { event.preventDefault(); opening.current = false; } }}>
      <DropdownMenuLabel>账号操作</DropdownMenuLabel><DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => select('details')}><Eye aria-hidden="true" />查看详情</DropdownMenuItem>
      {canAssign && <DropdownMenuItem onSelect={() => select('roles')}><ShieldCheck aria-hidden="true" />配置角色</DropdownMenuItem>}
      {canStatus && <><DropdownMenuSeparator /><DropdownMenuItem className={account.status === 'active' ? 'text-destructive focus:text-destructive' : undefined} onSelect={() => select('status')}><StatusIcon aria-hidden="true" />{account.status === 'active' ? '停用账号' : '恢复账号'}</DropdownMenuItem></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}

// Stable cell components keep the original trigger mounted during query refreshes.
function AccountCell({ column, account, open, showRoles }: { column: string; account: ManagedAccount; open: OpenAccount; showRoles: boolean }) {
  switch (column) {
    case 'account': return <div className="flex min-w-0 items-center gap-3"><div className="hidden sm:block"><AccountAvatar name={account.display_name} /></div><div className="min-w-0 flex-1 space-y-1"><Button id={`account-${account.id}-name`} variant="link" className="h-auto max-w-full justify-start truncate p-0 text-left font-medium" title={account.display_name} onClick={(event) => open(account, 'details', event.currentTarget)}><span className="truncate">{account.display_name}</span></Button><p className="truncate text-xs text-muted-foreground" title={account.username}>{account.username}</p>{showRoles && <p className="truncate text-xs text-muted-foreground lg:hidden">{account.roles.length ? account.roles.map((role) => `${role.name}${role.status === 'disabled' ? '（停用）' : ''}`).join('、') : '未绑定平台角色'}</p>}</div></div>;
    case 'status': return <AccountStatus status={account.status} />;
    case 'roles': return <div className="flex min-w-0 flex-wrap gap-1.5">{account.roles.slice(0, 2).map((role) => <Badge key={role.id} variant="secondary" className="max-w-full font-normal" title={`${role.name}${role.status === 'disabled' ? '（已停用，不贡献权限）' : ''}`}><span className="truncate">{role.name}{role.status === 'disabled' ? '（停用）' : ''}</span></Badge>)}{account.roles.length > 2 && <Button variant="ghost" className="h-6 px-1.5 text-xs" aria-label={`查看全部 ${account.roles.length} 个绑定角色`} onClick={(event) => open(account, 'details', event.currentTarget)}>+{account.roles.length - 2}</Button>}{!account.roles.length && <span className="text-sm text-muted-foreground">未绑定</span>}</div>;
    case 'created': return <time dateTime={account.created_at} title={new Date(account.created_at).toLocaleString('zh-CN')} className="text-sm tabular-nums text-muted-foreground">{new Date(account.created_at).toLocaleDateString('zh-CN')}</time>;
    case 'actions': return <AccountActions account={account} open={open} />;
    default: return null;
  }
}

export function AccountTable({ items, open, selectedId, toolbar, loading, fetching, error, retry }: { items: ManagedAccount[]; open: OpenAccount; selectedId?: string; toolbar: ReactNode; loading: boolean; fetching: boolean; error?: Error | null; retry: () => void }) {
  const [visibility, setVisibility] = useState<VisibilityState>({});
  const columns: ColumnDef<ManagedAccount>[] = [
    { id: 'account', header: '账号', size: 340, enableHiding: false },
    { id: 'status', header: '状态', size: 120, enableHiding: false },
    { id: 'roles', header: '绑定角色', size: 320 },
    { id: 'created', header: '创建时间', size: 180 },
    { id: 'actions', header: () => <span className="sr-only">操作</span>, enableHiding: false },
  ];
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({ data: items, columns, getRowId: (row) => row.id, getCoreRowModel: getCoreRowModel(), state: { columnVisibility: visibility }, onColumnVisibilityChange: setVisibility });
  const totalSize = table.getVisibleLeafColumns().filter((column) => column.id !== 'actions').reduce((total, column) => total + column.getSize(), 0);
  const style = (id: string, size: number): CSSProperties | undefined => id === 'actions' ? undefined : { '--column-width': `${size / totalSize * 100}%` } as CSSProperties;
  const classes = (id: string) => id === 'account' ? 'lg:w-(--column-width) lg:px-4' : id === 'actions' ? 'w-14 text-right' : id === 'status' ? 'w-20 lg:w-(--column-width) lg:px-4' : 'hidden lg:table-cell lg:w-(--column-width) lg:px-4';
  return <section aria-label="账号列表" className="space-y-4">
    <div className="flex flex-wrap items-center gap-2">{toolbar}<DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="ml-auto hidden h-9 lg:inline-flex"><SlidersHorizontal aria-hidden="true" />显示列</Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuLabel>显示列</DropdownMenuLabel><DropdownMenuSeparator />{table.getAllColumns().filter((column) => column.getCanHide()).map((column) => <DropdownMenuCheckboxItem key={column.id} checked={column.getIsVisible()} onCheckedChange={(value) => column.toggleVisibility(value)}>{String(column.columnDef.header)}</DropdownMenuCheckboxItem>)}</DropdownMenuContent></DropdownMenu></div>
    {error && <ErrorState message={error.message} onRetry={retry} />}
    <div className="overflow-hidden rounded-lg border" aria-busy={loading || fetching}>
      <Table aria-label="账号列表" className="table-fixed"><TableHeader>{table.getHeaderGroups().map((group) => <TableRow key={group.id}>{group.headers.map((header) => <TableHead key={header.id} className={classes(header.id)} style={style(header.id, header.getSize())}>{flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader>
        <TableBody>{!loading && table.getRowModel().rows.map((row) => <TableRow key={row.id} data-state={row.id === selectedId ? 'selected' : undefined}>{row.getVisibleCells().map((cell) => <TableCell key={cell.id} className={`py-3 ${classes(cell.column.id)}`}><AccountCell column={cell.column.id} account={row.original} open={open} showRoles={table.getColumn('roles')?.getIsVisible() ?? false} /></TableCell>)}</TableRow>)}
          {(loading || !items.length) && <TableRow><TableCell colSpan={table.getVisibleLeafColumns().length}>{loading ? <LoadingState label="正在读取账号…" /> : error ? <p className="p-8 text-center text-sm text-muted-foreground">暂时无法读取账号，请重试。</p> : <EmptyState title="没有符合条件的账号" description="试试其他用户名或显示名称，或重置筛选条件。" />}</TableCell></TableRow>}
        </TableBody>
      </Table>
    </div>
  </section>;
}
