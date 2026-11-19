import { useRef, type ReactNode } from 'react';
import { CheckCircle2, CircleDashed, MoreHorizontal, type LucideIcon } from 'lucide-react';
import type { AvatarTarget, TenantInvitation, TenantMember } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { AccountAvatar } from '../access/account-table';
import { InvitationStatus } from '../account/invitation-ui';
import { invitationStatus, memberStatus } from './membership-utils';

export function MembershipIdentity({ name, username, onClick, children, avatar, target }: { avatar?: TenantMember['account']; target?: AvatarTarget; name: string; username: string; onClick: (trigger: HTMLElement) => void; children?: ReactNode }) {
  return <div className="flex min-w-0 items-center gap-3"><div className="hidden sm:block"><AccountAvatar name={name} metadata={avatar} target={target} /></div><div className="min-w-0 flex-1 space-y-1"><Button variant="link" className="h-auto max-w-full justify-start truncate p-0 text-left font-medium" title={`${name}（${username}）`} onClick={(event) => onClick(event.currentTarget)}><span className="truncate">{name}</span></Button><p className="truncate text-xs text-muted-foreground" title={username}>{username}</p>{children}</div></div>;
}

export function MemberStatus({ item }: { item: TenantMember }) {
  const Icon = item.membership_status === 'active' ? CheckCircle2 : CircleDashed;
  return <div className="space-y-1"><span className="inline-flex items-center gap-2 whitespace-nowrap text-sm"><Icon aria-hidden="true" className="size-4 text-muted-foreground" />{memberStatus[item.membership_status]}</span><p className="text-xs text-muted-foreground">{item.account_available ? '账号可用' : '账号不可用'}</p></div>;
}

export function MemberRoles({ item, onMore }: { item: TenantMember; onMore: (trigger: HTMLElement) => void }) {
  const roles = item.roles.filter((role) => item.membership_status === 'ended' || !role.revoked_at);
  return <div className="space-y-1.5"><div className="flex min-w-0 flex-wrap gap-1.5">{roles.slice(0, 2).map((role) => <Badge key={role.binding_id} variant="secondary" className="max-w-full font-normal" title={role.name}><span className="truncate">{role.name}{role.status === 'disabled' ? '（停用）' : ''}</span></Badge>)}{roles.length > 2 && <Button variant="ghost" className="h-6 px-1.5 text-xs" aria-label={`查看全部 ${roles.length} 个绑定角色`} onClick={(event) => onMore(event.currentTarget)}>+{roles.length - 2}</Button>}{!roles.length && <span className="text-sm text-muted-foreground">未绑定</span>}</div>{!item.role_effective && roles.length > 0 && <p className="text-xs text-muted-foreground">当前不生效</p>}</div>;
}

export function TenantInvitationStatus({ status }: { status: TenantInvitation['status'] }) {
  return <InvitationStatus status={status === 'accepted' ? 'used' : status === 'pending' || status === 'expired' ? status : 'revoked'} label={invitationStatus[status]} />;
}

export function MembershipMenu({ name, label, actions, notice, triggerId }: { triggerId?: string; name: string; label: string; actions: { label: string; icon: LucideIcon; onSelect: (trigger: HTMLButtonElement | null) => void; destructive?: boolean; disabled?: boolean }[]; notice?: string }) {
  const trigger = useRef<HTMLButtonElement>(null), opening = useRef(false);
  return <DropdownMenu><DropdownMenuTrigger asChild><Button ref={trigger} id={triggerId} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${name}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-56 max-w-[calc(100vw-2rem)]" onCloseAutoFocus={(event) => { if (opening.current) { event.preventDefault(); opening.current = false; } }}><DropdownMenuLabel>{label}</DropdownMenuLabel><DropdownMenuSeparator />{actions.map((action) => <DropdownMenuItem key={action.label} disabled={action.disabled} variant={action.destructive ? 'destructive' : 'default'} onSelect={() => { opening.current = true; action.onSelect(trigger.current); }}><action.icon aria-hidden="true" />{action.label}</DropdownMenuItem>)}{notice && <><DropdownMenuSeparator /><p className="px-2 py-2 text-xs leading-5 text-muted-foreground">{notice}</p></>}</DropdownMenuContent></DropdownMenu>;
}
