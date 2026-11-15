import { useRef, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { RefreshCw, Search, UserPlus, X } from 'lucide-react';
import { Permissions as P, type ProjectMember } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@infrapro/ui/components/tabs';
import { DirectoryTable, type DirectoryColumn } from '@infrapro/ui/patterns/directory-table';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useCursorList } from '../../hooks/use-cursor-list';
import { AccountAvatar } from '../access/account-table';
import { useProject } from './context';
import { AddProjectMemberPanel, ProjectMemberPanel } from './member-panel';
import { projectMemberDate, type MemberMode } from './member-operations';
import { ProjectMemberEligibility, ProjectMemberIdentity, ProjectMemberMenu, ProjectMemberStatus } from './member-ui';

const statuses = [['current', '当前成员'], ['active', '正常'], ['suspended', '已暂停'], ['ended', '已结束'], ['all', '全部历史']] as const;

export function ProjectMembersPage({ membershipId }: { membershipId?: string } = {}) {
  const ctx = useProject(), { client } = useIdentity(), navigate = useNavigate();
  const { tenant_id: tenant, id: project } = ctx.project;
  const [q, setQ] = useState(''), [status, setStatus] = useState('current'), [pageSize, setPageSize] = useState(10);
  const [selection, setSelection] = useState<{ id: string; mode: MemberMode }>(), [adding, setAdding] = useState(false);
  const selected = selection ?? (membershipId ? { id: membershipId, mode: 'details' as const } : undefined);
  const returnFocus = useRef<HTMLElement | null>(null);
  const allowed = ctx.can(P.ProjectMemberRead);
  const list = useCursorList({ queryKey: [...ctx.key, 'members', q, status, pageSize], load: (cursor, signal) => client.projectMembers(tenant, project, { q, status: status === 'current' ? '' : status, cursor, limit: pageSize }, signal), enabled: allowed });
  const open = (member: ProjectMember, mode: MemberMode, trigger: HTMLElement | null) => { returnFocus.current = trigger; setSelection({ id: member.id, mode }); };
  const close = () => { setSelection(undefined); if (membershipId) void navigate({ to: '/t/$tenantId/p/$projectId/members', params: { tenantId: tenant, projectId: project } }); };
  const columns: DirectoryColumn<ProjectMember>[] = [
    { id: 'account', label: '成员账号', className: 'py-3', cell: (member) => <div className="flex min-w-0 items-center gap-3"><div className="hidden sm:block"><AccountAvatar name={member.display_name} /></div><div className="min-w-0 flex-1 space-y-1"><Button id={`project-member-${member.id}-name`} variant="link" className="h-auto max-w-full justify-start truncate p-0 text-left font-medium" title={member.display_name} onClick={(event) => open(member, 'details', event.currentTarget)}><span className="truncate">{member.display_name}</span></Button><p className="truncate text-xs text-muted-foreground" title={member.username}>{member.username}</p>{member.is_current_administrator && <p className="text-xs text-muted-foreground lg:hidden">当前管理员</p>}</div></div> },
    { id: 'status', label: '成员状态', className: 'w-24', cell: (member) => <div className="space-y-1"><ProjectMemberStatus member={member} />{(member.account_status !== 'active' || member.tenant_membership_status !== 'active') && <p className="text-xs text-muted-foreground">上层资格不可用</p>}</div> },
    { id: 'identity', label: '项目身份', secondary: true, className: 'lg:w-32', cell: (member) => <ProjectMemberIdentity member={member} /> },
    { id: 'eligibility', label: '账号 / 租户状态', secondary: true, className: 'lg:w-32', cell: (member) => <ProjectMemberEligibility member={member} /> },
    { id: 'joined', label: '加入时间', secondary: true, className: 'lg:w-32', cell: (member) => <time className="text-sm tabular-nums text-muted-foreground" dateTime={member.joined_at} title={`${projectMemberDate(member.joined_at, true)}（北京时间）`}>{projectMemberDate(member.joined_at)}</time> },
    { id: 'actions', label: '操作', className: 'w-14 text-right', cell: (member) => <ProjectMemberMenu member={member} open={(mode, trigger) => open(member, mode, trigger)} /> },
  ];
  if (!allowed) return <FormNotice error>没有项目成员读取权限。</FormNotice>;
  return <>
    <PageHeader title="项目成员" description="管理项目成员的状态、角色绑定与有效权限。" actions={<><Button variant="outline" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw aria-hidden="true" className={list.isFetching ? 'animate-spin' : undefined} />刷新成员</Button>{ctx.allows('ProjectMemberAdd') && <Button id="add-project-member" onClick={(event) => { returnFocus.current = event.currentTarget; setAdding(true); }}><UserPlus aria-hidden="true" />添加项目成员</Button>}</>} />
    <Tabs value={status} onValueChange={setStatus} className="gap-5">
      <TabsList aria-label="成员状态" className="max-sm:grid max-sm:h-auto max-sm:w-full max-sm:grid-cols-3">{statuses.map(([value, label]) => <TabsTrigger key={value} value={value} className="max-sm:min-h-11">{label}</TabsTrigger>)}</TabsList>
      <TabsContent value={status} className="space-y-4" aria-busy={list.isFetching}>
        <DirectoryTable label="项目成员列表" items={list.items} rowId={(member) => member.id} columns={columns} loading={list.isPending} error={list.error?.message} retry={() => void list.refetch()} empty={list.error ? '暂时无法读取成员，请重试。' : '没有符合条件的成员'} toolbar={<>
          <div className="relative w-full sm:w-72"><Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input data-project-member-search aria-label="搜索项目成员" placeholder="搜索用户名或显示名称…" className="h-9 pl-9 max-sm:h-11" maxLength={200} value={q} onChange={(event) => setQ(event.target.value)} /></div>
          {(q || status !== 'current') && <Button variant="ghost" className="h-9 max-sm:h-11" onClick={() => { setQ(''); setStatus('current'); }}>重置<X aria-hidden="true" /></Button>}
        </>} />
        <ListPagination label="项目成员列表" unit="位成员" count={list.items.length} page={list.page} pageCount={list.pageCount} hasNext={list.hasNext} fetching={list.isFetching} go={list.go} next={() => void list.next()} pageSize={pageSize} setPageSize={setPageSize} />
        <p className="text-xs text-muted-foreground">时间均为北京时间；已结束的成员关系与授权历史保留。</p>
      </TabsContent>
    </Tabs>
    {selected && <ProjectMemberPanel key={`${selected.id}:${selected.mode}`} id={selected.id} initialMode={selected.mode} close={close} returnFocus={returnFocus} />}
    {adding && <AddProjectMemberPanel close={() => setAdding(false)} returnFocus={returnFocus} />}
  </>;
}

export function ProjectMemberPage({ member }: { member: string }) { return <ProjectMembersPage membershipId={member} />; }
