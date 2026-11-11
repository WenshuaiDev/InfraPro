import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { EmptyState, ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { PermissionCodeToggle, PermissionInfo } from './permission-help';
import { groupPermissions } from './permission-groups';

export function AccountPermissions({ id, disabled }: { id: string; disabled: boolean }) {
  const { client, account, can } = useIdentity();
  const [q, setQ] = useState(''), [showCodes, setShowCodes] = useState(false);
  const auth = useQuery({ queryKey: ['access', account?.id, 'authorization', id], queryFn: ({ signal }) => client.authorization(id, signal) });
  const catalog = useQuery({ queryKey: ['access', account?.id, 'permissions'], queryFn: ({ signal }) => client.permissions(signal), enabled: can(P.PlatformPermissionRead) });
  const value = auth.data;
  if (!value) return auth.error ? <ErrorState message={auth.error.message} onRetry={() => void auth.refetch()} /> : <LoadingState label="正在读取有效权限…" />;
  const entries = value.permissions.map((code) => {
    const permission = catalog.data?.find((item) => item.code === code);
    const sources = value.sources[code]?.map((roleId) => value.roles.find((role) => role.id === roleId)?.name ?? '未知角色').join('、') || '暂无来源说明';
    return { code, permission, sources, group: permission?.group ?? '其他权限' };
  }).filter((item) => `${item.code} ${item.permission?.name ?? ''} ${item.permission?.description ?? ''} ${item.sources}`.toLowerCase().includes(q.trim().toLowerCase()));
  return <section aria-label="有效权限及来源" className="space-y-4">
    <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">有效权限及来源</h3><Badge variant="secondary">{value.permissions.length} 项</Badge></div>
    <p className="text-sm leading-6 text-muted-foreground">仅展示本人账号与平台范围的授权，权限由启用角色合并计算。租户和项目权限在对应工作空间查看。</p>
    {disabled && <FormNotice>该账号已停用，当前不能执行操作；以下内容仅供核对保留的角色授权。</FormNotice>}
    {auth.error && <ErrorState message={auth.error.message} onRetry={() => void auth.refetch()} />}
    {catalog.error && <ErrorState title="权限说明读取失败" message={catalog.error.message} onRetry={() => void catalog.refetch()} />}
    {!can(P.PlatformPermissionRead) && <p className="text-sm text-muted-foreground">当前可查看权限编码和来源；中文说明需要权限目录查看权限。</p>}
    {can(P.PlatformPermissionRead) && catalog.isPending && <p role="status" className="text-sm text-muted-foreground">正在读取权限说明…</p>}
    <Input aria-label="搜索有效权限" placeholder="搜索权限名称、编码或来源角色…" value={q} onChange={(event) => setQ(event.target.value)} />
    {catalog.data && <PermissionCodeToggle checked={showCodes} onChange={setShowCodes} />}
    {groupPermissions(entries).map((group) => <section key={group.name} aria-label={`${group.name}权限`} className="rounded-xl border p-3"><div className="flex items-center justify-between gap-3 px-1 pb-2"><h4 className="text-sm font-semibold">{group.name}</h4><Badge variant="secondary" className="font-normal">{group.items.length} 项</Badge></div><ul className="divide-y">{group.items.map((item) => <li key={item.code} className="space-y-2 px-1 py-3 text-sm">{item.permission ? <PermissionInfo permission={item.permission} showCode={showCodes} /> : <code className="break-all">{item.code}</code>}<p className="break-words text-xs text-muted-foreground">来自：{item.sources}</p></li>)}</ul></section>)}
    {!entries.length && <EmptyState title={q ? '没有符合条件的权限' : '暂无有效的平台权限'} description={q ? '试试其他名称、编码或来源角色。' : '检查账号是否绑定了启用且配置了权限的角色。'} />}
  </section>;
}
