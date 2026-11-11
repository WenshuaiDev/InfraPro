import { useLivePage } from '../tenancy/membership-shared';
import { useRef, useState, useId, type ReactNode } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type ProjectAction, type ProjectAuthorization, type ProjectChange, type ProjectPerson, type ProjectPreview, type Role, type PermissionCode } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@infrapro/ui/components/dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../account/use-operation';
import { requestUUID } from '../tenancy/membership-utils';
import { CandidatePicker, PermissionPicker } from './shared';
import { refreshProjects } from './scope';
import { useOptionalProject } from './context';
import { useOptionalTenant } from '../tenancy/context';

type ActionProps = { tenant: string; project: string; action: ProjectAction; label: string; target?: string; role?: Role; authorization?: ProjectAuthorization; done?: () => void; disabled?: boolean };
export function ProjectActionButton(props: ActionProps) {
  const [open, setOpen] = useState(false);
  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button variant="outline" disabled={props.disabled}>{props.label}</Button></DialogTrigger><DialogContent className="max-h-[88dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{props.label}</DialogTitle><DialogDescription>先预览当前真实影响，再填写原因并确认；取消不改变项目。</DialogDescription></DialogHeader>{open && <ActionForm {...props} close={() => setOpen(false)} />}</DialogContent></Dialog>;
}
function ActionForm({ tenant, project, action, label, target, role, authorization, done, close }: ActionProps & { close: () => void }) {
  const { client, account } = useIdentity(); const cache = useQueryClient(); const current = useOptionalProject(); const [candidate, setCandidate] = useState<ProjectPerson>(); const [selected, setSelected] = useState<PermissionCode[]>(role?.permission_codes ?? []); const [roleIds, setRoleIds] = useState<string[]>(authorization?.roles.map((v) => v.id) ?? []); const [impact, setImpact] = useState<ProjectPreview>(); const generation = useRef(0); const alive = useLivePage();
  const pick = action === 'owner' ? 'owner' : action === 'administrator-transfer' ? 'administrator' : action === 'administrator-recovery' ? 'recovery' : undefined;
  const tenantContext = useOptionalTenant();
  const scopeKey = current?.key ?? ['project-governance', account?.id, tenant, tenantContext?.membership_id, project];
  const catalog = useQuery({ queryKey: [...scopeKey, 'action-catalog'], queryFn: ({ signal }) => client.projectPermissions(tenant, project, signal), enabled: action === 'role.permissions' });
  const roles = useInfiniteQuery({ queryKey: [...scopeKey, 'action-roles'], initialPageParam: '', queryFn: ({ pageParam, signal }) => client.projectRoles(tenant, project, { cursor: pageParam, limit: 100 }, signal), getNextPageParam: (p) => p.next_cursor || undefined, enabled: action === 'member.roles' });
  const changed = () => { generation.current++; setImpact(undefined); };
  const input = (): Partial<ProjectChange> => pick ? { tenant_membership_id: candidate?.tenant_membership_id ?? '' } : action === 'role.permissions' ? { permission_codes: selected } : action === 'member.roles' ? { role_ids: roleIds } : action === 'role.status' ? { status: role?.status === 'active' ? 'disabled' : 'active' } : {};
  const preview = useOperation({ mutationFn: async () => { const stamp = generation.current; const result = await client.previewProjectChange(tenant, project, action, input(), target); if (alive.current && generation.current === stamp) setImpact(result); } });
  const refresh = async () => { changed(); await preview.mutateAsync(); };
  return <MutationForm label={`确认${label}`} disabled={current?.available === false || !impact?.can_submit || preview.isPending || !!preview.error} refresh={refresh} cancel={close} submit={async (reason, requestId) => { if (!impact) throw new Error('请先预览变更影响。'); await client.changeProject(tenant, project, action, { ...input(), client_request_id: requestId, reason, expected_version: impact.expected_version, expected_project_version: impact.expected_project_version, preview_hash: impact.preview_hash, ...(action === 'member.roles' ? { role_versions: impact.role_versions } : {}) }, target); if (!alive.current) return; refreshProjects(cache, account?.id, tenant, project); done?.(); close(); }}>
    {pick && <CandidatePicker tenant={tenant} project={project} kind={pick} label={action === 'owner' ? '选择业务负责人' : '选择接任管理员'} value={candidate} onChange={(v) => { setCandidate(v); changed(); }} />}
    {action === 'role.permissions' && catalog.data && <PermissionPicker catalog={catalog.data} selected={selected} change={(v) => { setSelected(v); changed(); }} />}
    {action === 'member.roles' && <section className="space-y-2"><p>可选择多个角色；全部取消表示清空，不会自动补回普通成员。</p>{roles.data?.pages.flatMap((p) => p.items).filter((r) => r.builtin_key !== 'project_admin').map((r) => <label key={r.id} className="flex min-h-11 items-center gap-3 rounded border p-3 text-sm"><input type="checkbox" className="size-4" checked={roleIds.includes(r.id)} disabled={r.status !== 'active' && !authorization?.roles.some((v) => v.id === r.id)} onChange={(e) => { setRoleIds(e.target.checked ? [...roleIds, r.id] : roleIds.filter((id) => id !== r.id)); changed(); }} /><span>{r.name} · {r.status === 'active' ? '已启用' : '已停用，只能保留既有绑定'}</span></label>)}{roles.hasNextPage && <Button type="button" variant="outline" onClick={() => void roles.fetchNextPage()}>加载更多角色</Button>}</section>}
    <FormNotice error>{catalog.error?.message ?? roles.error?.message ?? preview.error?.message}</FormNotice><Button type="button" variant="outline" disabled={preview.isPending || current?.available === false} onClick={() => preview.mutate()}>{preview.isPending ? '正在计算影响…' : '预览变更影响'}</Button>
    {impact && <ImpactView value={impact} />}
  </MutationForm>;
}
export function ImpactView({ value }: { value: ProjectPreview }) {
  const [shown, setShown] = useState(20);
  return <section aria-label="项目变更影响" className="space-y-3 rounded-lg bg-muted/40 p-4 text-sm"><p>对象版本 {value.expected_version} · 项目版本 {value.expected_project_version}</p><p>涉及 {value.member_count} 名成员，其中 {value.losing_members} 人的实际权限减少。</p>{value.notes.map((v) => <p key={v}>{v}</p>)}{!value.can_submit && <FormNotice error>{value.blocked_reason}</FormNotice>}{value.members.slice(0, shown).map((m) => <div key={m.membership_id} className="space-y-1 rounded border p-3"><p>{m.display_name} · {m.status === 'active' ? '有效成员' : '当前暂停或结束'}</p><p className="break-words">实际新增：{m.added.join('、') || '无'}；实际减少：{m.removed.join('、') || '无'}</p><p className="break-words">恢复资格后的新增：{m.restorable_added.join('、') || '无'}；减少：{m.restorable_removed.join('、') || '无'}</p></div>)}{shown < value.members.length && <Button type="button" variant="outline" onClick={() => setShown((n) => n + 20)}>查看更多影响成员</Button>}</section>;
}
export function MutationForm({ children, label, submit, refresh, cancel, disabled = false }: { children: ReactNode; label: string; submit: (reason: string, requestId: string) => Promise<void>; refresh?: () => Promise<unknown>; cancel?: () => void; disabled?: boolean }) {
  const { client } = useIdentity(); const [reason, setReason] = useState(''); const [password, setPassword] = useState(''); const [requestId, setRequestId] = useState(requestUUID); const [notice, setNotice] = useState(''); const reasonId = useId(); const passwordId = useId();
  const mutation = useOperation({ mutationFn: async () => { if (!reason.trim() || Array.from(reason.trim()).length > 500) throw new Error('请填写 1–500 字符的变更原因。'); await submit(reason.trim(), requestId); } });
  const reauth = mutation.error instanceof ApiError && mutation.error.code === 'reauthentication_required';
  const stale = mutation.error instanceof ApiError && ['preview_stale', 'version_conflict', 'state_conflict', 'request_payload_conflict'].includes(mutation.error.code ?? '');
  const reload = useOperation({ mutationFn: async () => { await refresh?.(); mutation.reset(); setRequestId(requestUUID()); setNotice('已重新读取当前影响，输入已保留，请核对后再次确认。'); } });
  const confirm = useOperation({ mutationFn: () => client.reauthenticate(password), onSuccess: () => { setPassword(''); mutation.reset(); setNotice('密码已确认，请重新预览并再次提交。'); if (refresh) reload.mutate(); } });
  return <div className="space-y-4"><form className="space-y-4" onSubmit={(e) => { e.preventDefault(); mutation.mutate(); }}><fieldset className="min-w-0 space-y-4" disabled={mutation.isPending}>{children}<Label htmlFor={reasonId}>变更原因</Label><Input id={reasonId} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} required /></fieldset><FormNotice error>{mutation.error?.message ?? reload.error?.message}</FormNotice>{notice && <FormNotice>{notice}</FormNotice>}{mutation.error instanceof ApiError && !mutation.error.status && <FormNotice>结果尚未确认，请保持内容不变，使用本请求重试，或刷新记录核对结果。</FormNotice>}{stale && refresh && <Button type="button" variant="outline" disabled={reload.isPending} onClick={() => reload.mutate()}>重新读取并比较</Button>}<div className="flex flex-wrap gap-3"><Button disabled={disabled || mutation.isPending || reauth || stale || reload.isPending}>{mutation.isPending ? '正在提交…' : label}</Button>{cancel && <Button type="button" variant="outline" disabled={mutation.isPending} onClick={cancel}>取消</Button>}</div></form>{reauth && <form className="space-y-3 rounded-lg border p-4" onSubmit={(e) => { e.preventDefault(); confirm.mutate(); }}><Label htmlFor={passwordId}>当前密码</Label><Input id={passwordId} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /><FormNotice error>{confirm.error?.message}</FormNotice><Button disabled={confirm.isPending}>确认密码</Button></form>}</div>;
}
