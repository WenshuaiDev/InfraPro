import { useRef, useState } from 'react';
import { Link, useNavigate, useSearch } from '@tanstack/react-router';
import { Plus } from 'lucide-react';
import { Permissions as P, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { toast } from 'sonner';
import { RoleTable, type RoleAction } from '../access/role-table';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useProject } from './context';
import { ProjectRoleForm, type ProjectRoleMode } from './role-form';
import { projectRoleActions, projectRoleTitles } from './role-controls';
import { projectRoleSearch, type ProjectRoleSearch } from './role-search';

export function ProjectRolesPage() {
  const ctx = useProject(); const { client } = useIdentity(); const navigate = useNavigate();
  const search = useSearch({ from: '/_authenticated/t/$tenantId/p/$projectId/roles' });
  const { q = '', status = '', source = '', limit = 10 } = search;
  const { tenant_id: tenantId, id: projectId } = ctx.project;
  const [operation, setOperation] = useState<{ mode: ProjectRoleMode; role?: Role }>();
  const [busy, setBusy] = useState(false); const returnFocus = useRef<HTMLElement | null>(null);
  const allowed = ctx.can(P.ProjectRoleRead);
  const list = useCursorList({ pageSize: limit, queryKey: [...ctx.key, 'roles', q, status, source, limit], load: (cursor, signal) => client.projectRoles(tenantId, projectId, { q, status, source, limit, cursor }, signal), enabled: allowed });
  const filter = (patch: Partial<ProjectRoleSearch>) => { void navigate({ to: '/t/$tenantId/p/$projectId/roles', params: { tenantId, projectId }, search: previous => projectRoleSearch({ ...previous, ...patch }), replace: true }); };
  const open = (mode: ProjectRoleMode, role: Role | undefined, trigger: HTMLElement | null) => { returnFocus.current = trigger; setOperation({ mode, role }); };
  const close = () => { if (!busy) setOperation(undefined); };
  const actions = (role: Role): RoleAction[] => [
    { label: '查看角色详情', run: () => void navigate({ to: '/t/$tenantId/p/$projectId/roles/$roleId', params: { tenantId, projectId, roleId: role.id }, search }) },
    ...projectRoleActions(role, ctx, (mode, trigger) => open(mode, role, trigger)),
  ];
  if (!allowed) return <FormNotice error>无权查看项目角色。可在项目资料中查看本人的授权信息。</FormNotice>;
  return <div className="space-y-5">
    <PageHeader title="项目角色" description="为项目成员安排职责，查看和配置角色权限。" actions={ctx.allows('ProjectRoleCreate') ? <Button disabled={!ctx.available} onClick={event => open('create', undefined, event.currentTarget)}><Plus aria-hidden="true" />创建项目角色</Button> : undefined} />
    <Tabs value={source || 'all'} onValueChange={value => filter({ source: value === 'custom' || value === 'preset' ? value : undefined })}><TabsList aria-label="角色来源" className="max-w-full"><TabsTrigger value="all">全部角色</TabsTrigger><TabsTrigger value="custom">本项目自定义</TabsTrigger><TabsTrigger value="preset">平台预置</TabsTrigger></TabsList></Tabs>
    <RoleTable pagination={list.pagination} label="项目角色列表" items={list.error ? [] : list.items} q={q} setQ={value => filter({ q: Array.from(value).slice(0, 100).join('') || undefined })} status={status} setStatus={value => filter({ status: value === 'active' || value === 'disabled' ? value : undefined })} sourceName={role => role.builtin_key ? '平台预置' : '本项目自定义'} boundCount={role => role.account_count} renderName={role => <Link id={`role-${role.id}-name`} className="inline-block max-w-full truncate font-medium text-primary hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-ring" to="/t/$tenantId/p/$projectId/roles/$roleId" params={{ tenantId, projectId, roleId: role.id }} search={search}>{role.name}</Link>} actions={actions} loading={list.isPending} error={list.error} retry={() => void list.refetch()} setPageSize={value => filter({ limit: value })} />
    <p className="text-xs leading-5 text-muted-foreground">角色仅在当前项目生效。绑定人数包含暂停成员；设施、设备与监测数据范围分别授权。</p>
    {operation && <ResponsivePanel open size={['create', 'copy', 'permissions'].includes(operation.mode) ? 'wide' : 'default'} onOpenChange={value => { if (!value) close(); }} title={projectRoleTitles[operation.mode]} description={`${ctx.project.name} · ${operation.role?.name ?? '创建后不自动绑定成员'}`} returnFocus={returnFocus} fallbackFocusSelector="[data-role-list-search]"><ProjectRoleForm key={`${operation.mode}:${operation.role?.id ?? 'new'}`} mode={operation.mode} initial={operation.role} finish={() => { setOperation(undefined); toast.success('项目角色变更已保存'); }} cancel={close} onPendingChange={setBusy} /></ResponsivePanel>}
  </div>;
}
