import { useRef } from 'react';
import { CheckCircle2, CircleDashed, Eye, MoreHorizontal, ShieldCheck, UserCheck, UserMinus, UserRoundX } from 'lucide-react';
import { Permissions as P, type ProjectMember } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { useProject } from './context';
import { availableMemberOperations, memberOperations, type MemberMode } from './member-operations';

export function ProjectMemberStatus({ member }: { member: ProjectMember }) {
  const Icon = member.status === 'active' ? CheckCircle2 : CircleDashed;
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-sm"><Icon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />{member.status === 'active' ? '正常' : member.status === 'suspended' ? '已暂停' : '已结束'}</span>;
}

export function ProjectMemberIdentity({ member }: { member: ProjectMember }) {
  return member.is_current_administrator ? <Badge variant="secondary" className="font-normal"><ShieldCheck aria-hidden="true" />当前管理员</Badge> : <span className="text-sm text-muted-foreground">项目成员</span>;
}

export function ProjectMemberEligibility({ member }: { member: ProjectMember }) {
  return <div className="space-y-1 text-xs text-muted-foreground"><p>{member.account_status === 'active' ? '账号正常' : '账号不可用'}</p><p>{member.tenant_membership_status === 'active' ? '租户成员有效' : member.tenant_membership_status === 'suspended' ? '租户成员已暂停' : '租户关系已结束'}</p></div>;
}

export function ProjectMemberMenu({ member, open }: { member: ProjectMember; open: (mode: MemberMode, trigger: HTMLElement | null) => void }) {
  const ctx = useProject();
  const trigger = useRef<HTMLButtonElement>(null), opening = useRef(false);
  const actions = availableMemberOperations(member, ctx.authorization.account_id, ctx.allows, ctx.can(P.ProjectAuthorizationRead));
  const select = (mode: MemberMode) => { opening.current = true; open(mode, trigger.current); };
  const icons = { 'member.roles': ShieldCheck, 'member.suspend': UserRoundX, 'member.resume': UserCheck, 'member.remove': UserMinus };
  return <DropdownMenu><DropdownMenuTrigger asChild><Button id={`project-member-${member.id}-actions`} ref={trigger} variant="ghost" size="icon" className="size-9 max-sm:size-11" aria-label={`${member.display_name}的操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" onCloseAutoFocus={(event) => { if (opening.current) { event.preventDefault(); opening.current = false; } }}>
      <DropdownMenuLabel>成员操作</DropdownMenuLabel><DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => select('details')}><Eye aria-hidden="true" />查看详情</DropdownMenuItem>
      {actions.map((action) => { const Icon = icons[action]; return <DropdownMenuItem key={action} className={action === 'member.remove' || action === 'member.suspend' ? 'text-destructive focus:text-destructive' : undefined} onSelect={() => select(action)}><Icon aria-hidden="true" />{memberOperations[action].label}</DropdownMenuItem>; })}
    </DropdownMenuContent>
  </DropdownMenu>;
}
