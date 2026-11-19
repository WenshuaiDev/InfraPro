import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Ban, Eye, MailPlus, Plus, RotateCcw } from 'lucide-react';
import { Permissions as P, type TenantAccount, type TenantInvitation, type TenantInvitationCreated } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Dialog, DialogClose, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogTrigger } from '@infrapro/ui/components/dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useTenant } from './context';
import { ManagementForm, MembershipConfirm, useLivePage } from './membership-shared';
import { useDiscardInvalidCommand, type DirectoryDialog } from './directory-action';
import { MembershipMenu } from './membership-ui';
import { invitationStatus, requestUUID } from './membership-utils';
import { useOperation } from '../account/use-operation';

export function CreateMemberInvitation({ done }: { done: (value: TenantInvitationCreated) => void }) {
  const ctx = useTenant(); const { client } = useIdentity(); const [open, setOpen] = useState(false);
  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button disabled={!ctx.available}><Plus aria-hidden="true" />邀请成员</Button></DialogTrigger><DialogContent className="max-h-[85dvh] overflow-y-auto"><DialogHeader><DialogTitle className="flex items-center gap-2.5"><MailPlus className="size-5" aria-hidden="true" />邀请已有账号加入</DialogTitle><DialogDescription className="leading-6">{ctx.tenant.name} · 固定角色为租户普通成员，可查看基本资料；邀请有效 24 小时。</DialogDescription></DialogHeader>{open && <CreateForm resolve={(username) => client.resolveTenantMember(ctx.tenant.id, username)} done={(value) => { setOpen(false); done(value); }} />}</DialogContent></Dialog>;
}
function CreateForm({ resolve, done }: { resolve: (username: string) => Promise<TenantAccount>; done: (value: TenantInvitationCreated) => void }) {
  const ctx = useTenant(); const { client } = useIdentity(); const [username, setUsername] = useState(''); const [target, setTarget] = useState<TenantAccount>(); const [requestId] = useState(requestUUID); const live = useLivePage();
  const lookup = useOperation({ mutationFn: () => resolve(username.trim()), onSuccess: setTarget });
  return <ManagementForm disabled={!target} cancel={<DialogClose asChild><Button type="button" variant="outline">取消</Button></DialogClose>} label="创建成员邀请" submit={async (reason) => { if (!target) throw new Error('请先定位并核对目标账号。'); const result = await client.createTenantInvitation(ctx.tenant.id, { target_account_id: target.id, reason, client_request_id: requestId }); if (live.current) done(result); }}>
    <div className="space-y-2"><Label htmlFor="member-username">完整用户名</Label><Input id="member-username" value={username} disabled={lookup.isPending} autoCapitalize="none" spellCheck={false} onChange={(e) => { setUsername(e.target.value); setTarget(undefined); lookup.reset(); }} /></div>
    <Button type="button" variant="outline" disabled={lookup.isPending || !username.trim() || !ctx.available} onClick={() => lookup.mutate()}>{lookup.isPending ? '正在定位…' : '定位账号'}</Button><FormNotice error>{lookup.error?.message}</FormNotice>
    {target && <div className="space-y-2 rounded-lg bg-muted p-3" role="status"><p className="break-words">受邀账号：{target.display_name}（{target.username}）</p><p className="text-sm">租户普通成员 · 可查看本租户基本资料</p></div>}
  </ManagementForm>;
}

function invitationCommands(item: TenantInvitation, permissions: readonly string[]) {
  return [
    { key: 'regenerate', title: '重新生成', icon: RotateCcw, show: !['accepted', 'replaced'].includes(item.status) && !item.replaced_by_id && permissions.includes(P.TenantInvitationRegenerate) && permissions.includes(P.TenantInvitationCreate), effect: '旧链接永久失效，新链接有效 24 小时；请重新交付本人。' },
    { key: 'revoke', title: '撤销邀请', icon: Ban, show: item.status === 'pending' && permissions.includes(P.TenantInvitationRevoke), effect: '旧链接将不能领取，邀请记录仍会保留。' },
  ].filter((action) => action.show);
}

export function InvitationMenu({ item, details, select }: {
  item: TenantInvitation;
  details: (trigger: HTMLElement | null) => void;
  select: (item: TenantInvitation, command: string, trigger: HTMLButtonElement | null) => void;
}) {
  const ctx = useTenant();
  const actions = invitationCommands(item, ctx.permissions);
  return <MembershipMenu triggerId={'tenant-invitation-actions-' + item.id} name={item.target.display_name} label="邀请操作"
    actions={[{ label: '查看详情', icon: Eye, onSelect: details }, ...actions.map(action => ({
      label: action.title, icon: action.icon, destructive: action.key === 'revoke', disabled: !ctx.available,
      onSelect: (trigger: HTMLButtonElement | null) => select(item, action.key, trigger),
    }))]} />;
}

export function InvitationActions({ item, done, dialog }: { item: TenantInvitation; done: (value: TenantInvitationCreated) => void; dialog?: DirectoryDialog }) {
  const ctx = useTenant();
  const { account, client } = useIdentity();
  const cache = useQueryClient();
  const live = useLivePage();
  const [target, setTarget] = useState(item), [requestId, setRequestId] = useState(requestUUID);
  const actions = invitationCommands(item, ctx.permissions);
  useDiscardInvalidCommand(dialog, actions.some(action => action.key === dialog?.command));
  const refresh = async () => { setTarget(await client.tenantInvitation(ctx.tenant.id, item.id)); setRequestId(requestUUID()); };
  return <div className={dialog ? undefined : 'flex flex-wrap gap-2'}>{actions.map(action => (
    <MembershipConfirm key={action.key} title={action.title}
      description={`${ctx.tenant.name} · ${item.target.display_name}（${item.target.username}）。${action.effect}`}
      prepare={() => { setTarget(item); setRequestId(requestUUID()); }}
      open={dialog ? dialog.command === action.key : undefined}
      onOpenChange={open => { if (!open) dialog?.close(); }} paused={dialog?.paused}
      returnFocus={dialog ? { current: dialog.trigger } : undefined} refresh={refresh} submit={async reason => {
        if (action.key === 'revoke') await client.revokeTenantInvitation(ctx.tenant.id, item.id, { expected_version: target.version, reason });
        else {
          const value = await client.regenerateTenantInvitation(ctx.tenant.id, item.id, { expected_version: target.version, reason, client_request_id: requestId });
          if (live.current) done(value);
        }
        void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id] });
      }}>
      <p>当前状态：{invitationStatus[target.status]}</p>
    </MembershipConfirm>
  ))}</div>;
}
