import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type PermissionCode, type TenantRoleDefinition, type TenantAuthorizationImpact, type TenantRolePreviewInput } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Textarea } from '@infrapro/ui/components/textarea';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';
import { useTenant } from './context';
import { requestUUID } from './membership-utils';
import { ManagementForm, useLivePage } from './membership-shared';
import { TenantImpactView, TenantPermissionPicker } from './role-components';

export type TenantRoleMode = 'create' | 'copy' | 'update' | 'permissions' | 'status' | 'archive';
export function TenantRoleForm({ mode, initial, finish, cancel, onPendingChange }: { mode: TenantRoleMode; initial?: TenantRoleDefinition; finish: () => void; cancel: () => void; onPendingChange: (pending: boolean) => void }) {
  const ctx = useTenant(); const { client, account, reload } = useIdentity(); const cache = useQueryClient(); const live = useLivePage();
  const [role, setRole] = useState(initial); const [name, setName] = useState(mode === 'copy' ? `${initial?.name ?? ''} 副本` : initial?.name ?? ''); const [description, setDescription] = useState(initial?.description ?? ''); const [selected, setSelected] = useState<PermissionCode[]>(initial?.permission_codes ?? []); const [requestId] = useState(requestUUID);
  const selectionEpoch = useRef(0);
  const [impact, setImpact] = useState<TenantAuthorizationImpact>();
  const withPermissions = ['create', 'copy', 'permissions'].includes(mode); const withFields = ['create', 'copy', 'update'].includes(mode); const needsPreview = ['permissions', 'status', 'archive'].includes(mode);
  const catalog = useQuery({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'permissions'], queryFn: ({ signal }) => client.tenantPermissions(ctx.tenant.id, signal), enabled: withPermissions && ctx.available });
  const action = mode === 'status' || mode === 'archive' ? mode : 'permissions';
  const input = (): TenantRolePreviewInput => action === 'permissions' ? { expected_version: role!.version, permission_codes: selected } : action === 'status' ? { expected_version: role!.version, status: role!.status === 'active' ? 'disabled' : 'active' } : { expected_version: role!.version };
  const preview = useOperation<void, string>({ mutationFn: async (cursor) => { const epoch = selectionEpoch.current; const value = await client.previewTenantRole(ctx.tenant.id, role!.id, action, input(), { cursor }); if (live.current && epoch === selectionEpoch.current) setImpact((old) => cursor && old?.preview_hash === value.preview_hash ? { ...value, members: { ...value.members, items: [...old.members.items, ...value.members.items] } } : value); } });
  const refresh = async () => { if (role) { const current = await client.tenantRole(ctx.tenant.id, role.id); if (live.current) setRole(current); } if (live.current) setImpact(undefined); };
  return <ManagementForm onPendingChange={onPendingChange} stickyActions label={needsPreview ? '确认授权变更' : mode === 'update' ? '保存角色资料' : '创建角色'} disabled={(withPermissions && !catalog.data) || (needsPreview && (!impact || impact.blocked_reasons.length > 0))} refresh={refresh} cancel={<Button type="button" variant="outline" onClick={cancel}>取消</Button>} submit={async (reason) => {
    if (mode === 'create' || mode === 'copy') await client.createTenantRole(ctx.tenant.id, { name, description, permission_codes: selected, client_request_id: requestId, reason, ...(mode === 'copy' ? { source_version: role!.version } : {}) }, mode === 'copy' ? role!.id : undefined);
    else if (mode === 'update') await client.updateTenantRole(ctx.tenant.id, role!.id, { name, description, expected_version: role!.version, reason });
    else { if (!impact) throw new Error('请先预览影响。'); await client.changeTenantRole(ctx.tenant.id, role!.id, action, { ...input(), reason, preview_hash: impact.preview_hash }); }
    if (live.current) { void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id] }); void reload(); finish(); }
  }}>
    <p className="text-sm">{ctx.tenant.name} · {role ? `当前版本 v${role.version}` : '创建后不自动绑定任何成员'}</p>
    {withFields && <><Label htmlFor="tenant-role-name">角色名称</Label><Input id="tenant-role-name" required maxLength={50} value={name} onChange={(e) => setName(e.target.value)} /><Label htmlFor="tenant-role-description">角色说明</Label><Textarea id="tenant-role-description" rows={3} maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} />{mode === 'update' && <p className="text-sm">当前名称：{role?.name} → {name}；当前说明：{role?.description || '无'} → {description || '无'}。权限不变。</p>}</>}
    {withPermissions && (catalog.data ? <TenantPermissionPicker catalog={catalog.data} selected={selected} onChange={(next) => { selectionEpoch.current++; setSelected(next); setImpact(undefined); }} /> : <p role="status">正在读取权限说明…</p>)}<FormNotice error>{catalog.error?.message ?? preview.error?.message}</FormNotice>
    {mode === 'copy' && <FormNotice>复制自 {role?.name} v{role?.version}。副本独立维护，不跟随源角色发布更新，也不复制成员绑定。</FormNotice>}
    {mode === 'status' && <FormNotice>{role?.status === 'active' ? '停用后保留绑定，失去最后邀请来源的链接永久失效。' : '启用只恢复正常账号与有效成员的能力，不恢复失效邀请。'}</FormNotice>}
    {mode === 'archive' && <FormNotice>仅无任何未撤销绑定的角色可删除；暂停成员和不可用账号上的绑定也需先解除。</FormNotice>}
    {needsPreview && <Button type="button" variant="outline" disabled={preview.isPending} onClick={() => preview.mutate('')}>{preview.isPending ? '正在计算…' : '预览变更影响'}</Button>}
    {impact && <TenantImpactView value={impact} more={() => preview.mutate(impact.members.next_cursor)} />}
  </ManagementForm>;
}
