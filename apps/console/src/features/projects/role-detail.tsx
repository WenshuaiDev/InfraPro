import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useSearch } from '@tanstack/react-router';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Permissions as P, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@infrapro/ui/components/tabs';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { toast } from 'sonner';
import { RoleActions, RoleStatus } from '../access/role-table';
import { ObjectHistory } from '../audit/object-history';
import { useProject } from './context';
import { ProjectRoleForm, type ProjectRoleMode } from './role-form';
import { projectRoleActions, projectRoleTitles } from './role-controls';
import { ProjectRolePermissions } from './role-permissions';

export function ProjectRolePage({ role }: { role: string }) {
  const ctx = useProject(); const { client } = useIdentity(); const navigate = useNavigate();
  const search = useSearch({ from: '/_authenticated/t/$tenantId/p/$projectId/roles' });
  const [mode, setMode] = useState<ProjectRoleMode>(); const [busy, setBusy] = useState(false);
  const detailRef = useRef<HTMLDivElement>(null); const previousMode = useRef(mode);
  useEffect(() => { if (previousMode.current && !mode) detailRef.current?.querySelector<HTMLButtonElement>('[data-role-actions], button')?.focus(); previousMode.current = mode; }, [mode]);
  const { tenant_id: tenantId, id: projectId } = ctx.project; const allowed = ctx.can(P.ProjectRoleRead);
  const state = useQuery({ queryKey: [...ctx.key, 'role', role], queryFn: ({ signal }) => client.projectRole(tenantId, projectId, role, signal), enabled: allowed });
  const leave = () => { void navigate({ to: '/t/$tenantId/p/$projectId/roles', params: { tenantId, projectId }, search }); };
  const close = () => { if (!busy) leave(); };
  const cancel = () => { if (!busy) setMode(undefined); };
  return <ResponsivePanel open size={mode && ['create', 'copy', 'permissions'].includes(mode) ? 'wide' : 'default'} fallbackFocusSelector={`[id="role-${role}-name"], [data-role-list-search]`} onOpenChange={value => { if (!value) close(); }} title={mode ? projectRoleTitles[mode] : state.error ? '项目角色详情' : state.data?.name ?? '项目角色详情'} description={`${ctx.project.name} · 仅在当前项目生效`}>
    {!allowed ? <FormNotice error>无权查看项目角色。</FormNotice> : state.error ? <ErrorState message={state.error.message} onRetry={() => void state.refetch()} /> : !state.data ? <p role="status">正在读取角色…</p> : mode ? <ProjectRoleForm key={`${role}:${mode}`} mode={mode} initial={state.data} cancel={cancel} onPendingChange={setBusy} finish={() => { toast.success('项目角色变更已保存'); if (mode === 'delete') leave(); else setMode(undefined); }} /> : <div ref={detailRef}><ProjectRoleDetails role={state.data} edit={setMode} /></div>}
  </ResponsivePanel>;
}

