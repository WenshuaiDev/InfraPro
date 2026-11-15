import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Permissions as P, type ProjectAuthorization } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { EmptyState, ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { groupPermissions } from '../access/permission-groups';
import { PermissionCodeToggle, PermissionInfo } from '../access/permission-help';
import { useProject } from './context';
import { projectMemberDate } from './member-operations';

const inactiveReasons: Record<string, string> = { tenant_disabled: '租户已停用', account_unavailable: '账号不可用', membership_suspended: '项目成员已暂停', membership_ended: '项目关系已结束', tenant_membership_suspended: '租户成员已暂停', tenant_membership_ended: '租户关系已结束', no_project_permissions: '尚未获项目业务授权' };

export function ProjectMemberAuthorization({ value }: { value: ProjectAuthorization }) {
  return <section aria-label="当前项目授权" className="space-y-4">
    {value.inactive_reasons.map((reason) => <FormNotice key={reason}>{inactiveReasons[reason] ?? reason}</FormNotice>)}
    <Tabs defaultValue="roles" className="gap-4">
      <TabsList className="w-full"><TabsTrigger value="roles" className="flex-1 max-sm:text-xs">绑定角色</TabsTrigger><TabsTrigger value="permissions" className="flex-1 max-sm:text-xs">有效权限</TabsTrigger><TabsTrigger value="history" className="flex-1 max-sm:text-xs">授权历史</TabsTrigger></TabsList>
      <TabsContent value="roles" className="space-y-4">
        <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">绑定的项目角色</h3><Badge variant="secondary">{value.roles.length} 个</Badge></div>
        <p className="text-sm leading-6 text-muted-foreground">项目权限由启用角色合并授予，实际可执行操作还取决于成员资格、项目状态和资源授权。</p>
        <ul className="space-y-3">{value.roles.map((role) => <li key={role.id} className="space-y-2 rounded-lg border p-4"><div className="flex flex-wrap items-start justify-between gap-2"><h4 className="min-w-0 break-words text-sm font-medium">{role.name}</h4><Badge variant="outline">{role.status === 'active' ? '启用' : '停用'}</Badge></div><p className="break-words text-sm text-muted-foreground">{role.description || '暂无角色说明。'}</p><p className="text-xs text-muted-foreground">{role.status === 'disabled' ? '已停用，不贡献权限；原绑定保留。' : `${role.permission_codes.length} 项配置权限`}</p></li>)}</ul>
        {!value.roles.length && <EmptyState title="尚未绑定项目角色" />}
      </TabsContent>
      <TabsContent value="permissions"><ProjectMemberPermissions value={value} /></TabsContent>
      <TabsContent value="history" className="space-y-4"><h3 className="text-sm font-semibold">本次成员关系的角色授予记录</h3><ul className="space-y-3">{value.bindings.map((binding) => <li key={binding.id} className="space-y-2 rounded-lg border p-4"><div className="flex flex-wrap justify-between gap-2"><h4 className="min-w-0 break-words text-sm font-medium">{binding.role_name}</h4><Badge variant="outline">{binding.revoked_at ? '已撤销' : '当前绑定'}</Badge></div><p className="text-xs text-muted-foreground">授予于 {projectMemberDate(binding.assigned_at, true)}</p>{binding.revoked_at && <p className="text-xs text-muted-foreground">撤销于 {projectMemberDate(binding.revoked_at, true)}</p>}</li>)}</ul>{!value.bindings.length && <EmptyState title="暂无角色授予记录" />}</TabsContent>
    </Tabs>
  </section>;
}

function ProjectMemberPermissions({ value }: { value: ProjectAuthorization }) {
  const ctx = useProject(), { client } = useIdentity();
  const [q, setQ] = useState(''), [showCodes, setShowCodes] = useState(false);
  const canReadCatalog = ctx.can(P.ProjectPermissionRead);
  const catalog = useQuery({ queryKey: [...ctx.key, 'permissions'], queryFn: ({ signal }) => client.projectPermissions(ctx.project.tenant_id, ctx.project.id, signal), enabled: canReadCatalog });
  const entries = value.permissions.map((code) => {
    const permission = catalog.data?.find((item) => item.code === code);
    const sources = value.sources[code]?.map((id) => value.roles.find((role) => role.id === id)?.name ?? '未知角色').join('、') || '暂无来源说明';
    return { code, permission, sources, group: permission?.group ?? '其他权限' };
  }).filter((item) => `${item.code} ${item.permission?.name ?? ''} ${item.permission?.description ?? ''} ${item.sources}`.toLowerCase().includes(q.trim().toLowerCase()));
  return <section aria-label="有效权限及来源" className="space-y-4">
    <div className="flex items-center justify-between gap-3"><h3 className="text-sm font-semibold">有效权限及来源</h3><Badge variant="secondary">{value.permissions.length} 项</Badge></div>
    <p className="text-sm leading-6 text-muted-foreground">仅展示当前项目的有效授权；资源和数据的可访问范围还取决于相应对象授权。</p>
    {catalog.error && <ErrorState title="权限说明读取失败" message={catalog.error.message} onRetry={() => void catalog.refetch()} />}
    {!canReadCatalog && <p className="text-sm text-muted-foreground">当前可查看权限编码和来源；中文说明需要权限目录查看权限。</p>}
    {canReadCatalog && catalog.isPending && <p role="status" className="text-sm text-muted-foreground">正在读取权限说明…</p>}
    <Input aria-label="搜索有效权限" placeholder="搜索权限名称、编码或来源角色…" maxLength={200} value={q} onChange={(event) => setQ(event.target.value)} />
    {catalog.data && <PermissionCodeToggle checked={showCodes} onChange={setShowCodes} />}
    {groupPermissions(entries).map((group) => <section key={group.name} aria-label={`${group.name}权限`} className="rounded-xl border p-3"><div className="flex items-center justify-between gap-3 px-1 pb-2"><h4 className="text-sm font-semibold">{group.name}</h4><Badge variant="secondary" className="font-normal">{group.items.length} 项</Badge></div><ul className="divide-y">{group.items.map((item) => <li key={item.code} className="space-y-2 px-1 py-3 text-sm">{item.permission ? <PermissionInfo permission={item.permission} showCode={showCodes} /> : <code className="break-all">{item.code}</code>}<p className="break-words text-xs text-muted-foreground">来自：{item.sources}</p></li>)}</ul></section>)}
    {!entries.length && <EmptyState title={q ? '没有符合条件的权限' : '暂无有效的项目权限'} description={q ? '试试其他名称、编码或来源角色。' : '检查成员资格及绑定角色的状态。'} />}
  </section>;
}
