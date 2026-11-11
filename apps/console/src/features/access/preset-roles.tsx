import { useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Permissions as P, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { RoleTable, RoleStatus } from './role-table';
import { useRoleList } from './use-role-list';
import { RolePermissionGroups, PermissionCodeToggle } from './permission-help';

type PresetRole = Pick<Role, 'id' | 'name' | 'description' | 'status' | 'permission_codes' | 'version' | 'assignment_policy'>;

export function PresetRoles({ scope }: { scope: 'tenant' | 'project' }) {
  const { client, account } = useIdentity();
  const [q, setQ] = useState(''); const [status, setStatus] = useState(''); const [pageSize, setPageSize] = useState(10);
  const [selected, setSelected] = useState<PresetRole>(); const returnFocus = useRef<HTMLElement | null>(null);
  const tenant = scope === 'tenant'; const title = tenant ? '预置租户角色' : '预置项目角色';
  const list = useRoleList<PresetRole>({ queryKey: ['access', account?.id, `${scope}-presets`, q, status, pageSize], load: (cursor, signal) => tenant ? client.tenantRolePresets({ q, status, cursor, limit: pageSize }, signal) : client.projectRolePresets({ q, status, cursor, limit: pageSize }, signal) });
  const open = (role: PresetRole, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelected(role); };
  return <div className="space-y-4"><p className="text-sm text-muted-foreground">{tenant ? '这些角色供租户成员使用。可在租户工作台分配或复制普通预置角色。' : '这些角色供具体项目成员使用。请在项目工作台管理成员的角色。'}</p>
    <RoleTable label={title} items={list.items} q={q} setQ={setQ} status={status} setStatus={setStatus} sourceName={() => '平台预置'} renderName={(role) => <Button id={`role-${role.id}-name`} variant="link" className="h-auto max-w-full justify-start truncate p-0 font-medium" onClick={(event) => open(role, event.currentTarget)}>{role.name}</Button>} actions={(role) => [{ label: '查看详情', run: (trigger) => open(role, trigger) }]} loading={list.isPending} error={list.error} retry={() => void list.refetch()} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
    {selected && <ResponsivePanel open onOpenChange={(open) => { if (!open) setSelected(undefined); }} title={selected.name} description={`${title} · 只读`} returnFocus={returnFocus}><PresetRoleDetails role={selected} scope={scope} /></ResponsivePanel>}
  </div>;
}

function PresetRoleDetails({ role, scope }: { role: PresetRole; scope: 'tenant' | 'project' }) {
  const { client, can } = useIdentity(); const [showCodes, setShowCodes] = useState(false);
  const catalog = useQuery({ queryKey: ['access', 'permissions'], queryFn: ({ signal }) => client.permissions(signal), enabled: can(P.PlatformPermissionRead) });
  return <div className="space-y-5"><div className="flex flex-wrap items-center gap-3"><Badge variant="outline">平台预置</Badge><RoleStatus status={role.status} /><span className="text-sm text-muted-foreground">版本 {role.version}</span></div><p className="break-words text-sm">{role.description}</p><p className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">{role.assignment_policy === 'manual' ? `由平台维护定义，供${scope === 'tenant' ? '租户' : '项目'}成员使用。复制后形成独立角色，不随源角色更新。` : '管理员角色通过专门的创建、交接或恢复流程任命，不能常规复制或分配。'}</p><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-medium">角色权限（{role.permission_codes.length}）</h3><PermissionCodeToggle checked={showCodes} onChange={setShowCodes} /></div>{catalog.error && <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} />}<RolePermissionGroups codes={role.permission_codes} catalog={catalog.data} scope={scope} showCodes={showCodes} /></div>;
}
