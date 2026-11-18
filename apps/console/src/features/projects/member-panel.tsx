import { useState, type RefObject } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft } from 'lucide-react';
import { Permissions as P, type ProjectPerson } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { AccountAvatar } from '../access/account-table';
import { ObjectHistory } from '../audit/object-history';
import { useLivePage } from '../tenancy/membership-shared';
import { MutationForm, ProjectActionForm } from './actions';
import { useProject } from './context';
import { ProjectMemberAuthorization } from './member-authorization';
import { availableMemberOperations, memberOperations, projectMemberDate, type MemberMode } from './member-operations';
import { ProjectMemberEligibility, ProjectMemberIdentity, ProjectMemberStatus } from './member-ui';
import { refreshProjects } from './scope';
import { CandidatePicker } from './shared';

type PanelProps = { close: () => void; returnFocus: RefObject<HTMLElement | null> };

export function ProjectMemberPanel({ id, initialMode, close, returnFocus }: PanelProps & { id: string; initialMode: MemberMode }) {
  const ctx = useProject(), { client } = useIdentity();
  const { tenant_id: tenant, id: project } = ctx.project;
  const [mode, setMode] = useState(initialMode);
  const canReadAuthorization = ctx.can(P.ProjectAuthorizationRead);
  const detail = useQuery({ queryKey: [...ctx.key, 'member', id], queryFn: ({ signal }) => client.projectMember(tenant, project, id, signal), enabled: ctx.can(P.ProjectMemberRead) });
  const auth = useQuery({ queryKey: [...ctx.key, 'member-auth', id], queryFn: ({ signal }) => client.projectAuthorization(tenant, project, id, signal), enabled: ctx.can(P.ProjectMemberRead) && canReadAuthorization });
  const member = detail.data;
  const actions = member ? availableMemberOperations(member, ctx.authorization.account_id, ctx.allows, canReadAuthorization) : [];
  const back = () => setMode('details');
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title={mode === 'details' ? '成员详情' : memberOperations[mode].label} description={member ? `${member.username} · 项目成员` : '查看项目成员资料与授权情况。'} returnFocus={returnFocus} fallbackFocusSelector="[data-project-member-search]">
    {detail.error ? <ErrorState message={detail.error.message} onRetry={() => void detail.refetch()} /> : !member ? <LoadingState label="正在读取成员…" /> : <section aria-label="项目成员详情" className="space-y-5">
      <div className="flex flex-wrap items-start gap-3"><AccountAvatar name={member.display_name} metadata={member} target={{ kind: 'project', tenant, project, id: member.id }} /><div className="min-w-0 flex-1 space-y-2"><h2 className="break-words font-semibold">{member.display_name}</h2><p className="break-all text-sm text-muted-foreground">{member.username}</p><div className="flex flex-wrap items-center gap-3"><ProjectMemberStatus member={member} /><ProjectMemberIdentity member={member} /></div></div><ObjectHistory objectType="project_member" objectId={id} /></div>
      {mode !== 'details' ? <>
        <Button variant="ghost" className="px-0" onClick={back}><ArrowLeft aria-hidden="true" />返回成员详情</Button>
        {!actions.includes(mode) ? <FormNotice error>当前操作不可用，请返回成员详情重新核对。</FormNotice> : <>
          <p className="rounded-lg border bg-muted/20 p-4 text-sm leading-6">{memberOperations[mode].description}</p>
          {mode === 'member.roles' && (auth.error || !auth.data) ? auth.error ? <ErrorState message={auth.error.message} onRetry={() => void auth.refetch()} /> : <LoadingState label="正在读取角色绑定…" /> : <ProjectActionForm key={mode} tenant={tenant} project={project} target={id} action={mode} label={memberOperations[mode].label} authorization={auth.data} disabled={!ctx.available || detail.isFetching || (mode === 'member.roles' && auth.isFetching)} close={back} />}
        </>}
      </> : <>
        <dl className="grid gap-4 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2">
          <div><dt className="text-muted-foreground">加入时间</dt><dd className="mt-1 tabular-nums"><time dateTime={member.joined_at}>{projectMemberDate(member.joined_at, true)}</time></dd></div>
          <div><dt className="text-muted-foreground">账号 / 租户状态</dt><dd className="mt-1"><ProjectMemberEligibility member={member} /></dd></div>
          <div className="sm:col-span-2"><dt className="text-muted-foreground">最近状态原因</dt><dd className="mt-1 break-words">{member.status_reason || '加入项目'}</dd></div>
          {member.ended_at && <div className="sm:col-span-2"><dt className="text-muted-foreground">关系结束时间</dt><dd className="mt-1"><time dateTime={member.ended_at}>{projectMemberDate(member.ended_at, true)}</time></dd></div>}
        </dl>
        {member.is_current_administrator ? <FormNotice>当前项目管理员受保护。如需变更，请在项目资料中交接管理员。</FormNotice> : member.account_id === ctx.authorization.account_id ? <FormNotice>不能在此修改本人的项目成员状态或角色。</FormNotice> : member.status === 'ended' ? <FormNotice>本次项目成员关系已结束，历史记录保留。如需重新加入，请添加新的成员关系。</FormNotice> : null}
        {(member.account_status !== 'active' || member.tenant_membership_status !== 'active') && <FormNotice>账号或租户成员资格不可用，恢复项目成员关系不会自动恢复上层资格。</FormNotice>}
        {actions.length > 0 && <div className="flex flex-wrap gap-2">{actions.map((action) => <Button key={action} variant="outline" className={action === 'member.remove' || action === 'member.suspend' ? 'text-destructive hover:text-destructive' : undefined} onClick={() => setMode(action)}>{memberOperations[action].label}</Button>)}</div>}
        {!canReadAuthorization ? <FormNotice>没有项目授权查看权限。</FormNotice> : auth.error ? <ErrorState message={auth.error.message} onRetry={() => void auth.refetch()} /> : !auth.data ? <LoadingState label="正在读取项目授权…" /> : <ProjectMemberAuthorization value={auth.data} />}
        <p className="text-xs text-muted-foreground">时间均为北京时间。</p>
      </>}
    </section>}
  </ResponsivePanel>;
}

