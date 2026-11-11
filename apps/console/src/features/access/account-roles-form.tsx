import { useState } from 'react';
import { Search } from 'lucide-react';
import type { ManagedAccount } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Checkbox } from '@infrapro/ui/components/checkbox';
import { Input } from '@infrapro/ui/components/input';
import { Toggle } from '@infrapro/ui/components/toggle';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useAccountRoleOptions } from './use-account-role-options';
import { RoleMutationForm } from './role-mutation-form';

export function AccountRolesForm({ account, complete, cancel, onPendingChange }: { account: ManagedAccount; complete: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const { client, can } = useIdentity();
  const query = useAccountRoleOptions();
  const [original] = useState(account);
  const [ids, setIds] = useState(original.roles.map((role) => role.id));
  const [q, setQ] = useState(''), [selectedOnly, setSelectedOnly] = useState(false);
  const available = [...new Map([...original.roles, ...query.data?.pages.flatMap((page) => page.items) ?? []].map((role) => [role.id, role])).values()];
  const before = original.roles.map((role) => role.id);
  const added = ids.filter((id) => !before.includes(id)), removed = before.filter((id) => !ids.includes(id));
  const names = (values: string[]) => values.map((id) => available.find((role) => role.id === id)?.name ?? id).join('、') || '无';
  const visible = available.filter((role) => (!selectedOnly || ids.includes(role.id)) && `${role.name} ${role.description}`.toLowerCase().includes(q.trim().toLowerCase()));
  return <RoleMutationForm label="保存角色绑定" disabled={!added.length && !removed.length || query.isPending || !!query.error} complete={complete} cancel={cancel} onPendingChange={onPendingChange} submit={(reason) => client.assignRoles(original.id, { expected_version: original.management_version, role_ids: ids, reason })}>
    <div className="space-y-2"><h3 className="text-sm font-semibold">选择绑定角色</h3><p className="text-sm leading-6 text-muted-foreground">为 {original.display_name} 配置本人及平台权限。租户和项目角色请在对应工作空间配置。</p></div>
    <div className="flex flex-wrap items-center gap-2"><div className="relative min-w-0 flex-1"><Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input aria-label="搜索可选角色" placeholder="搜索已加载的角色…" className="pl-9" value={q} onChange={(event) => setQ(event.target.value)} /></div><Toggle pressed={selectedOnly} onPressedChange={setSelectedOnly} variant="outline">仅看已选 ({ids.length})</Toggle></div>
    {query.isPending && <LoadingState label="正在读取角色…" />}{query.error && <ErrorState message={query.error.message} onRetry={() => void query.refetch()} />}
    <div className="space-y-2">{visible.map((role) => {
      const outside = role.permission_codes.some((code) => !can(code));
      const disabled = outside || (role.status !== 'active' && !before.includes(role.id));
      return <label key={role.id} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border p-4 has-[[data-state=checked]]:border-primary/40 has-[[data-state=checked]]:bg-muted/30 has-[:disabled]:cursor-default"><Checkbox className="mt-0.5" checked={ids.includes(role.id)} disabled={disabled} onCheckedChange={(value) => setIds((current) => value === true ? [...new Set([...current, role.id])] : current.filter((id) => id !== role.id))} /><span className="min-w-0 flex-1 space-y-1 text-sm"><span className="flex flex-wrap items-center gap-2 break-words font-medium">{role.name}<Badge variant="outline" className="font-normal">{role.status === 'active' ? `${role.permission_codes.length} 项权限` : '停用'}</Badge></span><span className="block break-words text-muted-foreground">{role.description || '暂无角色说明。'}</span>{role.status === 'disabled' && <span className="block text-xs text-muted-foreground">已停用，不贡献权限。{before.includes(role.id) ? '原绑定可保留或解除。' : '不能新增绑定。'}</span>}{outside && <span className="block text-xs text-muted-foreground">超出本人授权范围，不能修改此项绑定。</span>}</span></label>;
    })}</div>
    {!query.isPending && !query.error && !visible.length && <EmptyState title="没有符合条件的角色" description="调整搜索内容，或关闭仅看已选。" />}
    {query.hasNextPage && <div className="space-y-2"><p className="text-xs text-muted-foreground">还有角色未加载，继续加载后可搜索。</p><Button type="button" variant="outline" disabled={query.isFetching} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? '正在加载…' : '加载更多可选角色'}</Button></div>}
    <section aria-label="角色变更摘要" className="space-y-2 rounded-lg border bg-muted/30 p-4 text-sm"><h3 className="font-medium">变更摘要</h3><p className="break-words">新增：{names(added)}</p><p className="break-words">解除：{names(removed)}</p></section>
    {!ids.length && <FormNotice>保存后将清空本人及平台角色授权，账号仍可登录和退出；租户和项目角色不受本次变更影响。</FormNotice>}
  </RoleMutationForm>;
}
