import { useRef, useState } from 'react';
import { Building2, FolderKanban, Plus, ShieldCheck } from 'lucide-react';
import { Permissions as P, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { RoleTable, type RoleAction } from './role-table';
import { useRoleList } from './use-role-list';
import { CreatePlatformRolePanel, PlatformRolePanel, type PlatformRoleMode } from './platform-role-panel';
import { PresetRoles } from './preset-roles';

export function RolesPage() {
  const { can } = useIdentity();
  const [tab, setTab] = useState('platform');
  const [create, setCreate] = useState(false);
  const createTrigger = useRef<HTMLElement | null>(null);
  return <>
    <PageHeader title="角色管理" description="按工作范围管理角色，配置清晰的职责与权限。" actions={tab === 'platform' && can(P.PlatformRoleCreate) ? <Button onClick={(event) => { createTrigger.current = event.currentTarget; setCreate(true); }}><Plus aria-hidden="true" />创建角色</Button> : undefined} />
    <Tabs value={tab} onValueChange={setTab} className="gap-5">
      <TabsList className="max-w-full"><TabsTrigger value="platform" className="max-sm:text-xs"><ShieldCheck className="hidden size-4 sm:block" aria-hidden="true" />平台角色</TabsTrigger><TabsTrigger value="tenant" className="max-sm:text-xs"><Building2 className="hidden size-4 sm:block" aria-hidden="true" />预置租户角色</TabsTrigger><TabsTrigger value="project" className="max-sm:text-xs"><FolderKanban className="hidden size-4 sm:block" aria-hidden="true" />预置项目角色</TabsTrigger></TabsList>
      <TabsContent value="platform"><PlatformRoleList /></TabsContent>
      <TabsContent value="tenant"><PresetRoles scope="tenant" /></TabsContent>
      <TabsContent value="project"><PresetRoles scope="project" /></TabsContent>
    </Tabs>
    {create && can(P.PlatformRoleCreate) && <CreatePlatformRolePanel close={() => setCreate(false)} returnFocus={createTrigger} />}
  </>;
}

function PlatformRoleList() {
  const { client, account, can, canAll } = useIdentity();
  const [q, setQ] = useState(''); const [status, setStatus] = useState(''); const [pageSize, setPageSize] = useState(10);
  const [selected, setSelected] = useState<{ id: string; mode: PlatformRoleMode }>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const list = useRoleList({ queryKey: ['access', account?.id, 'roles', q, status, pageSize], load: (cursor, signal) => client.roles({ q, status, cursor, limit: pageSize }, signal) });
  const open = (role: Role, mode: PlatformRoleMode, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelected({ id: role.id, mode }); };
  const actions = (role: Role): RoleAction[] => [
    { label: '查看详情', run: (trigger) => open(role, 'details', trigger) },
    ...(!role.builtin_key && can(P.PlatformRoleUpdate) ? [{ label: '编辑名称与说明', run: (trigger: HTMLElement | null) => open(role, 'update', trigger) }] : []),
    ...(!role.builtin_key && canAll(P.PlatformRoleRead, P.PlatformPermissionRead, P.PlatformRoleSetPermissions) ? [{ label: '配置角色权限', run: (trigger: HTMLElement | null) => open(role, 'permissions', trigger) }] : []),
    ...(!role.builtin_key && can(P.PlatformRoleSetStatus) ? [{ label: role.status === 'active' ? '停用角色' : '启用角色', run: (trigger: HTMLElement | null) => open(role, 'status', trigger) }] : []),
    ...(!role.builtin_key && can(P.PlatformRoleDelete) ? [{ label: '删除角色', run: (trigger: HTMLElement | null) => open(role, 'archive', trigger), disabled: role.account_count > 0, destructive: true }] : []),
  ];
  return <>
    <RoleTable label="角色列表" items={list.items} q={q} setQ={setQ} status={status} setStatus={setStatus} sourceName={(role) => role.builtin_key ? '内置' : '自定义'} boundCount={(role) => role.account_count} renderName={(role) => <Button id={`role-${role.id}-name`} variant="link" className="h-auto max-w-full justify-start truncate p-0 text-left font-medium" onClick={(event) => open(role, 'details', event.currentTarget)}>{role.name}</Button>} actions={actions} loading={list.isPending} error={list.error} retry={() => void list.refetch()} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
    {selected && <PlatformRolePanel key={`${selected.id}:${selected.mode}`} id={selected.id} initialMode={selected.mode} close={() => setSelected(undefined)} returnFocus={returnFocus} />}
  </>;
}