export function AddProjectMemberPanel({ close, returnFocus }: PanelProps) {
  const ctx = useProject();
  return <ResponsivePanel open onOpenChange={(open) => { if (!open) close(); }} title="添加项目成员" description="从本租户有效成员中添加，并授予项目普通成员角色。" returnFocus={returnFocus} fallbackFocusSelector="[data-project-member-search]">
    {ctx.allows('ProjectMemberAdd') ? <AddMemberForm close={close} /> : <FormNotice error>当前不可添加项目成员。</FormNotice>}
  </ResponsivePanel>;
}

function AddMemberForm({ close }: { close: () => void }) {
  const ctx = useProject(), { client, account } = useIdentity(), cache = useQueryClient(), live = useLivePage();
  const [candidate, setCandidate] = useState<ProjectPerson>();
  return <MutationForm label="确认添加成员" disabled={!ctx.available || !candidate} cancel={close} submit={async (reason, requestId) => {
    if (!candidate) throw new Error('请选择成员。');
    await client.addProjectMember(ctx.project.tenant_id, ctx.project.id, { tenant_membership_id: candidate.tenant_membership_id, expected_project_version: ctx.project.version, reason, client_request_id: requestId });
    if (!live.current) return;
    refreshProjects(cache, account?.id, ctx.project.tenant_id, ctx.project.id);
    close();
  }}><CandidatePicker tenant={ctx.project.tenant_id} project={ctx.project.id} kind="member" label="选择项目成员" value={candidate} onChange={setCandidate} /><p className="text-sm leading-6 text-muted-foreground">添加后，可在成员详情中配置更多项目角色。</p></MutationForm>;
}
