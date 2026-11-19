import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Eye, PauseCircle, PlayCircle, UserRoundX } from 'lucide-react';
import { Permissions as P, type TenantMember } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { useTenant } from './context';
import { MembershipConfirm } from './membership-shared';
import { useDiscardInvalidCommand, type DirectoryDialog } from './directory-action';
import { MembershipMenu } from './membership-ui';
import { memberStatus } from './membership-utils';

function memberCommands(item: TenantMember, permissions: readonly string[], accountId?: string) {
  const notice = item.account.id === accountId ? '不能通过成员管理操作自己' : item.is_current_administrator ? '当前管理员，请联系平台先交接' : item.roles.filter((r) => !r.revoked_at).some((r) => r.permission_codes.some((p) => !permissions.includes(p))) ? '目标配置超出本人权限范围，请联系当前管理员' : undefined;
  const actions = [
    { key: 'suspend' as const, title: '暂停成员', icon: PauseCircle, permission: P.TenantMemberSuspend, show: item.membership_status === 'active', effect: '将失去本租户访问，可恢复；其他租户与登录不受影响。' },
    { key: 'resume' as const, title: '恢复成员', icon: PlayCircle, permission: P.TenantMemberResume, show: item.membership_status === 'suspended', effect: '将恢复同一成员关系及保留的本租户角色；要求账号正常。' },
    { key: 'remove' as const, title: '移除成员', icon: UserRoundX, permission: P.TenantMemberRemove, show: item.membership_status !== 'ended', effect: '本次关系将结束并撤销角色；再次加入需要新邀请。' },
  ].filter((action) => !notice && action.show && permissions.includes(action.permission));
  return { notice, actions };
}

export function MemberMenu({ item, details, select }: {
  item: TenantMember;
  details: (trigger: HTMLElement | null) => void;
  select: (item: TenantMember, command: string, trigger: HTMLButtonElement | null) => void;
}) {
  const ctx = useTenant();
  const { account } = useIdentity();
  const { notice, actions } = memberCommands(item, ctx.permissions, account?.id);
  return <MembershipMenu triggerId={'tenant-member-actions-' + item.id} name={item.account.display_name} label="成员操作" notice={notice}
    actions={[{ label: '查看详情', icon: Eye, onSelect: details }, ...actions.map(action => ({
      label: action.title, icon: action.icon, destructive: action.key === 'remove', disabled: !ctx.available,
      onSelect: (trigger: HTMLButtonElement | null) => select(item, action.key, trigger),
    }))]} />;
}

export function MemberActions({ item, dialog }: { item: TenantMember; dialog?: DirectoryDialog }) {
  const ctx = useTenant();
  const { client, account } = useIdentity();
  const cache = useQueryClient();
  const [target, setTarget] = useState(item);
  const { notice, actions } = memberCommands(item, ctx.permissions, account?.id);
  useDiscardInvalidCommand(dialog, actions.some(action => action.key === dialog?.command));
  return <>
    {!dialog && notice && <p className="text-sm text-muted-foreground">{notice}</p>}
    <div className={dialog ? undefined : 'flex flex-wrap gap-2'}>{actions.map(action => (
      <MembershipConfirm key={action.key} title={action.title}
        description={`${ctx.tenant.name} · ${item.account.display_name}（${item.account.username}）。${action.effect}`}
        prepare={() => setTarget(item)} open={dialog ? dialog.command === action.key : undefined}
        onOpenChange={open => { if (!open) dialog?.close(); }} paused={dialog?.paused}
        returnFocus={dialog ? { current: dialog.trigger } : undefined}
        refresh={async () => { setTarget(await client.tenantMember(ctx.tenant.id, item.id)); }}
        submit={async reason => {
          await client.changeTenantMember(ctx.tenant.id, item.id, action.key, { expected_version: target.version, reason });
          void cache.invalidateQueries({ queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id] });
        }}>
        <p>核对状态：{memberStatus[target.membership_status]} · {target.account_available ? '账号可用' : '账号不可用'}</p>
      </MembershipConfirm>
    ))}</div>
  </>;
}
