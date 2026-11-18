import { useArrayPagination } from '@infrapro/ui/hooks/use-array-pagination';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, Permissions as P, type Permission, type PermissionCode, type ProjectAction, type ProjectChange, type ProjectPreview, type Role } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Textarea } from '@infrapro/ui/components/textarea';
import { ErrorState, FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';
import { useLivePage } from '../tenancy/membership-shared';
import { MutationForm } from './actions';
import { useProject } from './context';
import { projectContextKey, refreshProjects } from './scope';
import { ProjectRolePermissions } from './role-permissions';

export type ProjectRoleMode = 'create' | 'copy' | 'update' | 'permissions' | 'status' | 'delete';
export function ProjectRoleForm({ mode, initial, finish, cancel, onPendingChange }: { mode: ProjectRoleMode; initial?: Role; finish: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const ctx = useProject(); const { client, account } = useIdentity(); const cache = useQueryClient(); const live = useLivePage();
  const { tenant_id: tenant, id: project } = ctx.project;
  const [role, setRole] = useState(initial); const [name, setName] = useState(mode === 'copy' ? `${initial?.name ?? ''} 副本` : initial?.name ?? '');
  const [description, setDescription] = useState(initial?.description ?? ''); const [selected, setSelected] = useState<PermissionCode[]>(initial?.permission_codes ?? []);
  const [impact, setImpact] = useState<ProjectPreview>(); const selectionEpoch = useRef(0);
  // An unconfirmed write must retry the same body, including versions and reason.
  // A background context refresh must not turn that retry into a second command.
  const replay = useRef<{ id: string; run: () => Promise<unknown> } | undefined>(undefined);
  const [targetStatus] = useState(initial?.status === 'active' ? 'disabled' as const : 'active' as const);
  const withPermissions = ['create', 'copy', 'permissions'].includes(mode); const withFields = ['create', 'copy', 'update'].includes(mode);
  const needsPreview = ['permissions', 'status', 'delete'].includes(mode);
  const action: ProjectAction = mode === 'status' ? 'role.status' : mode === 'delete' ? 'role.delete' : 'role.permissions';
  const catalog = useQuery({ queryKey: [...ctx.key, 'permissions'], queryFn: ({ signal }) => client.projectPermissions(tenant, project, signal), enabled: ctx.can(P.ProjectPermissionRead) });
  const input = (): Partial<ProjectChange> => mode === 'permissions' ? { permission_codes: selected } : mode === 'status' ? { status: targetStatus } : {};
  const preview = useOperation({ mutationFn: async () => { const epoch = ++selectionEpoch.current; const value = await client.previewProjectChange(tenant, project, action, input(), role?.id); if (live.current && epoch === selectionEpoch.current) setImpact(value); } });
  const refresh = async () => {
    selectionEpoch.current++; replay.current = undefined; setImpact(undefined); preview.reset();
    const [context, current] = await Promise.all([client.projectContext(tenant, project), role ? client.projectRole(tenant, project, role.id) : Promise.resolve(undefined)]);
    if (!live.current) return;
    cache.setQueryData(projectContextKey(account?.id, tenant, project), context);
    if (current) setRole(current);
    if (needsPreview && current) { const value = await client.previewProjectChange(tenant, project, action, input(), current.id); if (live.current) setImpact(value); }
  };
  const label = mode === 'create' ? '创建项目角色' : mode === 'copy' ? '复制为本项目角色' : mode === 'update' ? '保存角色资料' : mode === 'permissions' ? '确认配置角色权限' : mode === 'delete' ? '确认删除角色' : targetStatus === 'active' ? '确认启用角色' : '确认停用角色';
  const operation = mode === 'create' ? 'ProjectRoleCreate' : mode === 'copy' ? 'ProjectRoleCopy' : mode === 'update' ? 'ProjectRoleUpdate' : mode === 'permissions' ? 'ProjectRolePermissions' : mode === 'delete' ? 'ProjectRoleDelete' : 'ProjectRoleStatus';
  const blocked = !ctx.available || !ctx.allows(operation) || mode !== 'create' && !role || mode !== 'copy' && !!role?.builtin_key || role?.builtin_key === 'project_admin' || withPermissions && (!catalog.data || !!catalog.error) || needsPreview && (!impact?.can_submit || preview.isPending || !!preview.error);
  return <MutationForm label={label} disabled={!!blocked} onPendingChange={onPendingChange} stickyActions freezeOnUnknown cancel={cancel} refresh={refresh} submit={async (reason, requestId) => {
    if (!replay.current || replay.current.id !== requestId) {
      if (withFields && (!name.trim() || Array.from(name.trim()).length > 50)) throw new Error('角色名称须为 1–50 个字符。');
      const common = { name: name.trim(), description, reason, client_request_id: requestId, expected_project_version: ctx.project.version };
      let run: () => Promise<unknown>;
      if (mode === 'create' || mode === 'copy') { const source = mode === 'copy' ? role?.id : undefined; const body = { ...common, permission_codes: [...selected], ...(source ? { source_version: role!.version } : {}) }; run = () => client.createProjectRole(tenant, project, body, source); }
      else if (mode === 'update' && role) { const id = role.id; const body = { ...common, expected_version: role.version }; run = () => client.updateProjectRole(tenant, project, id, body); }
      else { if (!impact || !role) throw new Error('请先预览变更影响。'); const id = role.id; const body = { ...input(), reason, client_request_id: requestId, expected_version: impact.expected_version, expected_project_version: impact.expected_project_version, preview_hash: impact.preview_hash }; run = () => client.changeProject(tenant, project, action, body, id); }
      replay.current = { id: requestId, run };
    }
    try { await replay.current.run(); } catch (error) { if (!(error instanceof ApiError) || error.status && error.status >= 400 && error.status < 500) replay.current = undefined; throw error; }
    if (!live.current) return;
    refreshProjects(cache, account?.id, tenant, project); finish();
  }}>
    <p className="text-sm text-muted-foreground">{role ? `${role.name} · 当前版本 v${role.version}` : '创建后不自动绑定任何成员'}</p>
    {withFields && <><Label htmlFor="project-role-name">角色名称</Label><Input id="project-role-name" value={name} maxLength={50} onChange={event => setName(event.target.value)} required /><Label htmlFor="project-role-description">角色说明</Label><Textarea id="project-role-description" value={description} maxLength={500} onChange={event => setDescription(event.target.value)} rows={3} />{mode === 'update' && <p className="text-xs leading-5 text-muted-foreground">当前名称：{role?.name}；当前说明：{role?.description || '暂无说明'}。</p>}</>}
    {mode === 'copy' && <FormNotice>复制自 {role?.name} v{role?.version}。副本独立维护，不跟随来源更新，不复制成员绑定。退役或无效权限须先取消。</FormNotice>}
    {withPermissions && (catalog.error ? <ErrorState message={catalog.error.message} onRetry={() => void catalog.refetch()} /> : !catalog.data ? <p role="status">正在读取权限说明…</p> : <ProjectRolePermissions catalog={catalog.data} codes={selected} initialCodes={role?.permission_codes ?? []} archived={ctx.project.status === 'archived'} grantable={ctx.authorization.permissions} onChange={codes => { selectionEpoch.current++; setSelected(codes); setImpact(undefined); preview.reset(); }} />)}
    {mode === 'status' && <FormNotice>{targetStatus === 'disabled' ? '停用后保留成员绑定，该角色不再贡献操作权限。' : '启用后，正常账号及有效项目成员重新获得此角色的权限。'}</FormNotice>}
    {mode === 'delete' && <FormNotice>仅无当前绑定的角色可删除；暂停成员的绑定也需先解除。授权与操作历史保留。</FormNotice>}
    {needsPreview && <><Button type="button" variant="outline" disabled={!ctx.available || preview.isPending || withPermissions && (!catalog.data || !!catalog.error)} onClick={() => preview.mutate()}>{preview.isPending ? '正在计算影响…' : '预览变更影响'}</Button><FormNotice error>{preview.error?.message}</FormNotice>{impact && <ProjectRoleImpact value={impact} catalog={catalog.data ?? []} permissionChanges={mode === 'permissions'} />}</>}
  </MutationForm>;
}

function snapshotPermissions(value: unknown): string[] {
  if (!value || typeof value !== 'object' || !('permission_codes' in value) || !Array.isArray(value.permission_codes)) return [];
  return value.permission_codes.filter((code): code is string => typeof code === 'string');
}
function ProjectRoleImpact({ value, catalog, permissionChanges }: { value: ProjectPreview; catalog: Permission[]; permissionChanges: boolean }) {
  const memberPage = useArrayPagination(value.members, 20, value.preview_hash); const names = (codes: string[]) => codes.map(code => catalog.find(item => item.code === code)?.name ?? code).join('、') || '无';
  const before = snapshotPermissions(value.before); const after = snapshotPermissions(value.after);
  return <section aria-label="项目变更影响" className="space-y-4 rounded-lg border bg-muted/20 p-4 text-sm">{permissionChanges && <div className="space-y-2 border-b pb-4"><h3 className="font-medium">本次权限变更</h3><p className="break-words text-xs leading-5">新增权限：{names(after.filter(code => !before.includes(code)))}</p><p className="break-words text-xs leading-5">移除权限：{names(before.filter(code => !after.includes(code)))}</p></div>}<div className="grid grid-cols-2 gap-4"><div><p className="text-xs text-muted-foreground">涉及成员</p><p className="mt-1 text-xl font-semibold">{value.member_count} 人</p></div><div><p className="text-xs text-muted-foreground">实际权限减少</p><p className="mt-1 text-xl font-semibold">{value.losing_members} 人</p></div></div>{value.notes.map(note => <p key={note}>{note}</p>)}{!value.can_submit && <FormNotice error>{value.blocked_reason}</FormNotice>}{memberPage.items.map(member => <div className="space-y-2 rounded-lg border bg-background p-3" key={member.membership_id}><h4 className="font-medium">{member.display_name} · {member.status === 'active' ? '正常成员' : '暂停或结束'}</h4><p className="break-words text-xs leading-5">实际新增：{names(member.added)}</p><p className="break-words text-xs leading-5">实际减少：{names(member.removed)}</p>{(member.restorable_added.length > 0 || member.restorable_removed.length > 0) && <p className="break-words text-xs leading-5 text-muted-foreground">恢复资格后新增：{names(member.restorable_added)}；减少：{names(member.restorable_removed)}</p>}</div>)}<ListPagination label="影响成员" {...memberPage.pagination} /><p className="text-xs text-muted-foreground">成员最终权限按所有角色合并计算，项目状态和资源范围另行验证。</p></section>;
}