function ProjectRoleDetails({ role, edit }: { role: Role; edit: (mode: ProjectRoleMode) => void }) {
  const ctx = useProject(); const { client } = useIdentity(); const [tab, setTab] = useState('permissions');
  const { tenant_id: tenantId, id: projectId } = ctx.project;
  const catalog = useQuery({ queryKey: [...ctx.key, 'permissions'], queryFn: ({ signal }) => client.projectPermissions(tenantId, projectId, signal), enabled: ctx.can(P.ProjectPermissionRead) });
  const members = useInfiniteQuery({ queryKey: [...ctx.key, 'role-members', role.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.projectMembers(tenantId, projectId, { cursor: pageParam }, signal, role.id), getNextPageParam: page => page.next_cursor || undefined, enabled: tab === 'members' && ctx.can(P.ProjectMemberRead) });
  const actions = projectRoleActions(role, ctx, mode => edit(mode));
  return <section aria-label="项目角色详情" className="space-y-5">
    <div className="flex flex-wrap items-center gap-3"><Badge variant="outline">{role.builtin_key ? '平台预置' : '本项目自定义'}</Badge><RoleStatus status={role.status} /><div className="ml-auto">{actions.length > 0 && <RoleActions name={role.name} actions={actions} />}</div></div>
    <p className="break-words text-sm leading-6 text-muted-foreground">{role.description || '暂无说明'}</p>
    <div className="grid grid-cols-3 gap-3 rounded-lg border bg-muted/20 p-4">{[['角色权限', role.permission_codes.length], ['绑定成员', role.account_count], ['当前版本', `v${role.version}`]].map(([label, value]) => <div key={label}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-xl font-semibold tabular-nums">{value}</p></div>)}</div>
    {role.builtin_key && <FormNotice>{role.builtin_key === 'project_admin' ? '项目管理员为受保护角色，仅通过项目创建、管理员交接或失管恢复任命。' : '预置角色由平台统一维护；可按权限复制为本项目独立角色。'}</FormNotice>}
    <Tabs value={tab} onValueChange={setTab} className="gap-4"><TabsList className="w-full"><TabsTrigger value="permissions">角色权限</TabsTrigger><TabsTrigger value="profile">基本资料</TabsTrigger>{ctx.can(P.ProjectMemberRead) && <TabsTrigger value="members">绑定成员</TabsTrigger>}{ctx.can(P.ProjectAuditRead) && <TabsTrigger value="audit">操作记录</TabsTrigger>}</TabsList>
      <TabsContent value="permissions" className="space-y-4"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-medium">已配置权限（{role.permission_codes.length}）</h3>{!role.builtin_key && ctx.allows('ProjectRolePermissions') && <Button variant="outline" disabled={!ctx.available} onClick={() => edit('permissions')}>配置角色权限</Button>}</div>{catalog.error ? <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} /> : ctx.can(P.ProjectPermissionRead) && !catalog.data ? <p role="status">正在读取权限说明…</p> : <>{!ctx.can(P.ProjectPermissionRead) && <FormNotice>暂无权限说明读取权限，可开启编码显示核对已配置权限。</FormNotice>}<ProjectRolePermissions catalog={catalog.data ?? []} codes={role.permission_codes} /></>}</TabsContent>
      <TabsContent value="profile" className="space-y-4"><dl className="space-y-4 text-sm"><div><dt className="text-muted-foreground">角色名称</dt><dd className="mt-1 break-words font-medium">{role.name}</dd></div><div><dt className="text-muted-foreground">角色说明</dt><dd className="mt-1 whitespace-pre-wrap break-words">{role.description || '暂无说明'}</dd></div><div><dt className="text-muted-foreground">创建时间</dt><dd className="mt-1">{new Date(role.created_at).toLocaleString('zh-CN', { hour12: false })}</dd></div></dl>{role.source_role_id && <p className="text-sm leading-6 text-muted-foreground">复制自来源角色的版本 {role.source_role_version}。副本独立维护，不跟随来源更新，不复制成员绑定。</p>}{!role.builtin_key && ctx.allows('ProjectRoleUpdate') && <Button variant="outline" disabled={!ctx.available} onClick={() => edit('update')}>编辑名称与说明</Button>}</TabsContent>
      {ctx.can(P.ProjectMemberRead) && <TabsContent value="members" className="space-y-4"><div><h3 className="font-medium">当前绑定成员</h3><p className="mt-1 text-xs leading-5 text-muted-foreground">包含暂停成员。绑定关系不代表当前具备操作资格，权限由成员状态与全部角色共同决定。</p></div>{members.error ? <ErrorState message={members.error.message} onRetry={() => void members.refetch()} /> : members.isPending ? <p role="status">正在读取绑定成员…</p> : <><ul className="divide-y rounded-lg border">{members.data?.pages.flatMap(page => page.items).map(member => <li key={member.id} className="flex items-center justify-between gap-3 p-4 text-sm"><Link className="inline-flex min-h-11 items-center break-words font-medium text-primary hover:underline" to="/t/$tenantId/p/$projectId/members/$membershipId" params={{ tenantId, projectId, membershipId: member.id }}>{member.display_name}</Link><span className="shrink-0 text-xs text-muted-foreground">{member.status === 'active' ? '正常' : '暂停'}</span></li>)}</ul>{!members.data?.pages[0]?.items.length && <p className="text-sm text-muted-foreground">该角色尚未绑定成员。</p>}{members.hasNextPage && <Button variant="outline" disabled={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>加载更多绑定成员</Button>}</>}</TabsContent>}
      {ctx.can(P.ProjectAuditRead) && <TabsContent value="audit" className="space-y-4"><h3 className="font-medium">角色变更记录</h3><p className="text-sm leading-6 text-muted-foreground">查看此角色的创建、权限配置、状态变更及相关授权记录。</p><ObjectHistory objectType="project_role" objectId={role.id} label="查看角色操作记录" /></TabsContent>}
    </Tabs>
  </section>;
}
